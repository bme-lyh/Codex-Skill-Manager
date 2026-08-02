package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
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

type usageError struct {
	err error
}

func (e *usageError) Error() string { return e.err.Error() }
func (e *usageError) Unwrap() error { return e.err }

type stringList []string

var configureSchedule = scheduler.Configure

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
	return runCLI(args, os.Stdout, os.Stderr)
}

func runCLI(args []string, stdout, stderr io.Writer) int {
	configPath, jsonOutput, args, err := globals(args)
	if err != nil {
		return output("", nil, err, jsonOutput, stdout, stderr)
	}
	if len(args) == 0 {
		printHelp(stdout)
		return 2
	}
	command := args[0]
	if command == "version" {
		if len(args) != 1 {
			return output(command, nil, usagef("version does not accept arguments"), jsonOutput, stdout, stderr)
		}
		if jsonOutput {
			return output(command, map[string]string{"version": model.Version}, nil, true, stdout, stderr)
		}
		fmt.Fprintln(stdout, model.Version)
		return 0
	}
	if !knownCommand(command) {
		return output(command, nil, usagef("unknown command: %s", command), jsonOutput, stdout, stderr)
	}
	m, err := manager.Open(configPath)
	if err != nil {
		return output(command, nil, err, jsonOutput, stdout, stderr)
	}
	defer m.Close()

	commandTimeout := 2 * time.Minute
	if command == "install" && containsArgument(args[1:], "--assist") {
		commandTimeout = 30 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	var data any
	switch command {
	case "discover", "dashboard":
		if err = requireNoArguments(command, args[1:]); err == nil {
			data, err = m.Dashboard()
		}
	case "bootstrap":
		if err = requireNoArguments(command, args[1:]); err == nil {
			err = m.BootstrapCurrentSkills()
		}
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
		if parseErr := parseFlags(fs, args[1:]); parseErr != nil {
			err = parseErr
		} else if *skill != "" {
			data, err = m.AuditSkills([]string{*skill})
		} else {
			var dashboard model.Dashboard
			dashboard, err = m.Dashboard()
			if err == nil {
				names := make([]string, 0, len(dashboard.Skills))
				for _, discovered := range dashboard.Skills {
					if !discovered.System {
						names = append(names, discovered.Name)
					}
				}
				if len(names) == 0 {
					data, err = m.Audit("")
				} else {
					data, err = m.AuditSkills(names)
				}
			}
		}
	case "check":
		fs := flag.NewFlagSet("check", flag.ContinueOnError)
		var groups stringList
		fs.Var(&groups, "group", "source group ID; repeatable")
		force := fs.Bool("force", false, "bypass the short-lived GitHub cache")
		if parseErr := parseFlags(fs, args[1:]); parseErr != nil {
			err = parseErr
		} else {
			data, err = m.CheckUpdatesSelected(ctx, groups, *force)
		}
	case "github-auth":
		if err = requireNoArguments(command, args[1:]); err == nil {
			data = m.ValidateGitHubCredentials(ctx)
		}
	case "codex":
		data, err = codexCommand(m, args[1:])
	case "update":
		fs := flag.NewFlagSet("update", flag.ContinueOnError)
		groupID := fs.String("group", "", "explicit GitHub source group ID")
		if parseErr := parseFlags(fs, args[1:]); parseErr != nil {
			err = parseErr
		} else if strings.TrimSpace(*groupID) == "" {
			err = usagef("update requires --group")
		} else {
			data, err = m.PrepareUpdate(ctx, *groupID)
		}
	case "install":
		data, err = install(ctx, m, args[1:])
	case "remove":
		if len(args) < 2 {
			err = usagef("remove requires one or more explicit skill names")
		} else if invalid := invalidSkillTarget(args[1:]); invalid != "" {
			err = usagef("remove requires explicit skill names; invalid target: %s", invalid)
		} else {
			data, err = m.Quarantine(args[1:])
		}
	case "restore":
		fs := flag.NewFlagSet("restore", flag.ContinueOnError)
		skill := fs.String("skill", "", "skill name")
		tx := fs.String("transaction", "", "quarantine transaction")
		if parseErr := parseFlags(fs, args[1:]); parseErr != nil {
			err = parseErr
		} else if strings.TrimSpace(*skill) == "" || strings.TrimSpace(*tx) == "" {
			err = usagef("restore requires --skill and --transaction")
		} else {
			data, err = m.Restore(*skill, *tx)
		}
	case "rollback":
		fs := flag.NewFlagSet("rollback", flag.ContinueOnError)
		tx := fs.String("transaction", "", "transaction ID")
		if parseErr := parseFlags(fs, args[1:]); parseErr != nil {
			err = parseErr
		} else if strings.TrimSpace(*tx) == "" {
			err = usagef("rollback requires --transaction")
		} else {
			data, err = m.Rollback(*tx)
		}
	case "history":
		if err = requireNoArguments(command, args[1:]); err == nil {
			data, err = m.History(100)
		}
	case "reports":
		if err = requireNoArguments(command, args[1:]); err == nil {
			data, err = m.Reports(100)
		}
	case "warning":
		data, err = warning(m, args[1:])
	case "schedule":
		fs := flag.NewFlagSet("schedule", flag.ContinueOnError)
		enabled := fs.Bool("enabled", true, "enable task")
		frequency := fs.String("frequency", "weekly", "daily or weekly")
		at := fs.String("at", "09:00", "HH:mm")
		if parseErr := parseFlags(fs, args[1:]); parseErr != nil {
			err = parseErr
		} else if normalized := strings.ToLower(strings.TrimSpace(*frequency)); normalized != "daily" && normalized != "weekly" {
			err = usagef("--frequency must be daily or weekly")
		} else if _, parseErr := time.Parse("15:04", *at); parseErr != nil {
			err = usagef("--at must use HH:mm")
		} else {
			*frequency = strings.ToLower(strings.TrimSpace(*frequency))
			exe, executableErr := os.Executable()
			if executableErr != nil {
				err = executableErr
			} else {
				err = configureSchedule(exe, m.ConfigPath, *frequency, *at, *enabled)
			}
			data = map[string]any{"enabled": *enabled, "frequency": *frequency, "time": *at}
		}
	case "doctor":
		if err = requireNoArguments(command, args[1:]); err == nil {
			data = doctor(m)
		}
	}
	return output(command, data, err, jsonOutput, stdout, stderr)
}

func manage(m *manager.Manager, args []string) (any, error) {
	fs := flag.NewFlagSet("manage", flag.ContinueOnError)
	apply := fs.Bool("apply", false, "apply the management plan")
	planID := fs.String("plan-id", "", "existing management plan ID")
	var selected stringList
	fs.Var(&selected, "skill", "unmanaged skill name; repeatable")
	if err := parseFlags(fs, args); err != nil {
		return nil, err
	}
	if *planID != "" {
		if !*apply {
			return nil, usagef("--plan-id requires --apply")
		}
		if len(selected) == 0 {
			return nil, usagef("applying a management plan requires at least one --skill")
		}
		return m.ApplyAdoption(*planID, selected)
	}
	if *apply {
		return nil, usagef("--apply requires --plan-id")
	}
	return m.PrepareAdoption(selected)
}

func group(m *manager.Manager, args []string) (any, error) {
	if len(args) == 0 {
		return nil, usagef("group requires create, rename, reorder, or move")
	}
	switch args[0] {
	case "create":
		fs := flag.NewFlagSet("group create", flag.ContinueOnError)
		name := fs.String("name", "", "new group name")
		if err := parseFlags(fs, args[1:]); err != nil {
			return nil, err
		}
		if strings.TrimSpace(*name) == "" {
			return nil, usagef("group create requires --name")
		}
		return m.CreateGroup(*name)
	case "rename":
		fs := flag.NewFlagSet("group rename", flag.ContinueOnError)
		id := fs.String("id", "", "group ID")
		name := fs.String("name", "", "new group name")
		if err := parseFlags(fs, args[1:]); err != nil {
			return nil, err
		}
		if strings.TrimSpace(*id) == "" || strings.TrimSpace(*name) == "" {
			return nil, usagef("group rename requires --id and --name")
		}
		return m.RenameGroup(*id, *name)
	case "reorder":
		fs := flag.NewFlagSet("group reorder", flag.ContinueOnError)
		var ids stringList
		fs.Var(&ids, "id", "group ID in desired order; repeatable")
		if err := parseFlags(fs, args[1:]); err != nil {
			return nil, err
		}
		if len(ids) == 0 {
			return nil, usagef("group reorder requires at least one --id")
		}
		return m.ReorderGroups(ids)
	case "move":
		fs := flag.NewFlagSet("group move", flag.ContinueOnError)
		id := fs.String("group", "", "target group ID")
		var skills stringList
		fs.Var(&skills, "skill", "skill name; repeatable")
		if err := parseFlags(fs, args[1:]); err != nil {
			return nil, err
		}
		if strings.TrimSpace(*id) == "" || len(skills) == 0 {
			return nil, usagef("group move requires --group and at least one --skill")
		}
		return m.MoveSkillsToGroup(skills, *id)
	default:
		return nil, usagef("unknown group action: %s", args[0])
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
	confirmDeterministic := fs.Bool("confirm-deterministic", false, "explicitly confirm accepting a High-risk cluster; Critical clusters cannot be ignored")
	reportID := fs.String("report", "", "apply the decision to every matching cluster in a scan report")
	reason := fs.String("reason", "", "audit reason; required when explicitly accepting a High-risk cluster")
	restore := fs.Bool("restore", false, "restore a previously ignored warning")
	dryRun := fs.Bool("dry-run", false, "show explicit targets without changing state")
	if err := parseFlags(fs, args); err != nil {
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
		return nil, usagef("warning requires one --fingerprint or a --cluster with repeatable fingerprints")
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
		if len(args) > 1 {
			return nil, usagef("codex status does not accept arguments")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		return m.CodexCLIStatus(ctx), nil
	}
	if args[0] != "review" {
		return nil, usagef("codex supports status or review")
	}
	fs := flag.NewFlagSet("codex review", flag.ContinueOnError)
	reportID := fs.String("report", "", "scan report ID")
	var skills stringList
	fs.Var(&skills, "skill", "Skill name to review (repeatable; defaults to all detected Skills)")
	if err := parseFlags(fs, args[1:]); err != nil {
		return nil, err
	}
	if strings.TrimSpace(*reportID) == "" {
		return nil, usagef("codex review requires --report")
	}
	reports, err := m.Reports(500)
	if err != nil {
		return nil, err
	}
	for _, report := range reports {
		if report.ID != *reportID {
			continue
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		return m.ReviewScanWithCodex(ctx, report, skills, nil)
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
	assess := fs.Bool("assess", false, "read and persist the mandatory layered assessment for an existing source plan")
	assist := fs.Bool("assist", false, "use the consent-gated Codex project scan and assisted-install flow")
	acceptHigh := fs.Bool("accept-high-risk", false, "final apply acknowledgement for previously audited High-risk cluster decisions")
	planID := fs.String("plan-id", "", "existing plan ID")
	projectScanID := fs.String("project-scan-id", "", "completed Codex project scan ID")
	createPlan := fs.Bool("create-plan", false, "approve creating an assisted-install plan from --project-scan-id")
	projectRoot := fs.String("project-root", "", "target Git or SVN working tree for an approved MCP integration")
	var selected stringList
	var grants stringList
	fs.Var(&selected, "skill", "skill name; repeatable")
	fs.Var(&grants, "grant", "approved assisted-install permission ID; repeatable")
	if err := parseFlags(fs, args); err != nil {
		return nil, err
	}
	if *projectScanID != "" {
		if !*assist || !*createPlan {
			return nil, usagef("--project-scan-id requires --assist --create-plan")
		}
		if *planID != "" || *apply || *assess || *rawURL != "" || *local != "" || *ref != "" ||
			*all || len(selected) > 0 || len(grants) > 0 || *projectRoot != "" {
			return nil, usagef("--project-scan-id --create-plan cannot be combined with a source, assess, apply, selection, grant, or project-root flag")
		}
		return m.AnalyzeInstallFromProjectScan(ctx, *projectScanID, nil)
	}
	if *createPlan {
		return nil, usagef("--create-plan requires --project-scan-id")
	}
	if *planID != "" {
		if *assess {
			if *apply || *assist || *all || len(selected) > 0 || len(grants) > 0 ||
				*projectRoot != "" || *acceptHigh || *rawURL != "" || *local != "" || *ref != "" {
				return nil, usagef("--plan-id --assess cannot be combined with source, apply, assist, selection, grant, project-root, or risk-acceptance flags")
			}
			return m.AssessInstallSource(*planID)
		}
		if !*apply {
			return nil, usagef("--plan-id requires --assess or --apply")
		}
		if *assist {
			if *all {
				plan, err := m.GetAssistedInstallPlan(*planID)
				if err != nil {
					return nil, err
				}
				for _, skill := range plan.Skills {
					selected = append(selected, skill.Name)
				}
			}
			if len(selected) == 0 {
				return nil, usagef("applying an assisted plan requires --all or at least one --skill")
			}
			return m.ApplyAssistedInstall(ctx, *planID, selected, grants, *projectRoot, nil)
		}
		if *all {
			return nil, usagef("--all is supported only when applying an assisted plan")
		}
		if len(selected) == 0 {
			return nil, usagef("applying an install plan requires at least one --skill")
		}
		return m.ApplyInstall(*planID, selected, *acceptHigh)
	}
	if *assess {
		return nil, usagef("--assess requires --plan-id")
	}
	if *assist && *apply {
		return nil, usagef(
			"assisted installation is three-phase: scan the source, approve plan creation with --project-scan-id ID --create-plan, then apply the reviewed --plan-id ID with explicit --grant values",
		)
	}
	if (*rawURL == "") == (*local == "") {
		return nil, usagef("provide exactly one of --url or --local")
	}
	if *apply && !*all && len(selected) == 0 {
		return nil, usagef("--apply requires --all or at least one --skill")
	}
	var preview any
	if *rawURL != "" {
		value, err := m.PrepareGitHub(ctx, *rawURL, *ref)
		if err != nil {
			return nil, err
		}
		if *assist {
			return m.ScanProjectWithCodex(ctx, value.ID, nil)
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
		if *assist {
			return m.ScanProjectWithCodex(ctx, value.ID, nil)
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

func globals(args []string) (string, bool, []string, error) {
	var configPath string
	var jsonOutput bool
	var rest []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			jsonOutput = true
		case "--config":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return "", jsonOutput, nil, usagef("--config requires a path")
			}
			i++
			configPath = args[i]
		default:
			rest = append(rest, args[i])
		}
	}
	return configPath, jsonOutput, rest, nil
}

func output(command string, data any, err error, asJSON bool, stdout, stderr io.Writer) int {
	status := "ok"
	code := 0
	message := ""
	if err != nil {
		status, code, message = "error", 1, err.Error()
		var invalidUsage *usageError
		if errors.As(err, &invalidUsage) {
			code = 2
		}
	}
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(envelope{SchemaVersion: "1.0", Command: command, Status: status, Data: data, Error: message})
	} else if err != nil {
		fmt.Fprintln(stderr, "Error:", err)
	} else {
		encoded, _ := json.MarshalIndent(data, "", "  ")
		fmt.Fprintln(stdout, string(encoded))
	}
	return code
}

func usagef(format string, args ...any) error {
	return &usageError{err: fmt.Errorf(format, args...)}
}

func parseFlags(fs *flag.FlagSet, args []string) error {
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return &usageError{err: err}
	}
	if fs.NArg() != 0 {
		return usagef("unexpected argument(s): %s", strings.Join(fs.Args(), " "))
	}
	return nil
}

func knownCommand(command string) bool {
	switch command {
	case "discover", "dashboard", "bootstrap", "manage", "adopt", "group", "audit", "check",
		"github-auth", "codex", "update", "install", "remove", "restore", "rollback", "history",
		"reports", "warning", "schedule", "doctor":
		return true
	default:
		return false
	}
}

func requireNoArguments(command string, args []string) error {
	if len(args) != 0 {
		return usagef("%s does not accept arguments", command)
	}
	return nil
}

func invalidSkillTarget(names []string) string {
	for _, name := range names {
		if strings.TrimSpace(name) == "" || strings.HasPrefix(name, "-") ||
			filepath.Base(name) != name || strings.ContainsAny(name, "*?") {
			return name
		}
	}
	return ""
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

func containsArgument(args []string, target string) bool {
	for _, value := range args {
		if value == target {
			return true
		}
	}
	return false
}

func printHelp(w io.Writer) {
	fmt.Fprint(w, `Codex Skill Manager

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
  codex review          对指定 --report 运行可选语义复核；支持重复 --skill
  update --group ID     为一个来源创建安全更新计划
  install               从 GitHub URL 或本地目录创建计划；--assist 使用 Codex 辅助安装
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
