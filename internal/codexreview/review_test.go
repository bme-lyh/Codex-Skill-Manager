package codexreview

import (
	"context"
	"encoding/json"
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
	skills, err := discoverReviewSkills(root, clusters, []string{"beta"})
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 || skills[0].Name != "beta" {
		t.Fatalf("expected only requested beta Skill, got %#v", skills)
	}
	if len(skills[0].Clusters) != 1 || skills[0].Clusters[0].ID != "risk-beta" {
		t.Fatalf("expected beta cluster assignment, got %#v", skills[0].Clusters)
	}
}

func TestDiscoverReviewSkillsRejectsUnknownRequestedSkill(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("---\nname: known\ndescription: fixture\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := discoverReviewSkills(root, nil, []string{"missing"}); err == nil {
		t.Fatal("expected unknown requested Skill to fail")
	}
}

func TestSplitReviewSkillsUsesBoundedStableBatches(t *testing.T) {
	skills := []reviewSkill{{Name: "a"}, {Name: "b"}, {Name: "c"}, {Name: "d"}, {Name: "e"}}
	batches := splitReviewSkills(skills, 2)
	if len(batches) != 3 {
		t.Fatalf("expected three batches, got %d", len(batches))
	}
	if got := reviewSkillNames(batches[1]); !slices.Equal(got, []string{"c", "d"}) {
		t.Fatalf("unexpected stable batch: %v", got)
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
		[]reviewSkill{{Name: "alpha", SourcePath: ".", FileCount: 1}},
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

func TestProgressTrackerPreservesParallelBatchGroups(t *testing.T) {
	var latest model.CodexReviewProgress
	tracker := &progressTracker{
		progress: func(progress model.CodexReviewProgress) { latest = progress },
		reviewID: "review", reportID: "report", startedAt: time.Now().UTC(),
		batchCount: 2, totalSkills: 3, active: map[int][]string{},
	}
	tracker.startBatch(1, []string{"gamma"})
	tracker.startBatch(0, []string{"alpha", "beta"})
	if len(latest.ActiveBatches) != 2 || latest.ActiveBatches[0].Index != 1 ||
		!slices.Equal(latest.ActiveBatches[0].SkillNames, []string{"alpha", "beta"}) ||
		latest.ActiveBatches[1].Index != 2 {
		t.Fatalf("unexpected grouped progress: %#v", latest.ActiveBatches)
	}
}

func TestCompactReviewSkillsOmitsFingerprintsAndCapsFileLists(t *testing.T) {
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
	if strings.Contains(string(data), "fingerprint") {
		t.Fatalf("compact prompt must omit fingerprints: %s", data)
	}
	if len(compact[0].RiskClusters[0].AffectedFiles) != 50 {
		t.Fatalf("expected affected file cap, got %d", len(compact[0].RiskClusters[0].AffectedFiles))
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
