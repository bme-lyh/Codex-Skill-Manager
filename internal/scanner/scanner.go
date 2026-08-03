package scanner

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/bme-lyh/Codex-Skill-Manager/internal/model"
)

const Version = "1.1.0"

type textFile struct {
	path string
	rel  string
}

type textScanResult struct {
	index    int
	findings []model.Finding
	err      error
}

type rule struct {
	id, title, explanation, recommendation string
	severity                               model.RiskSeverity
	confidence                             float64
	pattern                                *regexp.Regexp
}

var rules = []rule{
	{"CSM-INJ-001", "疑似提示注入", "命中了要求忽略既有系统或用户指令的表达。这类内容可能让 Agent 绕过原本的权限和安全边界；若它只是安全教程中的示例，可以在核对上下文后忽略。", "确认这段话是否真的需要改变指令优先级；不需要时删除或改写为说明文字", model.RiskHigh, .9, regexp.MustCompile(`(?i)(ignore|disregard).{0,40}(previous|system|user).{0,20}(instruction|prompt)`)},
	{"CSM-CRED-001", "浏览器会话凭据访问", "命中了 Cookie 导出、复制或请求表达。Cookie 往往等同于登录凭据，泄露后可能被用于冒用当前会话。", "禁止把 Cookie 粘贴到对话或日志；改用受控浏览器会话", model.RiskHigh, .95, regexp.MustCompile(`(?i)(cookie-editor|export.{0,20}cookie|cookie.{0,30}(header|string|复制|粘贴|导出))`)},
	{"CSM-CRED-002", "敏感凭据或私钥访问", "命中了私钥、密码、API Token 或凭据目录相关表达。若实际执行，可能读取或暴露本机身份凭据，因此按最高风险处理。", "删除此访问，或将范围限制为用户明确授权的凭据代理", model.RiskCritical, .9, regexp.MustCompile(`(?i)(\.ssh[/\\]|id_rsa|credentials|api[_ -]?key|github_token|gh_token|password)`)},
	{"CSM-EXEC-001", "动态命令或代码执行", "命中了动态执行接口。此类接口会把运行时字符串直接当作代码或命令执行，输入可控时可能执行任意操作。", "确认输入不可被外部内容控制，并只在受限沙箱中运行", model.RiskHigh, .85, regexp.MustCompile(`(?i)(invoke-expression|shell\s*=\s*true|os\.system\s*\(|subprocess\.|eval\s*\(|exec\s*\()`)},
	{"CSM-DOWN-001", "下载后直接执行", "命中了下载内容随后立即交给命令解释器或进程启动器的模式。下载内容没有独立审查窗口，供应链风险较高。", "拆分下载、人工审查、哈希校验和执行步骤，并固定来源版本", model.RiskCritical, .95, regexp.MustCompile(`(?i)(curl|wget|invoke-webrequest).{0,120}(\|\s*(sh|bash|powershell)|-exec|start-process)`)},
	{"CSM-DEL-001", "递归或批量删除", "命中了可一次删除整棵目录或多批文件的命令。目标路径判断错误时可能造成大范围、难以恢复的数据丢失。", "移除批量删除；改为逐个明确路径的隔离或可恢复移动", model.RiskCritical, .98, regexp.MustCompile(`(?i)(rm\s+-[a-z]*r|remove-item.{0,30}-recurse|rmdir\s+/s|del\s+/s|find\s+.{0,60}-delete)`)},
	{"CSM-PERSIST-001", "系统持久化或计划任务", "命中了开机启动、计划任务或系统持久化相关表达。它可能让操作在用户未主动运行 Skill 时继续发生。", "仅在功能确实需要时保留，并要求用户明确审批完整变更", model.RiskHigh, .9, regexp.MustCompile(`(?i)(schtasks|task scheduler|startup folder|runonce|crontab|launchd)`)},
	{"CSM-NET-001", "外部网络地址", "发现了外部 URL。它本身不一定危险，但可能涉及下载内容、发送数据或依赖第三方服务，需要确认具体用途。", "核对域名、用途、发送的数据范围和隐私影响", model.RiskMedium, .8, regexp.MustCompile(`https?://[^\s\)\]>"']+`)},
	{"CSM-OBF-001", "编码或混淆载荷", "命中了 Base64 解码或长编码字符串。编码内容不便直接审查，可能隐藏脚本、命令或外传数据。", "先解码并人工检查真实内容，不要直接执行", model.RiskHigh, .75, regexp.MustCompile(`(?i)(frombase64string|base64\.b64decode|buffer\.from\(.{0,80}base64|[A-Za-z0-9+/]{180,}={0,2})`)},
	{"CSM-CONFIG-001", "修改 Codex 全局配置", "命中了对 AGENTS.md、Codex 配置或全局 Skills 目录的写入表达。这可能改变其他任务的默认行为和权限边界。", "限制为明确的配置命令，并在修改前展示目标文件和差异", model.RiskHigh, .85, regexp.MustCompile(`(?i)(agents\.md|\.codex[/\\](config|skills)|codex_home).{0,60}(write|modify|overwrite|append|写入|修改|覆盖)`)},
}

var allowedExt = map[string]bool{
	".md": true, ".txt": true, ".json": true, ".yaml": true, ".yml": true,
	".py": true, ".js": true, ".ts": true, ".ps1": true, ".sh": true,
	".html": true, ".css": true, ".png": true, ".jpg": true, ".jpeg": true,
	".svg": true, ".docx": true, ".pdf": true,
}

var dangerousExt = map[string]bool{
	".exe": true, ".dll": true, ".msi": true, ".com": true, ".scr": true,
	".bat": true, ".cmd": true, ".psm1": true, ".vbs": true, ".jscript": true,
	".jar": true, ".lnk": true,
}

func Scan(root string, maxFiles int, maxFileBytes int64) (model.ScanReport, error) {
	return scanWithSystemDir(root, maxFiles, maxFileBytes, false, ".system")
}

// ScanRoot is the root-aware scanner entry point. It skips manager-reserved
// .system content and records RootID in the report; an auto-enabled root that
// has not been created yet yields a clean empty report.
func ScanRoot(rootID, root string, maxFiles int, maxFileBytes int64) (model.ScanReport, error) {
	if strings.TrimSpace(rootID) == "" {
		rootID = model.RootIDCodexDefault
	}
	if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
		now := time.Now().UTC()
		return model.ScanReport{ID: fmt.Sprintf("scan-%s", now.Format("20060102T150405.000000000")), Target: root, RootID: rootID, StartedAt: now, CompletedAt: now, HighestSeverity: model.RiskInfo, ActiveHighestSeverity: model.RiskInfo, Findings: []model.Finding{}, Skills: []model.ScanSkillSummary{}, ScannerVersion: Version, Status: "passed"}, nil
	}
	report, err := scanWithSystemDir(root, maxFiles, maxFileBytes, true, ".system")
	report.RootID = rootID
	for i := range report.Findings {
		report.Findings[i].RootID = rootID
	}
	for i := range report.Skills {
		report.Skills[i].RootID = rootID
	}
	return report, err
}

// ScanSkillRoot applies the explicit root system-directory policy.
func ScanSkillRoot(root model.SkillRoot, maxFiles int, maxFileBytes int64) (model.ScanReport, error) {
	rootID := root.ID
	if strings.TrimSpace(rootID) == "" {
		rootID = model.RootIDCodexDefault
	}
	if _, err := os.Stat(root.Path); errors.Is(err, os.ErrNotExist) {
		return ScanRoot(rootID, root.Path, maxFiles, maxFileBytes)
	}
	report, err := scanWithSystemDir(root.Path, maxFiles, maxFileBytes, true, model.RootSystemDir(root))
	report.RootID = rootID
	for i := range report.Findings {
		report.Findings[i].RootID = rootID
	}
	for i := range report.Skills {
		report.Skills[i].RootID = rootID
	}
	return report, err
}

// ScanRoots scans every enabled root independently. Independent reports keep
// per-root audit history and avoid conflating equal Skill names.
func ScanRoots(roots []model.SkillRoot, maxFiles int, maxFileBytes int64) ([]model.ScanReport, error) {
	reports := make([]model.ScanReport, 0, len(roots))
	for _, root := range roots {
		if !root.Enabled {
			continue
		}
		report, err := ScanSkillRoot(root, maxFiles, maxFileBytes)
		if err != nil {
			return nil, err
		}
		reports = append(reports, report)
	}
	return reports, nil
}

func ScanConfigured(roots []model.SkillRoot, maxFiles int, maxFileBytes int64) ([]model.ScanReport, error) {
	return ScanRoots(roots, maxFiles, maxFileBytes)
}

// ScanSkillsRoot excludes only the manager-owned directories directly beneath
// the configured Skills root. Candidate Skill scans must use Scan so a
// repository cannot hide untrusted content inside a reserved-looking folder.
func ScanSkillsRoot(root string, maxFiles int, maxFileBytes int64) (model.ScanReport, error) {
	return scanWithSystemDir(root, maxFiles, maxFileBytes, true, ".system")
}

func scan(root string, maxFiles int, maxFileBytes int64, skipRootInternals bool) (model.ScanReport, error) {
	return scanWithSystemDir(root, maxFiles, maxFileBytes, skipRootInternals, ".system")
}

func scanWithSystemDir(root string, maxFiles int, maxFileBytes int64, skipRootInternals bool, systemDir string) (model.ScanReport, error) {
	now := time.Now().UTC()
	report := model.ScanReport{
		ID:     fmt.Sprintf("scan-%s", now.Format("20060102T150405.000000000")),
		Target: root, StartedAt: now, ScannerVersion: Version, Status: "passed",
		HighestSeverity: model.RiskInfo, Findings: []model.Finding{},
	}
	textFiles := make([]textFile, 0)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipRootInternals && path != root && filepath.Clean(filepath.Dir(path)) == filepath.Clean(root) {
				if strings.EqualFold(d.Name(), systemDir) || d.Name() == ".csm-backups" || d.Name() == ".csm-quarantine" {
					return filepath.SkipDir
				}
			}
			if path != root {
				info, ierr := d.Info()
				if ierr != nil {
					return ierr
				}
				if info.Mode()&os.ModeSymlink != 0 {
					return fmt.Errorf("symbolic directory is forbidden: %s", path)
				}
			}
			return nil
		}
		report.FilesScanned++
		if report.FilesScanned > maxFiles {
			return fmt.Errorf("file count exceeds limit %d", maxFiles)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if info.Mode()&os.ModeSymlink != 0 {
			addFinding(&report, model.Finding{RuleID: "CSM-FS-001", Title: "包含符号链接", Severity: model.RiskCritical, Confidence: 1, File: filepath.ToSlash(rel), Explanation: "符号链接可能把扫描和安装范围指向 Skill 目录之外，导致读取或覆盖未授权文件。", Recommendation: "拒绝安装并移除符号链接"})
			return nil
		}
		if info.Size() > maxFileBytes {
			addFinding(&report, model.Finding{RuleID: "CSM-FS-002", Title: "文件超过大小限制", Severity: model.RiskHigh, Confidence: 1, File: filepath.ToSlash(rel), Explanation: "超大文件无法按正常策略完整审查，也可能消耗异常多的磁盘和扫描资源。", Recommendation: "核对文件必要性后再调整大小策略"})
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if dangerousExt[ext] {
			addFinding(&report, model.Finding{
				RuleID: "CSM-FILE-002", Title: "危险可执行文件", Severity: model.RiskCritical,
				Confidence: 1, File: filepath.ToSlash(rel), Evidence: ext,
				Explanation:    "Skill 中包含可直接执行或加载的二进制、脚本启动器或快捷方式，本地静态扫描无法证明其行为安全。",
				Recommendation: "优先移除该文件并改用可审查的源代码；若确认保留，由使用者明确执行人工忽略。",
			})
			return nil
		}
		if !allowedExt[ext] {
			addFinding(&report, model.Finding{RuleID: "CSM-FILE-001", Title: "未批准的文件类型", Severity: model.RiskHigh, Confidence: .95, File: filepath.ToSlash(rel), Evidence: ext, Explanation: "该扩展名不在允许审查的类型中，扫描器无法确认它是否包含可执行内容或隐藏载荷。", Recommendation: "人工确认文件用途、格式和来源；无法确认时拒绝安装"})
		}
		if info.Size() == 0 || info.Size() > 4<<20 || !isTextExt(ext) {
			return nil
		}
		textFiles = append(textFiles, textFile{path: path, rel: filepath.ToSlash(rel)})
		return nil
	})
	if err == nil {
		findings, scanErr := scanTextFiles(textFiles)
		if scanErr != nil {
			err = scanErr
		} else {
			for _, finding := range findings {
				addFinding(&report, finding)
			}
		}
	}
	report.CompletedAt = time.Now().UTC()
	sort.Slice(report.Findings, func(i, j int) bool {
		if report.Findings[i].Severity == report.Findings[j].Severity {
			if report.Findings[i].File != report.Findings[j].File {
				return report.Findings[i].File < report.Findings[j].File
			}
			if report.Findings[i].Line != report.Findings[j].Line {
				return report.Findings[i].Line < report.Findings[j].Line
			}
			return report.Findings[i].RuleID < report.Findings[j].RuleID
		}
		return severityRank(report.Findings[i].Severity) > severityRank(report.Findings[j].Severity)
	})
	if len(report.Findings) > 0 {
		report.Status = "findings"
	}
	return report, err
}

func scanTextFiles(files []textFile) ([]model.Finding, error) {
	if len(files) == 0 {
		return []model.Finding{}, nil
	}
	workerCount := runtime.GOMAXPROCS(0)
	if workerCount < 2 {
		workerCount = 2
	}
	if workerCount > 8 {
		workerCount = 8
	}
	if workerCount > len(files) {
		workerCount = len(files)
	}
	jobs := make(chan int)
	results := make(chan textScanResult, len(files))
	var workers sync.WaitGroup
	for worker := 0; worker < workerCount; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				findings, err := scanText(files[index].path, files[index].rel)
				results <- textScanResult{index: index, findings: findings, err: err}
			}
		}()
	}
	go func() {
		for index := range files {
			jobs <- index
		}
		close(jobs)
		workers.Wait()
		close(results)
	}()

	ordered := make([][]model.Finding, len(files))
	var firstErr error
	for result := range results {
		if result.err != nil && firstErr == nil {
			firstErr = result.err
		}
		ordered[result.index] = result.findings
	}
	findings := make([]model.Finding, 0)
	for _, fileFindings := range ordered {
		findings = append(findings, fileFindings...)
	}
	return findings, firstErr
}

func scanText(path, rel string) ([]model.Finding, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	findings := make([]model.Finding, 0)
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 64<<10), 4<<20)
	lineNo := 0
	for s.Scan() {
		lineNo++
		line := s.Text()
		if !utf8.ValidString(line) {
			findings = append(findings, model.Finding{RuleID: "CSM-ENC-001", Title: "文本编码异常", Severity: model.RiskHigh, Confidence: .9, File: rel, Line: lineNo, Explanation: "文本不是有效 UTF-8，可能导致审查内容与实际执行内容不一致，也可能用于隐藏载荷。", Recommendation: "转换为可审查编码并重新扫描"})
			continue
		}
		for _, r := range rules {
			match := r.pattern.FindString(line)
			if match == "" {
				continue
			}
			findings = append(findings, model.Finding{
				RuleID: r.id, Title: r.title, Severity: r.severity, Confidence: r.confidence,
				File: rel, Line: lineNo, Evidence: truncate(match, 180),
				Explanation: r.explanation, Recommendation: r.recommendation,
			})
		}
	}
	return findings, s.Err()
}

func isTextExt(ext string) bool {
	switch ext {
	case ".md", ".txt", ".json", ".yaml", ".yml", ".py", ".js", ".ts", ".ps1", ".sh", ".html", ".css", ".svg":
		return true
	default:
		return false
	}
}

func addFinding(report *model.ScanReport, f model.Finding) {
	f.FileClass = classifyFile(f.File)
	f.Category = categoryForRule(f.RuleID)
	f.Deterministic = deterministicRule(f.RuleID)
	report.Findings = append(report.Findings, f)
	if severityRank(f.Severity) > severityRank(report.HighestSeverity) {
		report.HighestSeverity = f.Severity
	}
}

func classifyFile(path string) string {
	clean := strings.ToLower(filepath.ToSlash(path))
	base := strings.ToLower(filepath.Base(clean))
	switch {
	case base == "skill.md" || base == "agents.md" || strings.Contains(clean, "/agents/"):
		return "instruction"
	case strings.Contains(clean, "/test/") || strings.Contains(clean, "/tests/") ||
		strings.Contains(clean, "/fixtures/") || strings.HasSuffix(base, "_test.go") ||
		strings.HasPrefix(clean, "test/") || strings.HasPrefix(clean, "tests/") ||
		strings.HasPrefix(clean, "fixtures/") ||
		strings.HasSuffix(base, ".test.ts") || strings.HasSuffix(base, ".test.js"):
		return "test"
	case strings.Contains(clean, "/docs/") || strings.Contains(clean, "/examples/") ||
		strings.HasPrefix(clean, "docs/") || strings.HasPrefix(clean, "examples/") ||
		strings.HasPrefix(base, "readme"):
		return "documentation"
	case strings.Contains(clean, "/scripts/") || strings.HasPrefix(clean, "scripts/") ||
		strings.Contains(clean, "/src/") || strings.HasPrefix(clean, "src/") ||
		isExecutableSource(filepath.Ext(clean)):
		return "runtime"
	default:
		return "asset"
	}
}

func isExecutableSource(ext string) bool {
	switch ext {
	case ".py", ".js", ".ts", ".ps1", ".sh", ".html":
		return true
	default:
		return false
	}
}

func categoryForRule(ruleID string) string {
	switch {
	case strings.HasPrefix(ruleID, "CSM-FS-"), strings.HasPrefix(ruleID, "CSM-FILE-"), strings.HasPrefix(ruleID, "CSM-ENC-"):
		return "filesystem"
	case strings.HasPrefix(ruleID, "CSM-CRED-"):
		return "credentials"
	case strings.HasPrefix(ruleID, "CSM-EXEC-"), strings.HasPrefix(ruleID, "CSM-DOWN-"):
		return "execution"
	case strings.HasPrefix(ruleID, "CSM-DEL-"):
		return "destructive"
	case strings.HasPrefix(ruleID, "CSM-NET-"):
		return "network"
	case strings.HasPrefix(ruleID, "CSM-PERSIST-"):
		return "persistence"
	case strings.HasPrefix(ruleID, "CSM-INJ-"):
		return "prompt-injection"
	case strings.HasPrefix(ruleID, "CSM-OBF-"):
		return "obfuscation"
	case strings.HasPrefix(ruleID, "CSM-CONFIG-"):
		return "configuration"
	default:
		return "other"
	}
}

func deterministicRule(ruleID string) bool {
	switch ruleID {
	case "CSM-FS-001", "CSM-FILE-002", "CSM-DEL-001":
		return true
	default:
		return false
	}
}

func severityRank(s model.RiskSeverity) int {
	switch s {
	case model.RiskCritical:
		return 5
	case model.RiskHigh:
		return 4
	case model.RiskMedium:
		return 3
	case model.RiskLow:
		return 2
	default:
		return 1
	}
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
