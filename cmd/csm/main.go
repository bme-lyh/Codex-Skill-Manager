package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bme-lyh/Codex-Skill-Manager/internal/manager"
	"github.com/bme-lyh/Codex-Skill-Manager/internal/model"
	"github.com/bme-lyh/Codex-Skill-Manager/internal/scheduler"
)

type envelope struct {
	SchemaVersion string `json:"schemaVersion"`
	Command       string `json:"command"`
	Status        string `json:"status"`
	Data          any    `json:"data,omitempty"`
	Error         string `json:"error,omitempty"`
}

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }
func (s *stringList) Set(value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("empty value")
	}
	*s = append(*s, value)
	return nil
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	configPath, jsonOutput, args := globals(args)
	if len(args) == 0 {
		printHelp()
		return 2
	}
	command := args[0]
	if command == "version" {
		fmt.Println(model.Version)
		return 0
	}
	m, err := manager.Open(configPath)
	if err != nil {
		return output(command, nil, err, jsonOutput)
	}
	defer m.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var data any
	switch command {
	case "discover", "dashboard":
		data, err = m.Dashboard()
	case "bootstrap":
		err = m.BootstrapCurrentSkills()
		if err == nil {
			data, err = m.Dashboard()
		}
	case "manage", "adopt":
		data, err = manage(m, args[1:])
	case "group":
		data, err = group(m, args[1:])
	case "audit":
		fs := flag.NewFlagSet("audit", flag.ContinueOnError)
		skill := fs.String("skill", "", "skill name")
		_ = fs.Parse(args[1:])
		target := ""
		if *skill != "" {
			target = filepath.Join(m.Config.Paths.SkillsRoot, *skill)
		}
		data, err = m.Audit(target)
	case "check":
		fs := flag.NewFlagSet("check", flag.ContinueOnError)
		var groups stringList
		fs.Var(&groups, "group", "source group ID; repeatable")
		force := fs.Bool("force", false, "bypass the short-lived GitHub cache")
		if parseErr := fs.Parse(args[1:]); parseErr != nil {
			err = parseErr
		} else {
			data, err = m.CheckUpdatesSelected(ctx, groups, *force)
		}
	case "github-auth":
		data = m.ValidateGitHubCredentials(ctx)
	case "codex":
		data, err = codexCommand(m, args[1:])
	case "update":
		fs := flag.NewFlagSet("update", flag.ContinueOnError)
		groupID := fs.String("group", "", "explicit GitHub source group ID")
		if parseErr := fs.Parse(args[1:]); parseErr != nil {
			err = parseErr
		} else {
			data, err = m.PrepareUpdate(ctx, *groupID)
		}
	case "install":
		data, err = install(ctx, m, args[1:])
	case "remove":
		if len(args) < 2 {
			err = errors.New("remove requires one or more explicit skill names")
		} else {
			data, err = m.Quarantine(args[1:])
		}
	case "restore":
		fs := flag.NewFlagSet("restore", flag.ContinueOnError)
		skill := fs.String("skill", "", "skill name")
		tx := fs.String("transaction", "", "quarantine transaction")
		_ = fs.Parse(args[1:])
		data, err = m.Restore(*skill, *tx)
	case "rollback":
		fs := flag.NewFlagSet("rollback", flag.ContinueOnError)
		tx := fs.String("transaction", "", "transaction ID")
		_ = fs.Parse(args[1:])
		data, err = m.Rollback(*tx)
	case "history":
		data, err = m.History(100)
	case "reports":
		data, err = m.Reports(100)
	case "warning":
		data, err = warning(m, args[1:])
	case "schedule":
		fs := flag.NewFlagSet("schedule", flag.ContinueOnError)
		enabled := fs.Bool("enabled", true, "enable task")
		frequency := fs.String("frequency", "weekly", "daily or weekly")
		at := fs.String("at", "09:00", "HH:mm")
		_ = fs.Parse(args[1:])
		exe, _ := os.Executable()
		err = scheduler.Configure(exe, m.ConfigPath, *frequency, *at, *enabled)
		data = map[string]any{"enabled": *enabled, "frequency": *frequency, "time": *at}
	case "doctor":
		data = doctor(m)
	default:
		err = fmt.Errorf("unknown command: %s", command)
	}
	return output(command, data, err, jsonOutput)
}

func manage(m *manager.Manager, args []string) (any, error) {
	fs := flag.NewFlagSet("manage", flag.ContinueOnError)
	apply := fs.Bool("apply", false, "apply the management plan")
	planID := fs.String("plan-id", "", "existing management plan ID")
	var selected stringList
	fs.Var(&selected, "skill", "unmanaged skill name; repeatable")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if *planID != "" {
		if !*apply {
			return nil, errors.New("--plan-id requires --apply")
		}
		return m.ApplyAdoption(*planID, selected)
	}
	if *apply {
		return nil, errors.New("--apply requires --plan-id")
	}
	return m.PrepareAdoption(selected)
}

func group(m *manager.Manager, args []string) (any, error) {
	if len(args) == 0 {
		return nil, errors.New("group requires create, rename, reorder, or move")
	}
	switch args[0] {
	case "create":
		fs := flag.NewFlagSet("group create", flag.ContinueOnError)
		name := fs.String("name", "", "new group name")
		if err := fs.Parse(args[1:]); err != nil {
			return nil, err
		}
		return m.CreateGroup(*name)
	case "rename":
		fs := flag.NewFlagSet("group rename", flag.ContinueOnError)
		id := fs.String("id", "", "group ID")
		name := fs.String("name", "", "new group name")
		if err := fs.Parse(args[1:]); err != nil {
			return nil, err
		}
		return m.RenameGroup(*id, *name)
	case "reorder":
		fs := flag.NewFlagSet("group reorder", flag.ContinueOnError)
		var ids stringList
		fs.Var(&ids, "id", "group ID in desired order; repeatable")
		if err := fs.Parse(args[1:]); err != nil {
			return nil, err
		}
		return m.ReorderGroups(ids)
	case "move":
		fs := flag.NewFlagSet("group move", flag.ContinueOnError)
		id := fs.String("group", "", "target group ID")
		var skills stringList
		fs.Var(&skills, "skill", "skill name; repeatable")
		if err := fs.Parse(args[1:]); err != nil {
			return nil, err
		}
		return m.MoveSkillsToGroup(skills, *id)
	default:
		return nil, fmt.Errorf("unknown group action: %s", args[0])
	}
}

func warning(m *manager.Manager, args []string) (any, error) {
	fs := flag.NewFlagSet("warning", flag.ContinueOnError)
	var fingerprints stringList
	fs.Var(&fingerprints, "fingerprint", "finding fingerprint; repeatable for a cluster")
	clusterID := fs.String("cluster", "", "risk cluster ID")
	rule := fs.String("rule", "", "finding rule ID")
	file := fs.String("file", "", "finding file")
	fileClass := fs.String("file-class", "", "risk cluster file class")
	deterministic := fs.Bool("deterministic", false, "cluster is a deterministic local baseline")
	confirmDeterministic := fs.Bool("confirm-deterministic", false, "deprecated compatibility flag; human ignore no longer needs extra confirmation")
	reportID := fs.String("report", "", "apply the decision to every matching cluster in a scan report")
	reason := fs.String("reason", "", "optional ignore reason")
	restore := fs.Bool("restore", false, "restore a previously ignored warning")
	dryRun := fs.Bool("dry-run", false, "show explicit targets without changing state")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if *reportID != "" {
		reports, err := m.Reports(500)
		if err != nil {
			return nil, err
		}
		for _, report := range reports {
			if report.ID != *reportID {
				continue
			}
			clusters := make([]model.RiskCluster, 0, len(report.Clusters))
			targets := make([]string, 0, len(report.Clusters))
			for _, cluster := range report.Clusters {
				if cluster.Ignored == *restore {
					clusters = append(clusters, cluster)
					targets = append(targets, cluster.ID)
				}
			}
			if !*dryRun && len(clusters) > 0 {
				if err := m.SetRiskClustersIgnored(clusters, !*restore, *reason); err != nil {
					return nil, err
				}
			}
			return map[string]any{
				"reportId": *reportID, "clusterIds": targets, "dryRun": *dryRun,
				"ignored": !*restore, "reason": *reason,
			}, nil
		}
		return nil, fmt.Errorf("scan report not found: %s", *reportID)
	}
	if *clusterID != "" {
		cluster := model.RiskCluster{
			ID: *clusterID, RuleID: *rule, FileClass: *fileClass, Deterministic: *deterministic,
			Fingerprints: fingerprints,
		}
		if !*dryRun {
			if err := m.SetRiskClusterIgnored(cluster, !*restore, *reason, *confirmDeterministic); err != nil {
				return nil, err
			}
		}
		return map[string]any{
			"clusterId": *clusterID, "fingerprints": fingerprints,
			"ignored": !*restore, "reason": *reason, "dryRun": *dryRun,
		}, nil
	}
	if len(fingerprints) != 1 {
		return nil, errors.New("warning requires one --fingerprint or a --cluster with repeatable fingerprints")
	}
	finding := model.Finding{Fingerprint: fingerprints[0], RuleID: *rule, File: *file}
	if !*dryRun {
		if err := m.SetFindingIgnored(finding, !*restore, *reason); err != nil {
			return nil, err
		}
	}
	return map[string]any{
		"fingerprint": fingerprints[0],
		"ignored":     !*restore,
		"reason":      *reason,
		"dryRun":      *dryRun,
	}, nil
}

func codexCommand(m *manager.Manager, args []string) (any, error) {
	if len(args) == 0 || args[0] == "status" {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		return m.CodexCLIStatus(ctx), nil
	}
	if args[0] != "review" {
		return nil, errors.New("codex supports status or review")
	}
	fs := flag.NewFlagSet("codex review", flag.ContinueOnError)
	reportID := fs.String("report", "", "scan report ID")
	if err := fs.Parse(args[1:]); err != nil {
		return nil, err
	}
	if strings.TrimSpace(*reportID) == "" {
		return nil, errors.New("codex review requires --report")
	}
	reports, err := m.Reports(500)
	if err != nil {
		return nil, err
	}
	for _, report := range reports {
		if report.ID != *reportID {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(m.Config.CodexReview.TimeoutSeconds)*time.Second)
		defer cancel()
		return m.ReviewScanWithCodex(ctx, report)
	}
	return nil, fmt.Errorf("scan report not found: %s", *reportID)
}

func install(ctx context.Context, m *manager.Manager, args []string) (any, error) {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	rawURL := fs.String("url", "", "GitHub URL")
	local := fs.String("local", "", "local directory")
	ref := fs.String("ref", "", "branch, tag, or commit")
	all := fs.Bool("all", false, "select all discovered skills")
	apply := fs.Bool("apply", false, "apply the plan")
	acceptHigh := fs.Bool("accept-high-risk", false, "deprecated compatibility flag; use warning decisions")
	planID := fs.String("plan-id", "", "existing plan ID")
	var selected stringList
	fs.Var(&selected, "skill", "skill name; repeatable")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if *planID != "" {
		if !*apply {
			return nil, errors.New("--plan-id requires --apply")
		}
		return m.ApplyInstall(*planID, selected, *acceptHigh)
	}
	if (*rawURL == "") == (*local == "") {
		return nil, errors.New("provide exactly one of --url or --local")
	}
	var preview any
	var p interface {
	}
	_ = p
	if *rawURL != "" {
		value, err := m.PrepareGitHub(ctx, *rawURL, *ref)
		if err != nil {
			return nil, err
		}
		if *all {
			for _, skill := range value.Skills {
				selected = append(selected, skill.Name)
			}
		}
		if *apply {
			return m.ApplyInstall(value.ID, selected, *acceptHigh)
		}
		preview = value
	} else {
		value, err := m.PrepareLocal(*local)
		if err != nil {
			return nil, err
		}
		if *all {
			for _, skill := range value.Skills {
				selected = append(selected, skill.Name)
			}
		}
		if *apply {
			return m.ApplyInstall(value.ID, selected, *acceptHigh)
		}
		preview = value
	}
	return preview, nil
}

func globals(args []string) (string, bool, []string) {
	var configPath string
	var jsonOutput bool
	var rest []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			jsonOutput = true
		case "--config":
			if i+1 < len(args) {
				i++
				configPath = args[i]
			}
		default:
			rest = append(rest, args[i])
		}
	}
	return configPath, jsonOutput, rest
}

func output(command string, data any, err error, asJSON bool) int {
	status := "ok"
	code := 0
	message := ""
	if err != nil {
		status, code, message = "error", 1, err.Error()
	}
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(envelope{SchemaVersion: "1.0", Command: command, Status: status, Data: data, Error: message})
	} else if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
	} else {
		encoded, _ := json.MarshalIndent(data, "", "  ")
		fmt.Println(string(encoded))
	}
	return code
}

func doctor(m *manager.Manager) map[string]any {
	checks := map[string]any{
		"version":          model.Version,
		"skillsRoot":       m.Config.Paths.SkillsRoot,
		"dataRoot":         m.Config.Paths.DataRoot,
		"logsRoot":         m.Config.Paths.LogsRoot,
		"reportsRoot":      m.Config.Paths.ReportsRoot,
		"configPath":       m.ConfigPath,
		"skillsRootExists": exists(m.Config.Paths.SkillsRoot),
		"dataRootExists":   exists(m.Config.Paths.DataRoot),
		"platform":         "windows",
	}
	return checks
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func printHelp() {
	fmt.Print(`Codex Skill Manager

用法:
  csm [--config PATH] [--json] <command>

命令:
  discover              发现已安装 Skills
  bootstrap             管理当前已知的历史 Skills
  manage                分析并管理一个或多个现有 Skill
  group                 新建、改名、排序分组或移动 Skills
  audit [--skill NAME]  本地安全扫描
  check                 检查 GitHub 更新；支持 --group 与 --force
  github-auth           验证 GitHub 凭据并显示 API 限额
  codex status          检查 Codex CLI 与登录状态
  codex review          对指定 --report 运行可选语义复核
  update --group ID     为一个来源创建安全更新计划
  install               从 GitHub URL 或本地目录创建安装计划
  remove NAME [...]     移动一个或多个 Skill 到隔离区
  restore               从隔离区恢复
  rollback              回滚事务
  history               查看事务历史
  reports               查看扫描报告
  warning               忽略或恢复指定安全警告
  schedule              配置定时检查
  doctor                环境诊断
  version               显示版本
`)
}
