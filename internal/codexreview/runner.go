package codexreview

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/bme-lyh/Codex-Skill-Manager/internal/model"
	"github.com/bme-lyh/Codex-Skill-Manager/internal/processutil"
)

const (
	defaultCodexAttemptTimeout = 300 * time.Second
	maxCodexDiagnosticBytes    = 64 << 10
	maxCodexDiagnosticMessage  = 4000
	defaultCodexOutputBytes    = 1 << 20
)

// codexRunner owns the common, non-business part of a Codex invocation. The
// caller still constructs and validates the request payload and validates the
// decoded business result. This keeps repository safety decisions and command
// execution policy in one place without granting the model any new authority.
type codexRunner struct {
	path string
	cfg  model.CodexReviewConfig
}

type codexRunOptions struct {
	WorkDir                 string
	OutputName              string
	SchemaPath              string
	Payload                 []byte
	OutputLimit             int64
	Label                   string
	TimeoutMessage          string
	FailureMessage          string
	ProgressMessage         string
	OnActivity              func()
	CaptureJSONLDiagnostics bool
	MaxInputBytes           int
}

type codexRunResult struct {
	Output     []byte
	Diagnostic string
}

// prepareCodexRunner performs the shared executable and authentication
// preflight used by all analysis entry points.
func prepareCodexRunner(ctx context.Context, cfg model.CodexReviewConfig) (codexRunner, error) {
	path, err := reviewPreflight(ctx, cfg.CLIPath)
	if err != nil {
		return codexRunner{}, err
	}
	return codexRunner{path: path, cfg: cfg}, nil
}

func (runner codexRunner) run(ctx context.Context, options codexRunOptions) (codexRunResult, error) {
	if strings.TrimSpace(runner.path) == "" {
		return codexRunResult{}, errors.New("Codex CLI path is required")
	}
	if strings.TrimSpace(options.WorkDir) == "" {
		return codexRunResult{}, errors.New("Codex CLI work directory is required")
	}
	if strings.TrimSpace(options.OutputName) == "" {
		return codexRunResult{}, errors.New("Codex CLI output name is required")
	}
	if filepath.Base(options.OutputName) != options.OutputName {
		return codexRunResult{}, errors.New("Codex CLI output name must be a file name")
	}
	if options.OutputLimit <= 0 {
		options.OutputLimit = defaultCodexOutputBytes
	}
	if options.MaxInputBytes <= 0 {
		options.MaxInputBytes = maxAssistedPromptBytes
	}
	if len(options.Payload) > options.MaxInputBytes {
		return codexRunResult{}, fmt.Errorf(
			"Codex %s input exceeds the %d byte limit",
			strings.TrimSpace(options.Label), options.MaxInputBytes,
		)
	}
	if options.SchemaPath != "" {
		schema, err := os.ReadFile(options.SchemaPath)
		if err != nil {
			return codexRunResult{}, fmt.Errorf("read Codex %s output schema: %w", options.Label, err)
		}
		if err := validateStrictCodexSchema(schema); err != nil {
			return codexRunResult{}, fmt.Errorf("invalid Codex %s output schema: %w", options.Label, err)
		}
	}

	attemptTimeout := time.Duration(runner.cfg.TimeoutSeconds) * time.Second
	if runner.cfg.TimeoutSeconds < 1 {
		attemptTimeout = defaultCodexAttemptTimeout
	}
	attemptCtx, cancel := context.WithTimeout(ctx, attemptTimeout)
	defer cancel()
	outputPath := filepath.Join(options.WorkDir, options.OutputName)
	command := exec.CommandContext(
		attemptCtx,
		runner.path,
		installReviewArgs(runner.cfg, options.SchemaPath, outputPath)...,
	)
	processutil.ConfigureBackground(command)
	command.Dir = options.WorkDir
	command.Env = sanitizedEnvironment()
	command.Stdin = bytes.NewReader(options.Payload)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return codexRunResult{}, err
	}
	diagnostic := newBoundedDiagnosticBuffer(maxCodexDiagnosticBytes)
	command.Stderr = &diagnostic
	if err := command.Start(); err != nil {
		return codexRunResult{}, err
	}
	onActivity := func() {}
	if options.OnActivity != nil {
		onActivity = options.OnActivity
	}
	if options.CaptureJSONLDiagnostics {
		writer := newCodexJSONLDiagnosticWriter(onActivity, maxCodexDiagnosticBytes)
		_, streamErr := io.Copy(writer, stdout)
		writer.Flush()
		waitErr := command.Wait()
		if err := runner.commandError(attemptCtx, waitErr, streamErr, options, diagnostic.String(), writer.String()); err != nil {
			return codexRunResult{}, err
		}
		data, err := readBoundedOutput(outputPath, options.OutputLimit)
		if err != nil {
			return codexRunResult{}, err
		}
		return codexRunResult{Output: data, Diagnostic: strings.TrimSpace(joinDiagnostics(diagnostic.String(), writer.String()))}, nil
	}
	_, streamErr := io.Copy(activityWriter{onActivity: onActivity}, stdout)
	waitErr := command.Wait()
	if err := runner.commandError(attemptCtx, waitErr, streamErr, options, diagnostic.String(), ""); err != nil {
		return codexRunResult{}, err
	}
	data, err := readBoundedOutput(outputPath, options.OutputLimit)
	if err != nil {
		return codexRunResult{}, err
	}
	return codexRunResult{Output: data, Diagnostic: strings.TrimSpace(diagnostic.String())}, nil
}

func (runner codexRunner) commandError(
	attemptCtx context.Context,
	waitErr error,
	streamErr error,
	options codexRunOptions,
	stderr string,
	stdoutDiagnostic string,
) error {
	if waitErr != nil {
		if errors.Is(attemptCtx.Err(), context.DeadlineExceeded) {
			message := options.TimeoutMessage
			if message == "" {
				message = fmt.Sprintf("Codex %s exceeded the %d second attempt limit", options.Label, int(runner.attemptTimeout().Seconds()))
			}
			return errors.New(message)
		}
		message := strings.TrimSpace(joinDiagnostics(stderr, stdoutDiagnostic))
		if message == "" {
			message = waitErr.Error()
		}
		message = boundedDiagnosticMessage(message)
		prefix := options.FailureMessage
		if prefix == "" {
			prefix = fmt.Sprintf("Codex %s failed", options.Label)
		}
		return fmt.Errorf("%s: %s", prefix, message)
	}
	if streamErr != nil {
		message := options.ProgressMessage
		if message == "" {
			message = fmt.Sprintf("read Codex %s progress", options.Label)
		}
		return fmt.Errorf("%s: %w", message, streamErr)
	}
	return nil
}

func (runner codexRunner) attemptTimeout() time.Duration {
	if runner.cfg.TimeoutSeconds < 1 {
		return defaultCodexAttemptTimeout
	}
	return time.Duration(runner.cfg.TimeoutSeconds) * time.Second
}

func boundedDiagnosticMessage(message string) string {
	message = strings.TrimSpace(message)
	if len(message) <= maxCodexDiagnosticMessage {
		return message
	}
	return "(earlier CLI diagnostics omitted)\n" + message[len(message)-maxCodexDiagnosticMessage:]
}

func joinDiagnostics(stderr, stdout string) string {
	stderr = strings.TrimSpace(stderr)
	stdout = strings.TrimSpace(stdout)
	switch {
	case stderr == "":
		return stdout
	case stdout == "":
		return stderr
	default:
		return stderr + "\n" + stdout
	}
}

func decodeCodexJSON[T any](data []byte, label string) (T, error) {
	var value T
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return value, fmt.Errorf("decode Codex %s: %w", label, err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return value, err
	}
	return value, nil
}

func runCodexJSON[T any](ctx context.Context, runner codexRunner, options codexRunOptions, label string) (T, error) {
	result, err := runner.run(ctx, options)
	if err != nil {
		var zero T
		return zero, err
	}
	return decodeCodexJSON[T](result.Output, label)
}

func runCodexAttempts[T any](attempts int, attempt func(int) (T, error)) (T, error) {
	if attempts < 1 {
		attempts = 1
	}
	var value T
	var lastErr error
	for index := 1; index <= attempts; index++ {
		value, lastErr = attempt(index)
		if lastErr == nil {
			return value, nil
		}
	}
	return value, lastErr
}
