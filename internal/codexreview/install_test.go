package codexreview

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bme-lyh/Codex-Skill-Manager/internal/model"
)

func TestFinalizeAssistedInstallPlanForcesTransactionalSkillInstall(t *testing.T) {
	candidate := assistedTestCandidate("review-pr")
	plan := assistedTestPlan(candidate)
	plan.Steps = []model.AssistedInstallStep{{
		ID: "install-skills", Kind: model.AssistedInstallStepInstallSkills,
		Title: "Install Skills", Description: "Install the discovered Skill.",
		SkillNames: []string{candidate.Name}, Required: false,
	}}

	finalized, err := FinalizeAssistedInstallPlan(plan, []model.CandidateSkill{candidate}, "missing")
	if err != nil {
		t.Fatal(err)
	}
	if !finalized.Steps[0].Required || !finalized.Steps[0].Supported ||
		finalized.Steps[0].Status != "planned" {
		t.Fatalf("Skill installation was not made a required supported step: %#v", finalized.Steps[0])
	}
	if len(finalized.Permissions) != 1 ||
		finalized.Permissions[0].ID != model.AssistedInstallPermissionSkillsWrite ||
		!finalized.Permissions[0].Required {
		t.Fatalf("unexpected derived permissions: %#v", finalized.Permissions)
	}
}

func TestFinalizeAssistedInstallPlanRejectsDuplicateSkillActions(t *testing.T) {
	candidate := assistedTestCandidate("review-pr")
	plan := assistedTestPlan(candidate)
	plan.Steps = []model.AssistedInstallStep{
		{
			ID: "install-one", Kind: model.AssistedInstallStepInstallSkills,
			Title: "Install one", Description: "First duplicate.",
			SkillNames: []string{candidate.Name},
		},
		{
			ID: "install-two", Kind: model.AssistedInstallStepInstallSkills,
			Title: "Install two", Description: "Second duplicate.",
			SkillNames: []string{candidate.Name},
		},
	}
	if _, err := FinalizeAssistedInstallPlan(
		plan,
		[]model.CandidateSkill{candidate},
		"missing",
	); err == nil || !strings.Contains(err.Error(), "more than once") {
		t.Fatalf("expected duplicate Skill rejection, got %v", err)
	}
}

func TestFinalizeAssistedInstallPlanConsolidatesNonOverlappingSkillActions(t *testing.T) {
	alpha := assistedTestCandidate("review-pr")
	beta := assistedTestCandidate("review-delta")
	candidates := []model.CandidateSkill{alpha, beta}
	plan := assistedTestPlan(alpha)
	plan.OutputLocale = "en-US"
	plan.Skills = candidates
	plan.Steps = []model.AssistedInstallStep{
		{
			ID: "install-alpha", Kind: model.AssistedInstallStepInstallSkills,
			Title: "Install alpha", Description: "Install the first Skill.",
			SkillNames: []string{alpha.Name},
		},
		{
			ID: "install-beta", Kind: model.AssistedInstallStepInstallSkills,
			Title: "Install beta", Description: "Install the second Skill.",
			SkillNames: []string{beta.Name},
		},
	}

	finalized, err := FinalizeAssistedInstallPlan(plan, candidates, "missing")
	if err != nil {
		t.Fatal(err)
	}
	if finalized.Status != "ready" {
		t.Fatalf("redundant merged install action should not require manual work: %s", finalized.Status)
	}
	if finalized.Steps[0].Kind != model.AssistedInstallStepInstallSkills ||
		!finalized.Steps[0].Supported ||
		!slices.Equal(finalized.Steps[0].SkillNames, []string{beta.Name, alpha.Name}) {
		t.Fatalf("candidate Skills were not consolidated into one automatic step: %#v", finalized.Steps)
	}
	if finalized.Steps[1].Kind != model.AssistedInstallStepManual ||
		finalized.Steps[1].Required ||
		finalized.Steps[1].Supported {
		t.Fatalf("extra install action was not made a non-required manual record: %#v", finalized.Steps[1])
	}
	if !containsAssistedWarning(finalized.Warnings, "install-beta", "install-alpha") {
		t.Fatalf("consolidation warning is missing step identities: %#v", finalized.Warnings)
	}
}

func TestGeneratedFreeFormCommandBecomesBlockingManualStep(t *testing.T) {
	step, warning, err := generatedActionToStep(generatedInstallAction{
		ID: "run-installer", Kind: "command", Title: "Run installer",
		Description: "Execute the repository installer.", Required: false,
		Command: "python", RawArgs: []string{"install.py"},
	}, 0, "en-US")
	if err != nil {
		t.Fatal(err)
	}
	if step.Kind != model.AssistedInstallStepManual || step.Supported || !step.Required {
		t.Fatalf("unsafe action was not downgraded: %#v", step)
	}
	if warning == "" {
		t.Fatal("expected a user-facing downgrade warning")
	}
}

func TestGeneratedPathOrEnvironmentBecomesRequiredManualStep(t *testing.T) {
	tests := []struct {
		name   string
		action generatedInstallAction
	}{
		{
			name: "target path",
			action: generatedInstallAction{
				ID: "custom-target", Kind: model.AssistedInstallStepManagedPythonTool,
				Title: "Install tool", Description: "Install into a custom directory.",
				PythonPackage: "example-tool", VersionSpec: "==1.2.3",
				Entrypoint: "example-tool", TargetPath: `C:\custom`,
			},
		},
		{
			name: "environment",
			action: generatedInstallAction{
				ID: "custom-environment", Kind: model.AssistedInstallStepConfigureCodexMCP,
				Title: "Configure MCP", Description: "Configure with a custom environment.",
				Entrypoint: "example-tool", MCPServerName: "example_tool",
				MCPArgs: []string{"serve"}, Environment: map[string]string{"TOKEN": "untrusted"},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			step, warning, err := generatedActionToStep(test.action, 0, "en-US")
			if err != nil {
				t.Fatal(err)
			}
			if step.Kind != model.AssistedInstallStepManual || step.Supported ||
				!step.Required || step.TargetPath != "" || len(step.PermissionIDs) != 0 ||
				step.PythonPackage != "" || step.Entrypoint != "" ||
				step.MCPServerName != "" || len(step.MCPArgs) != 0 {
				t.Fatalf("path/environment action retained automatic authority: %#v", step)
			}
			if !strings.Contains(warning, "fields were cleared") {
				t.Fatalf("expected a clear downgrade warning, got %q", warning)
			}
		})
	}
}

func TestFinalizeAssistedInstallPlanBindsNativeWheelLockAndPermission(t *testing.T) {
	candidate := assistedTestCandidate("review-pr")
	plan := assistedTestPlan(candidate)
	plan.Steps = []model.AssistedInstallStep{
		{
			ID: "install-skills", Kind: model.AssistedInstallStepInstallSkills,
			Title: "Install Skills", Description: "Install the discovered Skill.",
			SkillNames: []string{candidate.Name},
		},
		{
			ID: "managed-tool", Kind: model.AssistedInstallStepManagedPythonTool,
			Title: "Install managed tool", Description: "Install the reviewed MCP runtime.",
			PythonPackage: "example-tool", VersionSpec: "==1.2.3",
			Entrypoint: "example-tool", Required: true,
			PythonWheels: []model.AssistedPythonWheelLock{
				{
					Name: "example-tool", Version: "1.2.3",
					Filename: "example_tool-1.2.3-py3-none-any.whl",
					SHA256:   strings.Repeat("a", 64), Tags: []string{"py3-none-any"},
				},
				{
					Name: "tree-sitter", Version: "0.25.2",
					Filename: "tree_sitter-0.25.2-cp39-abi3-win_amd64.whl",
					SHA256:   strings.Repeat("b", 64), Native: true,
					Tags: []string{"cp39-abi3-win_amd64"},
				},
			},
		},
	}
	finalized, err := FinalizeAssistedInstallPlan(
		plan,
		[]model.CandidateSkill{candidate},
		"missing",
	)
	if err != nil {
		t.Fatal(err)
	}
	foundNativePermission := false
	for _, permission := range finalized.Permissions {
		if permission.ID != model.AssistedInstallPermissionManagedNativeCode {
			continue
		}
		foundNativePermission = true
		if !permission.Required || permission.Risk != "high" || len(permission.Targets) != 1 ||
			!strings.Contains(permission.Targets[0], "win_amd64") ||
			!strings.Contains(permission.Targets[0], strings.Repeat("b", 64)) {
			t.Fatalf("native permission does not expose the approved identity: %#v", permission)
		}
	}
	if !foundNativePermission {
		t.Fatal("native Wheel lock did not derive the high-risk permission")
	}
	originalDigest := finalized.PlanDigest
	finalized.Steps[1].PythonWheels[1].SHA256 = strings.Repeat("c", 64)
	changedDigest, err := AssistedInstallPlanDigest(finalized)
	if err != nil {
		t.Fatal(err)
	}
	if changedDigest == originalDigest {
		t.Fatal("Wheel SHA256 was not bound into the assisted-install plan digest")
	}
}

func TestFinalizeAssistedInstallPlanLimitsManagedPythonAndMCPActions(t *testing.T) {
	candidate := assistedTestCandidate("review-pr")
	plan := assistedTestPlan(candidate)
	plan.OutputLocale = "en-US"
	plan.Steps = []model.AssistedInstallStep{
		{
			ID: "install-skills", Kind: model.AssistedInstallStepInstallSkills,
			Title: "Install Skills", Description: "Install the discovered Skill.",
			SkillNames: []string{candidate.Name},
		},
		{
			ID: "managed-primary", Kind: model.AssistedInstallStepManagedPythonTool,
			Title: "Install primary tool", Description: "Install the approved runtime.",
			PythonPackage: "primary-tool", VersionSpec: "==1.2.3",
			Entrypoint: "primary-tool", Required: true,
			PythonWheels: []model.AssistedPythonWheelLock{{
				Name: "primary-tool", Version: "1.2.3",
				Filename: "primary_tool-1.2.3-py3-none-any.whl",
				SHA256:   strings.Repeat("a", 64), Tags: []string{"py3-none-any"},
			}},
		},
		{
			ID: "managed-extra", Kind: model.AssistedInstallStepManagedPythonTool,
			Title: "Install extra tool", Description: "Install another runtime.",
			PythonPackage: "extra-tool", VersionSpec: "==4.5.6",
			Entrypoint: "extra-tool", Required: true,
		},
		{
			ID: "mcp-conflict", Kind: model.AssistedInstallStepConfigureCodexMCP,
			Title: "Configure conflicting MCP", Description: "Register the extra runtime.",
			Entrypoint: "extra-tool", MCPServerName: "extra_tool",
			MCPArgs: []string{"serve"}, Required: true,
		},
		{
			ID: "mcp-primary", Kind: model.AssistedInstallStepConfigureCodexMCP,
			Title: "Configure primary MCP", Description: "Register the approved runtime.",
			Entrypoint: "primary-tool", MCPServerName: "primary_tool",
			MCPArgs: []string{"serve"}, Required: true,
		},
		{
			ID: "mcp-extra", Kind: model.AssistedInstallStepConfigureCodexMCP,
			Title: "Configure duplicate MCP", Description: "Register the runtime twice.",
			Entrypoint: "primary-tool", MCPServerName: "primary_tool_duplicate",
			MCPArgs: []string{"serve"}, Required: true,
		},
	}

	finalized, err := FinalizeAssistedInstallPlan(
		plan,
		[]model.CandidateSkill{candidate},
		"missing",
	)
	if err != nil {
		t.Fatal(err)
	}
	automaticByKind := map[string][]string{}
	manualIDs := make([]string, 0)
	for _, step := range finalized.Steps {
		if step.Kind == model.AssistedInstallStepManual {
			manualIDs = append(manualIDs, step.ID)
			continue
		}
		if step.Supported {
			automaticByKind[step.Kind] = append(automaticByKind[step.Kind], step.ID)
		}
	}
	if !slices.Equal(
		automaticByKind[model.AssistedInstallStepInstallSkills],
		[]string{"install-skills"},
	) || !slices.Equal(
		automaticByKind[model.AssistedInstallStepManagedPythonTool],
		[]string{"managed-primary"},
	) || !slices.Equal(
		automaticByKind[model.AssistedInstallStepConfigureCodexMCP],
		[]string{"mcp-primary"},
	) {
		t.Fatalf("automatic action cardinality was not enforced: %#v", automaticByKind)
	}
	if !slices.Equal(manualIDs, []string{"managed-extra", "mcp-conflict", "mcp-extra"}) {
		t.Fatalf("conflicting integration steps were not made manual: %#v", finalized.Steps)
	}
	if !containsAssistedWarning(finalized.Warnings, "managed-extra", "managed-primary") ||
		!containsAssistedWarning(finalized.Warnings, "mcp-conflict", "managed-primary") ||
		!containsAssistedWarning(finalized.Warnings, "mcp-extra", "mcp-primary") {
		t.Fatalf("integration downgrade warnings are incomplete: %#v", finalized.Warnings)
	}
	if !finalized.NeedsProjectRoot {
		t.Fatal("the one retained automatic MCP step did not require a project root")
	}
}

func TestFinalizeAssistedInstallPlanMakesMissingWheelLockManual(t *testing.T) {
	candidate := assistedTestCandidate("review-pr")
	plan := assistedTestPlan(candidate)
	plan.Steps = []model.AssistedInstallStep{
		{
			ID: "install-skills", Kind: model.AssistedInstallStepInstallSkills,
			Title: "Install Skills", Description: "Install the discovered Skill.",
			SkillNames: []string{candidate.Name},
		},
		{
			ID: "managed-tool", Kind: model.AssistedInstallStepManagedPythonTool,
			Title: "Install managed tool", Description: "Install the MCP runtime.",
			PythonPackage: "example-tool", VersionSpec: "==1.2.3",
			Entrypoint: "example-tool", Required: true,
		},
		{
			ID: "configure-mcp", Kind: model.AssistedInstallStepConfigureCodexMCP,
			Title: "Configure MCP", Description: "Register the managed runtime.",
			Entrypoint: "example-tool", MCPServerName: "example_tool",
			MCPArgs: []string{"serve"}, Required: true,
		},
	}
	finalized, err := FinalizeAssistedInstallPlan(
		plan,
		[]model.CandidateSkill{candidate},
		"missing",
	)
	if err != nil {
		t.Fatal(err)
	}
	if finalized.Status != "manual-required" ||
		finalized.Steps[1].Kind != model.AssistedInstallStepManual ||
		finalized.Steps[2].Kind != model.AssistedInstallStepManual {
		t.Fatalf("missing approval-time dependency lock was not made manual: %#v", finalized.Steps)
	}
}

func TestAssistedInstallContextDigestCoversNonSkillRepositoryFiles(t *testing.T) {
	root := t.TempDir()
	writeAssistedTestFile(t, filepath.Join(root, "skills", "one", "SKILL.md"), "---\nname: one\n---\n")
	readme := filepath.Join(root, "README.md")
	writeAssistedTestFile(t, readme, "first")

	before, count, err := AssistedInstallContextDigest(root)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("unexpected context count: %d", count)
	}
	writeAssistedTestFile(t, readme, "second")
	after, _, err := AssistedInstallContextDigest(root)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("non-Skill repository documentation was not bound into the context digest")
	}
	if err := VerifyAssistedInstallContext(root, before); err == nil {
		t.Fatal("expected repository context drift to be rejected")
	}
}

func TestAssistedInstallContextDigestIncludesGeneratedAndVendoredContent(t *testing.T) {
	root := t.TempDir()
	readme := filepath.Join(root, "README.md")
	dependency := filepath.Join(root, "node_modules", "package", "index.js")
	writeAssistedTestFile(t, readme, "stable")
	writeAssistedTestFile(t, dependency, "first")
	before, count, err := AssistedInstallContextDigest(root)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("complete context did not inventory dependency content: %d files", count)
	}
	writeAssistedTestFile(t, dependency, "second")
	after, _, err := AssistedInstallContextDigest(root)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("dependency content was not bound into the trusted context")
	}
}

func TestAssistedInstallInputPackagesContextAndDisablesShell(t *testing.T) {
	root := t.TempDir()
	writeAssistedTestFile(t, filepath.Join(root, "README.md"), "installation guidance")
	binaryPath := filepath.Join(root, "assets", "logo.bin")
	if err := os.MkdirAll(filepath.Dir(binaryPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binaryPath, []byte{0, 1, 2, 3}, 0o600); err != nil {
		t.Fatal(err)
	}
	files, digest, count, err := assistedInstallContextSnapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 || len(files) != 2 || len(digest) != 64 {
		t.Fatalf("unexpected packaged context: count=%d files=%#v digest=%q", count, files, digest)
	}
	if files[0].Path != "README.md" || files[0].Kind != "text" ||
		files[0].Content != "installation guidance" {
		t.Fatalf("text context was not packaged verbatim: %#v", files[0])
	}
	if files[1].Path != "assets/logo.bin" || files[1].Kind != "binary" ||
		files[1].Content != "" || len(files[1].SHA256) != 64 {
		t.Fatalf("binary context metadata is invalid: %#v", files[1])
	}

	preview := model.InstallPreview{
		Repository: model.Repository{Provider: "local", FullName: "local:test"},
		Skills:     []model.CandidateSkill{assistedTestCandidate("review-pr")},
	}
	payload, err := buildInstallAnalysisInput(preview, "en", files)
	if err != nil {
		t.Fatal(err)
	}
	var input installAnalysisInput
	if err := json.Unmarshal(payload, &input); err != nil {
		t.Fatal(err)
	}
	if input.ContextMode != "full-repository-packaged-no-tools" ||
		len(input.Files) != 2 || strings.Contains(string(payload), filepath.ToSlash(root)) {
		t.Fatalf("unexpected assisted input: %#v", input)
	}

	args := installReviewArgs(model.CodexReviewConfig{
		Model: "default", ReasoningEffort: "medium",
	}, "schema.json", "result.json")
	execIndex := slices.Index(args, "exec")
	disableIndex := slices.Index(args, "--disable")
	if disableIndex < 0 || disableIndex > execIndex || args[disableIndex+1] != "shell_tool" {
		t.Fatalf("assisted analysis did not disable the shell tool: %v", args)
	}
	if !slices.Contains(args, "shell_snapshot") {
		t.Fatalf("assisted analysis did not disable shell snapshots: %v", args)
	}
}

func TestAssistedInstallOutputSchemaIsStrictCodexCompatible(t *testing.T) {
	if err := validateStrictCodexSchema([]byte(installPlanOutputSchema)); err != nil {
		t.Fatalf("assisted-install schema is not strict-output compatible: %v", err)
	}
}

func TestStrictCodexSchemaRejectsOptionalProperties(t *testing.T) {
	schema := `{
		"type":"object",
		"properties":{"requiredValue":{"type":"string"},"optionalValue":{"type":"string"}},
		"required":["requiredValue"],
		"additionalProperties":false
	}`
	if err := validateStrictCodexSchema([]byte(schema)); err == nil ||
		!strings.Contains(err.Error(), "optionalValue is not required") {
		t.Fatalf("expected an optional-property validation error, got %v", err)
	}
}

func TestCodexJSONLDiagnosticWriterKeepsOnlyFailureMessages(t *testing.T) {
	activityCount := 0
	writer := newCodexJSONLDiagnosticWriter(func() { activityCount++ }, 1024)
	stream := strings.Join([]string{
		`{"type":"thread.started","thread_id":"secret-thread-id"}`,
		`{"type":"item.completed","item":{"type":"agent_message","text":"private repository summary"}}`,
		`{"type":"turn.failed","error":{"message":"Invalid schema for response_format: every property must be required"}}`,
	}, "\n")
	if _, err := writer.Write([]byte(stream)); err != nil {
		t.Fatal(err)
	}
	writer.Flush()
	if activityCount != 2 {
		t.Fatalf("activity count = %d, want 2 completed JSONL records", activityCount)
	}
	diagnostic := writer.String()
	if !strings.Contains(diagnostic, "Invalid schema for response_format") {
		t.Fatalf("failure message was not retained: %q", diagnostic)
	}
	if strings.Contains(diagnostic, "private repository summary") ||
		strings.Contains(diagnostic, "secret-thread-id") {
		t.Fatalf("non-error Codex output leaked into diagnostics: %q", diagnostic)
	}
}

func TestAssistedInstallInputAcceptsLargeComplexRepositoryContext(t *testing.T) {
	root := t.TempDir()
	content := strings.Repeat("package context\n", 62_500)
	for index := 0; index < 5; index++ {
		writeAssistedTestFile(
			t,
			filepath.Join(root, "context", fmt.Sprintf("part-%d.txt", index)),
			content,
		)
	}
	files, _, count, err := assistedInstallContextSnapshot(root)
	if err != nil {
		t.Fatalf("package a repository-sized text context: %v", err)
	}
	if count != 5 {
		t.Fatalf("unexpected packaged context count: %d", count)
	}
	preview := model.InstallPreview{
		Repository: model.Repository{Provider: "local", FullName: "local:large"},
		Skills:     []model.CandidateSkill{assistedTestCandidate("review-pr")},
	}
	payload, err := buildInstallAnalysisInput(preview, "en", files)
	if err != nil {
		t.Fatalf("build repository-sized Codex payload: %v", err)
	}
	if len(payload) >= maxAssistedPromptBytes {
		t.Fatalf("test context unexpectedly reached the hard payload limit: %d", len(payload))
	}
}

func TestAssistedInstallContextDigestRejectsOversizedFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "oversized.bin")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxAssistedContextFileBytes + 1); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := AssistedInstallContextDigest(root); err == nil ||
		!strings.Contains(err.Error(), "file exceeds") {
		t.Fatalf("expected oversized context file rejection, got %v", err)
	}
}

func TestVerifyAssistedInstallPlanIgnoresRuntimeRecoveryState(t *testing.T) {
	candidate := assistedTestCandidate("review-pr")
	plan := assistedTestPlan(candidate)
	plan.Steps = []model.AssistedInstallStep{{
		ID: "install-skills", Kind: model.AssistedInstallStepInstallSkills,
		Title: "Install Skills", Description: "Install the discovered Skill.",
		SkillNames: []string{candidate.Name},
	}}
	finalized, err := FinalizeAssistedInstallPlan(plan, []model.CandidateSkill{candidate}, "missing")
	if err != nil {
		t.Fatal(err)
	}
	finalized.Status = "running"
	finalized.TransactionID = "tx-test"
	finalized.RecoveryStatus = "available"
	finalized.Steps[0].Status = "completed"
	finalized.Steps[0].TargetPath = `C:\managed\skill`
	finalized.Steps[0].BackupPath = `C:\backup\skill`
	finalized.Steps[0].ManifestPath = `C:\managed\manifest.json`
	finalized.Steps[0].ChildTransactionID = "tx-child"
	finalized.Steps[0].OutputHashes = map[string]string{"file": strings.Repeat("a", 64)}
	finalized.Steps[0].AppliedHash = strings.Repeat("b", 64)
	now := time.Now().UTC()
	finalized.Steps[0].CompletedAt = &now
	finalized.Permissions[0].Approved = true
	if err := VerifyAssistedInstallPlan(
		finalized,
		[]model.CandidateSkill{candidate},
		"missing",
	); err != nil {
		t.Fatalf("runtime state invalidated approved semantics: %v", err)
	}
	if finalized.Status != "running" ||
		finalized.Steps[0].Status != "completed" ||
		finalized.Steps[0].TargetPath != `C:\managed\skill` ||
		finalized.Steps[0].OutputHashes["file"] != strings.Repeat("a", 64) ||
		!finalized.Permissions[0].Approved {
		t.Fatalf("verification mutated the caller's runtime state: %#v", finalized)
	}
	finalized.Steps[0].Description = "tampered semantics"
	if err := VerifyAssistedInstallPlan(
		finalized,
		[]model.CandidateSkill{candidate},
		"missing",
	); err == nil {
		t.Fatal("semantic plan tampering was not rejected")
	}
}

func TestAssistedInstallPlanDigestDoesNotReorderCallerSlices(t *testing.T) {
	plan := assistedTestPlan(assistedTestCandidate("review-pr"))
	plan.Steps = []model.AssistedInstallStep{{
		ID:            "install-skills",
		Kind:          model.AssistedInstallStepInstallSkills,
		Title:         "Install Skills",
		Description:   "Install the discovered Skills.",
		SkillNames:    []string{"zeta", "alpha"},
		PermissionIDs: []string{"write-zeta", "write-alpha"},
	}}
	plan.Permissions = []model.AssistedInstallPermission{{
		ID:      "permission",
		Targets: []string{"zeta", "alpha"},
	}}

	if _, err := AssistedInstallPlanDigest(plan); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(plan.Steps[0].SkillNames, []string{"zeta", "alpha"}) ||
		!slices.Equal(plan.Steps[0].PermissionIDs, []string{"write-zeta", "write-alpha"}) ||
		!slices.Equal(plan.Permissions[0].Targets, []string{"zeta", "alpha"}) {
		t.Fatalf("digest calculation reordered caller-owned slices: %#v", plan)
	}
}

func assistedTestCandidate(name string) model.CandidateSkill {
	return model.CandidateSkill{
		Name: name, Description: "test Skill", SourcePath: "skills/" + name,
		Files: []model.FileRecord{{
			Path: "SKILL.md", Size: 16, SHA256: strings.Repeat("1", 64), Kind: "markdown",
		}},
	}
}

func assistedTestPlan(candidate model.CandidateSkill) model.AssistedInstallPlan {
	now := time.Now().UTC()
	return model.AssistedInstallPlan{
		ID: "assisted-plan-test", SourcePlanID: "plan-test", Status: "analyzing",
		Repository: model.Repository{
			Provider: "local", FullName: "local:test", LocalPath: `C:\test`,
			ResolvedRef: "local",
		},
		Summary: "Repository overview", Approach: "Install the discovered Skills.",
		Complexity: "simple", Skills: []model.CandidateSkill{candidate},
		ContextDigest: strings.Repeat("0", 64), ContextFileCount: 1,
		CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	}
}

func containsAssistedWarning(warnings []string, fragments ...string) bool {
	for _, warning := range warnings {
		matched := true
		for _, fragment := range fragments {
			if !strings.Contains(warning, fragment) {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func writeAssistedTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
