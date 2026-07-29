package codexreview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bme-lyh/Codex-Skill-Manager/internal/model"
)

func TestExecutableCandidatesKeepsLaterIndependentCLI(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific CLI discovery")
	}
	protected := t.TempDir()
	independent := t.TempDir()
	for _, path := range []string{
		filepath.Join(protected, "codex.exe"),
		filepath.Join(independent, "codex.cmd"),
	} {
		if err := os.WriteFile(path, []byte("@echo off\r\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	candidates := executableCandidates(
		protected+string(os.PathListSeparator)+independent,
		"windows",
		"",
	)
	if len(candidates) != 2 {
		t.Fatalf("expected both PATH candidates, got %v", candidates)
	}
	if candidates[1] != filepath.Join(independent, "codex.cmd") {
		t.Fatalf("expected later independent CLI candidate, got %v", candidates)
	}
}

func TestExecutableCandidatesAddsDefaultNPMDirectory(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific CLI discovery")
	}
	home := t.TempDir()
	npmDir := filepath.Join(home, "AppData", "Roaming", "npm")
	if err := os.MkdirAll(npmDir, 0o700); err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(npmDir, "codex.cmd")
	if err := os.WriteFile(expected, []byte("@echo off\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	candidates := executableCandidates("", "windows", home)
	if len(candidates) != 1 || candidates[0] != expected {
		t.Fatalf("expected default npm CLI %q, got %v", expected, candidates)
	}
}

func TestReviewArgsPlacesGlobalOptionsBeforeExec(t *testing.T) {
	args := reviewArgs(model.CodexReviewConfig{
		Model: "default", ReasoningEffort: "medium",
	}, "schema.json", "result.json")
	execIndex := slices.Index(args, "exec")
	approvalIndex := slices.Index(args, "--ask-for-approval")
	sandboxIndex := slices.Index(args, "--sandbox")
	ephemeralIndex := slices.Index(args, "--ephemeral")
	jsonIndex := slices.Index(args, "--json")
	if execIndex < 0 {
		t.Fatal("exec subcommand missing")
	}
	if approvalIndex < 0 || approvalIndex > execIndex {
		t.Fatalf("approval option must precede exec: %v", args)
	}
	if sandboxIndex < 0 || sandboxIndex > execIndex {
		t.Fatalf("sandbox option must precede exec: %v", args)
	}
	if ephemeralIndex < execIndex {
		t.Fatalf("exec option must follow exec subcommand: %v", args)
	}
	if jsonIndex < execIndex {
		t.Fatalf("JSON progress output must follow exec subcommand: %v", args)
	}
}

func TestMissingCapabilitiesUsesFlagsNotVersion(t *testing.T) {
	rootHelp := "--config --model --sandbox --ask-for-approval"
	execHelp := "--ephemeral --skip-git-repo-check --ignore-user-config --ignore-rules --json --output-schema --output-last-message"
	if missing := missingCapabilitiesFromHelp(rootHelp, execHelp); len(missing) != 0 {
		t.Fatalf("expected compatible capability set, got %v", missing)
	}
	missing := missingCapabilitiesFromHelp(rootHelp, strings.ReplaceAll(execHelp, "--output-schema", ""))
	if !slices.Contains(missing, "exec --output-schema") {
		t.Fatalf("expected missing output-schema capability, got %v", missing)
	}
}

func TestParseModelCatalogOnlyReturnsListedModels(t *testing.T) {
	options, err := parseModelCatalog([]byte(`{"models":[
		{"slug":"gpt-current","display_name":"GPT Current","description":"Current model","visibility":"list","default_reasoning_level":"medium","supported_reasoning_levels":[{"effort":"low","description":"Fast"},{"effort":"medium","description":"Balanced"}]},
		{"slug":"internal-review","display_name":"Internal","visibility":"hidden"},
		{"slug":"","display_name":"Invalid","visibility":"list"}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(options) != 1 {
		t.Fatalf("expected one listed model, got %#v", options)
	}
	if options[0].Slug != "gpt-current" || options[0].DisplayName != "GPT Current" {
		t.Fatalf("unexpected model: %#v", options[0])
	}
	if len(options[0].ReasoningLevels) != 2 || options[0].ReasoningLevels[1].Effort != "medium" {
		t.Fatalf("unexpected reasoning levels: %#v", options[0].ReasoningLevels)
	}
}

func TestParseModelCatalogRejectsInvalidJSON(t *testing.T) {
	if _, err := parseModelCatalog([]byte(`not-json`)); err == nil {
		t.Fatal("expected invalid JSON error")
	}
}

func TestCountContextFilesUsesCompleteTargetButSkipsManagerOwnedSystemData(t *testing.T) {
	root := t.TempDir()
	for _, relative := range []string{
		"SKILL.md",
		filepath.Join("src", "main.go"),
		filepath.Join(".system", "internal", "SKILL.md"),
		filepath.Join(".csm-backups", "old", "SKILL.md"),
	} {
		path := filepath.Join(root, relative)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	count, err := countContextFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected complete user target without manager-owned directories, got %d files", count)
	}
}

func TestDiscoverReviewSkillsFiltersRequestedAndAssignsClusters(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"alpha", "beta", "node_modules/ignored"} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
		content := "---\nname: " + filepath.Base(path) + "\ndescription: fixture\n---\n"
		if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	clusters := []model.RiskCluster{{
		ID: "risk-beta", AffectedFiles: []string{"beta/scripts/run.ps1"},
	}}
	summaries := []model.ScanSkillSummary{{
		SkillName: "beta", SourcePath: "beta", GroupID: "research", GroupName: "研究",
	}}
	skills, err := discoverReviewSkills(root, summaries, clusters, []string{"beta"})
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 || skills[0].Name != "beta" {
		t.Fatalf("expected only requested beta Skill, got %#v", skills)
	}
	if len(skills[0].Clusters) != 1 || skills[0].Clusters[0].ID != "risk-beta" {
		t.Fatalf("expected beta cluster assignment, got %#v", skills[0].Clusters)
	}
	if skills[0].GroupID != "research" || skills[0].GroupName != "研究" {
		t.Fatalf("expected persisted group assignment, got %#v", skills[0])
	}
}

func TestDiscoverReviewSkillsRejectsUnknownRequestedSkill(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("---\nname: known\ndescription: fixture\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := discoverReviewSkills(root, nil, nil, []string{"missing"}); err == nil {
		t.Fatal("expected unknown requested Skill to fail")
	}
}

func TestGroupReviewSkillsKeepsCompleteGroupsTogether(t *testing.T) {
	skills := []reviewSkill{
		{Name: "b", GroupID: "research", GroupName: "研究"},
		{Name: "a", GroupID: "research", GroupName: "研究"},
		{Name: "c", GroupID: "development", GroupName: "开发"},
	}
	batches := groupReviewSkills(skills)
	if len(batches) != 2 {
		t.Fatalf("expected two group batches, got %d", len(batches))
	}
	var research reviewBatch
	for _, batch := range batches {
		if batch.GroupID == "research" {
			research = batch
		}
	}
	if got := reviewSkillNames(research.Skills); !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("expected complete stable research group, got %v", got)
	}
}

func TestValidateBatchOutputKeepsOnlyTrustedSkillsAndClusters(t *testing.T) {
	batch := []reviewSkill{{
		Name: "alpha", SourcePath: "skills/alpha", FileCount: 7,
		Clusters: []model.RiskCluster{{ID: "known-cluster"}},
	}}
	generated := generatedBatch{SkillReviews: []model.CodexSkillReview{
		{
			SkillName: "alpha", SourcePath: "skills/alpha", Verdict: "review-required",
			ClusterReviews: []model.CodexClusterReview{{ClusterID: "known-cluster"}, {ClusterID: "invented"}},
		},
		{SkillName: "invented", SourcePath: "elsewhere"},
	}}
	reviews := validateBatchOutput(batch, generated)
	if len(reviews) != 1 || reviews[0].SkillName != "alpha" {
		t.Fatalf("expected only trusted Skill output, got %#v", reviews)
	}
	if reviews[0].ContextFileCount != 7 || len(reviews[0].ClusterReviews) != 1 ||
		reviews[0].ClusterReviews[0].ClusterID != "known-cluster" {
		t.Fatalf("expected trusted counts and clusters, got %#v", reviews[0])
	}
}

func TestValidateGeneratedBatchRequiresEverySkillConclusion(t *testing.T) {
	batch := []reviewSkill{
		{Name: "alpha", SourcePath: "alpha"},
		{Name: "beta", SourcePath: "beta"},
	}
	generated := generatedBatch{SkillReviews: []model.CodexSkillReview{{
		SkillName: "alpha", SourcePath: "alpha",
	}}}
	err := validateGeneratedBatch(batch, generated)
	if err == nil || !strings.Contains(err.Error(), "beta") {
		t.Fatalf("expected missing beta to require a retry, got %v", err)
	}
}

func TestUserFacingBatchErrorSummarizesModelRefreshFailure(t *testing.T) {
	err := errors.New("Codex CLI failed to refresh available models: timeout waiting for child process to exit")
	message := userFacingBatchError(err)
	if !strings.Contains(message, "刷新模型目录") || len(message) > 180 {
		t.Fatalf("expected concise model refresh guidance, got %q", message)
	}
}

func TestOutputSchemaIsValidJSON(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal([]byte(outputSchema), &schema); err != nil {
		t.Fatalf("invalid output schema: %v", err)
	}
}

func TestSafeEvidencePathsRejectsAbsoluteTraversalAndDuplicates(t *testing.T) {
	paths := safeEvidencePaths([]string{
		"scripts/run.ps1", "scripts\\run.ps1", "../secret.txt", `C:\Users\demo\secret.txt`, "/etc/passwd",
	})
	if !slices.Equal(paths, []string{"scripts/run.ps1"}) {
		t.Fatalf("unexpected safe evidence paths: %v", paths)
	}
}

func TestRunBatchConsumesJSONLProgressAndStructuredOutput(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("fake Codex command fixture uses a Windows batch file")
	}
	root := t.TempDir()
	work := t.TempDir()
	schemaPath := filepath.Join(work, "schema.json")
	if err := os.WriteFile(schemaPath, []byte(outputSchema), 0o600); err != nil {
		t.Fatal(err)
	}
	fake := filepath.Join(t.TempDir(), "fake-codex.cmd")
	script := `@echo off
setlocal
set "OUT="
:args
if "%~1"=="" goto run
if "%~1"=="--output-last-message" set "OUT=%~2"
shift
goto args
:run
echo {"type":"turn.started"}
> "%OUT%" echo {"skillReviews":[{"skillName":"alpha","sourcePath":".","verdict":"no-material-risk","summary":"ok","confidence":0.9,"concerns":[],"clusterReviews":[]}]}
exit /b 0
`
	if err := os.WriteFile(fake, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	activity := 0
	output, err := runBatch(
		context.Background(),
		fake,
		model.CodexReviewConfig{Model: "default", ReasoningEffort: "medium"},
		root,
		work,
		schemaPath,
		0,
		1,
		reviewBatch{GroupID: "group", GroupName: "Group", Skills: []reviewSkill{{
			Name: "alpha", SourcePath: ".", FileCount: 1,
		}}},
		func() { activity++ },
	)
	if err != nil {
		t.Fatal(err)
	}
	if activity != 1 {
		t.Fatalf("expected one JSONL progress event, got %d", activity)
	}
	if len(output.SkillReviews) != 1 || output.SkillReviews[0].SkillName != "alpha" {
		t.Fatalf("unexpected structured output: %#v", output)
	}
}

func TestReviewRetriesFailedGroupOnceSerially(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("fake Codex command fixture uses a Windows batch file")
	}
	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "SKILL.md"),
		[]byte("---\nname: alpha\ndescription: fixture\n---\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	fixtureRoot := t.TempDir()
	counter := filepath.Join(fixtureRoot, "attempts.txt")
	fake := filepath.Join(fixtureRoot, "fake-codex.cmd")
	script := `@echo off
setlocal
if "%~1"=="--version" (
  echo codex-cli fixture
  exit /b 0
)
if "%~1"=="login" (
  echo Logged in
  exit /b 0
)
if "%~1"=="--help" (
  echo --config --model --sandbox --ask-for-approval
  exit /b 0
)
if "%~1"=="exec" if "%~2"=="--help" (
  echo --ephemeral --skip-git-repo-check --ignore-user-config --ignore-rules --json --output-schema --output-last-message
  exit /b 0
)
set "OUT="
:args
if "%~1"=="" goto run
if "%~1"=="--output-last-message" set "OUT=%~2"
shift
goto args
:run
set "COUNT=0"
if exist "__COUNTER__" set /p COUNT=<"__COUNTER__"
set /a COUNT+=1
> "__COUNTER__" echo %COUNT%
if %COUNT%==1 (
  echo transient model refresh failure 1>&2
  exit /b 1
)
echo {"type":"turn.started"}
> "%OUT%" echo {"skillReviews":[{"skillName":"alpha","sourcePath":".","verdict":"no-material-risk","summary":"ok","confidence":0.9,"concerns":[],"clusterReviews":[]}]}
exit /b 0
`
	script = strings.ReplaceAll(script, "__COUNTER__", counter)
	if err := os.WriteFile(fake, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	report := model.ScanReport{
		ID: "scan-retry", Target: root,
		Skills: []model.ScanSkillSummary{{
			SkillName: "alpha", SourcePath: ".", GroupID: "g1", GroupName: "Group",
		}},
	}
	result, err := reviewInBatches(
		context.Background(),
		model.CodexReviewConfig{
			CLIPath: fake, Model: "default", ReasoningEffort: "medium",
			TimeoutSeconds: 30, MaxParallelBatches: 1,
		},
		report,
		t.TempDir(),
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "completed" || len(result.Batches) != 1 || result.Batches[0].Attempts != 2 {
		t.Fatalf("expected successful second attempt, got %#v", result)
	}
	data, err := os.ReadFile(counter)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) != "2" {
		t.Fatalf("expected exactly two Codex attempts, got %q", data)
	}
}

func TestProgressTrackerPreservesParallelBatchGroups(t *testing.T) {
	var latest model.CodexReviewProgress
	tracker := &progressTracker{
		progress: func(progress model.CodexReviewProgress) { latest = progress },
		reviewID: "review", reportID: "report", startedAt: time.Now().UTC(),
		batchCount: 2, totalSkills: 3, active: map[int]model.CodexActiveBatch{},
	}
	tracker.startBatch(1, reviewBatch{GroupID: "g2", GroupName: "第二组", Skills: []reviewSkill{{Name: "gamma"}}}, 1)
	firstSequence := latest.Sequence
	tracker.startBatch(0, reviewBatch{
		GroupID: "g1", GroupName: "第一组", Skills: []reviewSkill{{Name: "alpha"}, {Name: "beta"}},
	}, 1)
	if len(latest.ActiveBatches) != 2 || latest.ActiveBatches[0].Index != 1 ||
		!slices.Equal(latest.ActiveBatches[0].SkillNames, []string{"alpha", "beta"}) ||
		latest.ActiveBatches[1].Index != 2 {
		t.Fatalf("unexpected grouped progress: %#v", latest.ActiveBatches)
	}
	if latest.Sequence <= firstSequence {
		t.Fatalf("progress sequence must increase monotonically: %d <= %d", latest.Sequence, firstSequence)
	}
}

func TestCompactReviewSkillsUsesOverviewWithoutEvidenceOrFilePaths(t *testing.T) {
	files := make([]string, 60)
	for index := range files {
		files[index] = fmt.Sprintf("src/file-%02d.go", index)
	}
	compact := compactReviewSkills([]reviewSkill{{
		Name: "alpha", SourcePath: "skills/alpha", FileCount: 60,
		Clusters: []model.RiskCluster{{
			ID: "risk", Fingerprints: []string{"sensitive-local-fingerprint"},
			AffectedFiles: files, SampleFindings: []model.Finding{{
				File: "src/main.go", Evidence: "fixture", Fingerprint: "finding-fingerprint",
			}},
		}},
	}})
	data, err := json.Marshal(compact)
	if err != nil {
		t.Fatal(err)
	}
	payload := string(data)
	if strings.Contains(payload, "fingerprint") || strings.Contains(payload, "src/main.go") ||
		strings.Contains(payload, "file-00.go") || strings.Contains(payload, "fixture") {
		t.Fatalf("compact prompt must omit fingerprints, evidence, and file paths: %s", data)
	}
	if len(compact[0].RiskOverview) != 1 || compact[0].RiskOverview[0].AffectedFileCount != 60 {
		t.Fatalf("expected count-only risk overview, got %#v", compact[0].RiskOverview)
	}
}

func TestTrustedReviewRootRejectsSymlinkTarget(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "linked-target")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("creating symlinks is unavailable: %v", err)
	}
	if _, err := trustedReviewRoot(link); err == nil {
		t.Fatal("expected a symlink review target to be rejected")
	}
}
