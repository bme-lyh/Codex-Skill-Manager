package codexreview

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bme-lyh/Codex-Skill-Manager/internal/model"
)

func TestRunCodexAttemptsRetriesOnce(t *testing.T) {
	attempts := 0
	value, err := runCodexAttempts(2, func(attempt int) (string, error) {
		attempts++
		if attempt == 1 {
			return "", errors.New("transient")
		}
		return "ok", nil
	})
	if err != nil || value != "ok" || attempts != 2 {
		t.Fatalf("expected one retry and success, value=%q err=%v attempts=%d", value, err, attempts)
	}
}

func TestRunCodexAttemptsStopsAfterConfiguredLimit(t *testing.T) {
	attempts := 0
	_, err := runCodexAttempts(2, func(int) (string, error) {
		attempts++
		return "", errors.New("still failing")
	})
	if err == nil || attempts != 2 {
		t.Fatalf("expected two attempts and final error, err=%v attempts=%d", err, attempts)
	}
}

func TestCodexRunnerTimeoutErrorUsesBoundedAttemptMessage(t *testing.T) {
	deadlineCtx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	runner := codexRunner{cfg: model.CodexReviewConfig{TimeoutSeconds: 3}}
	err := runner.commandError(
		deadlineCtx,
		errors.New("process exited"),
		nil,
		codexRunOptions{Label: "test", TimeoutMessage: "test timed out"},
		"",
		"",
	)
	if err == nil || err.Error() != "test timed out" {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func TestCodexRunnerRejectsOversizedOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "result.json")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 12)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readBoundedOutput(path, 8); err == nil {
		t.Fatal("expected bounded output error")
	}
}

func TestCodexRunnerValidatesStrictSchemaBeforeExecution(t *testing.T) {
	workDir := t.TempDir()
	schemaPath := filepath.Join(workDir, "schema.json")
	invalidSchema := []byte(`{"type":"object","properties":{"value":{"type":"string"}},"required":[]}`)
	if err := os.WriteFile(schemaPath, invalidSchema, 0o600); err != nil {
		t.Fatal(err)
	}
	runner := codexRunner{path: filepath.Join(workDir, "missing-codex"), cfg: model.CodexReviewConfig{TimeoutSeconds: 1}}
	_, err := runner.run(context.Background(), codexRunOptions{
		WorkDir: workDir, OutputName: "result.json", SchemaPath: schemaPath,
		Payload: []byte(`{}`), Label: "test",
	})
	if err == nil || !strings.Contains(err.Error(), "output schema") {
		t.Fatalf("expected strict schema validation error, got %v", err)
	}
}

func TestCodexRunnerBoundsDiagnosticsAndKeepsLatestBytes(t *testing.T) {
	message := boundedDiagnosticMessage(strings.Repeat("a", maxCodexDiagnosticMessage+20))
	if len(message) > maxCodexDiagnosticMessage+40 || !strings.Contains(message, "earlier CLI diagnostics omitted") {
		t.Fatalf("diagnostic was not bounded: len=%d", len(message))
	}
	joined := joinDiagnostics("stderr", "stdout")
	if joined != "stderr\nstdout" {
		t.Fatalf("unexpected joined diagnostics: %q", joined)
	}
}
