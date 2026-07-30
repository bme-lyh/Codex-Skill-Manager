package manager

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

const missingFileFingerprint = "missing"
const maxCodexConfigBytes = int64(8 << 20)

var managedMCPName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)

type mcpMutation struct {
	ConfigPath      string
	BackupPath      string
	AppliedHash     string
	OriginalMissing bool
	ManifestPath    string
}

type managedMCPManifest struct {
	SchemaVersion int       `json:"schemaVersion"`
	ServerName    string    `json:"serverName"`
	Command       string    `json:"command"`
	Args          []string  `json:"args"`
	Cwd           string    `json:"cwd"`
	PlanID        string    `json:"planId"`
	TransactionID string    `json:"transactionId"`
	ConfigHash    string    `json:"configHash"`
	CreatedAt     time.Time `json:"createdAt"`
}

type fileSnapshot struct {
	Content []byte
	Hash    string
	Missing bool
}

type codexMCPServerConfig struct {
	Command string   `toml:"command"`
	Args    []string `toml:"args"`
	Cwd     string   `toml:"cwd"`
}

type codexMCPConfig struct {
	MCPServers map[string]codexMCPServerConfig `toml:"mcp_servers"`
}

func (m *Manager) codexConfigPath() (string, error) {
	root := strings.TrimSpace(os.Getenv("CODEX_HOME"))
	if root != "" {
		if !filepath.IsAbs(root) {
			return "", errors.New("CODEX_HOME must be an absolute path")
		}
		root = filepath.Clean(root)
		if err := rejectSymlinkIfPresent(root, "CODEX_HOME"); err != nil {
			return "", err
		}
		return filepath.Join(root, "config.toml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	root = filepath.Join(home, ".codex")
	if err := rejectSymlinkIfPresent(root, "Codex configuration directory"); err != nil {
		return "", err
	}
	return filepath.Join(root, "config.toml"), nil
}

func fileFingerprint(path string) (string, error) {
	snapshot, err := readFileSnapshot(path)
	if err != nil {
		return "", err
	}
	return snapshot.Hash, nil
}

func readFileSnapshot(path string) (fileSnapshot, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return fileSnapshot{Hash: missingFileFingerprint, Missing: true}, nil
	}
	if err != nil {
		return fileSnapshot{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fileSnapshot{}, errors.New("fingerprinted file must be a regular file, not a symbolic link")
	}
	if info.Size() < 0 || info.Size() > maxCodexConfigBytes {
		return fileSnapshot{}, fmt.Errorf("fingerprinted file exceeds the %d byte limit", maxCodexConfigBytes)
	}
	file, err := os.Open(path)
	if err != nil {
		return fileSnapshot{}, err
	}
	openedInfo, err := file.Stat()
	if err != nil {
		file.Close()
		return fileSnapshot{}, err
	}
	if !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) {
		file.Close()
		return fileSnapshot{}, errors.New("fingerprinted file changed before it could be opened")
	}
	content, readErr := io.ReadAll(io.LimitReader(file, maxCodexConfigBytes+1))
	afterReadInfo, statErr := file.Stat()
	closeErr := file.Close()
	if readErr != nil {
		return fileSnapshot{}, readErr
	}
	if statErr != nil {
		return fileSnapshot{}, statErr
	}
	if closeErr != nil {
		return fileSnapshot{}, closeErr
	}
	if int64(len(content)) > maxCodexConfigBytes {
		return fileSnapshot{}, fmt.Errorf("fingerprinted file exceeds the %d byte limit", maxCodexConfigBytes)
	}
	if int64(len(content)) != openedInfo.Size() ||
		afterReadInfo.Size() != openedInfo.Size() ||
		!afterReadInfo.ModTime().Equal(openedInfo.ModTime()) {
		return fileSnapshot{}, errors.New("fingerprinted file changed while it was being read")
	}
	currentInfo, err := os.Lstat(path)
	if err != nil || currentInfo.Mode()&os.ModeSymlink != 0 ||
		!currentInfo.Mode().IsRegular() || !os.SameFile(openedInfo, currentInfo) {
		if err != nil {
			return fileSnapshot{}, err
		}
		return fileSnapshot{}, errors.New("fingerprinted file was replaced while it was being read")
	}
	sum := sha256.Sum256(content)
	return fileSnapshot{
		Content: content,
		Hash:    hex.EncodeToString(sum[:]),
	}, nil
}

func (m *Manager) configureManagedMCP(
	planID, transactionID, serverName, command string,
	args []string,
	projectRoot, expectedConfigHash string,
	checkpoint func(mcpMutation) error,
) (mcpMutation, error) {
	if !managedMCPName.MatchString(serverName) {
		return mcpMutation{}, errors.New("Codex MCP server name is invalid")
	}
	if !filepath.IsAbs(command) {
		return mcpMutation{}, errors.New("managed MCP command must be an absolute path")
	}
	toolsRoot := filepath.Join(m.Config.Paths.DataRoot, "tools")
	if err := ensureResolvedWithin(toolsRoot, command); err != nil {
		return mcpMutation{}, errors.New("managed MCP command is outside the application tools directory")
	}
	info, err := os.Lstat(command)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return mcpMutation{}, errors.New("managed MCP executable is unavailable")
	}
	if len(args) != 1 || args[0] != "serve" {
		return mcpMutation{}, errors.New("only the validated managed MCP serve action can be configured")
	}
	approvedProjectRoot := filepath.Clean(projectRoot)
	projectRoot, err = validateAssistedProjectRoot(projectRoot)
	if err != nil {
		return mcpMutation{}, err
	}
	if !strings.EqualFold(projectRoot, approvedProjectRoot) {
		return mcpMutation{}, errors.New(
			"target project directory changed after approval; create and approve a new installation plan",
		)
	}
	configPath, err := m.codexConfigPath()
	if err != nil {
		return mcpMutation{}, err
	}
	originalSnapshot, err := readFileSnapshot(configPath)
	if err != nil {
		return mcpMutation{}, err
	}
	if expectedConfigHash == "" {
		return mcpMutation{}, errors.New("Codex configuration fingerprint is missing from the approved plan")
	}
	if originalSnapshot.Hash != expectedConfigHash {
		return mcpMutation{}, errors.New("Codex configuration changed after analysis; review a new installation plan")
	}
	content := originalSnapshot.Content
	originalMissing := originalSnapshot.Missing
	existingConfig, err := parseCodexMCPConfig(content)
	if err != nil {
		return mcpMutation{}, fmt.Errorf("parse existing Codex configuration: %w", err)
	}
	if _, exists := existingConfig.MCPServers[serverName]; exists {
		return mcpMutation{}, fmt.Errorf(
			"Codex MCP server %q already exists and is not owned by this installation plan; no configuration was changed",
			serverName,
		)
	}

	backupPath := filepath.Join(
		m.Config.Paths.BackupsRoot, "_transactions", transactionID, "codex-config.toml",
	)
	manifestPath := filepath.Join(
		m.Config.Paths.DataRoot, "integrations", "mcp", strings.ToLower(serverName)+".json",
	)
	if _, err := os.Lstat(manifestPath); err == nil {
		return mcpMutation{}, errors.New(
			"managed MCP ownership record already exists; recover or remove that integration first",
		)
	} else if !errors.Is(err, os.ErrNotExist) {
		return mcpMutation{}, err
	}
	next := appendMCPSection(string(content), serverName, command, args, projectRoot)
	if err := validateWrittenMCPConfig([]byte(next), serverName, command, args, projectRoot); err != nil {
		return mcpMutation{}, fmt.Errorf("validate planned Codex MCP configuration: %w", err)
	}
	sum := sha256.Sum256([]byte(next))
	mutation := mcpMutation{
		ConfigPath:      configPath,
		BackupPath:      backupPath,
		AppliedHash:     hex.EncodeToString(sum[:]),
		OriginalMissing: originalMissing,
		ManifestPath:    manifestPath,
	}
	if originalMissing {
		mutation.BackupPath = ""
	}
	if checkpoint == nil {
		return mcpMutation{}, errors.New("MCP configuration requires a transaction checkpoint")
	}
	if err := checkpoint(mutation); err != nil {
		return mcpMutation{}, fmt.Errorf("persist MCP configuration intent: %w", err)
	}
	checkpointSnapshot, err := readFileSnapshot(configPath)
	if err != nil {
		return mcpMutation{}, fmt.Errorf("recheck Codex configuration after transaction checkpoint: %w", err)
	}
	if checkpointSnapshot.Hash != originalSnapshot.Hash ||
		checkpointSnapshot.Missing != originalSnapshot.Missing {
		return mcpMutation{}, errors.New(
			"Codex configuration changed after the transaction checkpoint; no configuration was overwritten",
		)
	}
	if !originalMissing {
		if err := writeNewFile(backupPath, content, 0o600); err != nil {
			return mcpMutation{}, fmt.Errorf("back up Codex configuration: %w", err)
		}
	}
	if err := atomicWriteFileIfUnchanged(
		configPath,
		[]byte(next),
		0o600,
		originalSnapshot.Hash,
	); err != nil {
		return mcpMutation{}, fmt.Errorf("write Codex MCP configuration: %w", err)
	}
	appliedHash, err := fileFingerprint(configPath)
	if err != nil {
		recoveryErr := restoreMCPConfig(m.Config.Paths.QuarantineRoot, transactionID, mutation)
		return mcpMutation{}, errors.Join(err, recoveryErr)
	}
	if appliedHash != mutation.AppliedHash {
		recoveryErr := restoreMCPConfig(m.Config.Paths.QuarantineRoot, transactionID, mutation)
		return mcpMutation{}, errors.Join(
			errors.New("Codex configuration changed while the MCP entry was being written"),
			recoveryErr,
		)
	}
	written, err := os.ReadFile(configPath)
	if err == nil {
		err = validateWrittenMCPConfig(written, serverName, command, args, projectRoot)
	}
	if err != nil {
		recoveryErr := restoreMCPConfig(m.Config.Paths.QuarantineRoot, transactionID, mutation)
		return mcpMutation{}, errors.Join(
			fmt.Errorf("validate written Codex MCP configuration: %w", err),
			recoveryErr,
		)
	}
	manifest := managedMCPManifest{
		SchemaVersion: 1, ServerName: serverName, Command: command,
		Args: append([]string(nil), args...), Cwd: projectRoot,
		PlanID: planID, TransactionID: transactionID, ConfigHash: appliedHash,
		CreatedAt: time.Now().UTC(),
	}
	if err := writeJSONAtomic(manifestPath, manifest); err != nil {
		restoreErr := restoreMCPConfig(m.Config.Paths.QuarantineRoot, transactionID, mutation)
		if restoreErr != nil {
			return mcpMutation{}, fmt.Errorf(
				"save MCP ownership record: %w; configuration recovery also failed: %v", err, restoreErr,
			)
		}
		return mcpMutation{}, fmt.Errorf("save MCP ownership record: %w", err)
	}
	return mutation, nil
}

func validateAssistedProjectRoot(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" || !filepath.IsAbs(path) {
		return "", errors.New("an absolute target project directory is required for this MCP integration")
	}
	path, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("resolve target project directory: %w", err)
	}
	inputInfo, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("target project directory: %w", err)
	}
	if inputInfo.Mode()&os.ModeSymlink != 0 || !inputInfo.IsDir() {
		return "", errors.New("target project must be a real directory, not a symbolic link")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve real target project directory: %w", err)
	}
	resolved, err = filepath.Abs(filepath.Clean(resolved))
	if err != nil {
		return "", fmt.Errorf("normalize real target project directory: %w", err)
	}
	info, err := os.Lstat(resolved)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("resolved target project is not a real directory")
	}
	hasVCSRoot := false
	if marker, markerErr := os.Lstat(filepath.Join(resolved, ".git")); markerErr == nil {
		if marker.Mode()&os.ModeSymlink != 0 || (!marker.IsDir() && !marker.Mode().IsRegular()) {
			return "", errors.New("target project has an unsafe Git metadata marker")
		}
		hasVCSRoot = true
	} else if !errors.Is(markerErr, os.ErrNotExist) {
		return "", fmt.Errorf("inspect Git metadata marker: %w", markerErr)
	}
	if !hasVCSRoot {
		if marker, markerErr := os.Lstat(filepath.Join(resolved, ".svn")); markerErr == nil {
			if marker.Mode()&os.ModeSymlink != 0 || !marker.IsDir() {
				return "", errors.New("target project has an unsafe SVN metadata marker")
			}
			hasVCSRoot = true
		} else if !errors.Is(markerErr, os.ErrNotExist) {
			return "", fmt.Errorf("inspect SVN metadata marker: %w", markerErr)
		}
	}
	if !hasVCSRoot {
		return "", errors.New("target project must be the root of a Git or SVN working tree")
	}
	return resolved, nil
}

func parseCodexMCPConfig(content []byte) (codexMCPConfig, error) {
	config := codexMCPConfig{
		MCPServers: map[string]codexMCPServerConfig{},
	}
	if len(strings.TrimSpace(string(content))) == 0 {
		return config, nil
	}
	if _, err := toml.Decode(string(content), &config); err != nil {
		return codexMCPConfig{}, err
	}
	if config.MCPServers == nil {
		config.MCPServers = map[string]codexMCPServerConfig{}
	}
	return config, nil
}

func validateWrittenMCPConfig(
	content []byte,
	serverName string,
	command string,
	args []string,
	projectRoot string,
) error {
	config, err := parseCodexMCPConfig(content)
	if err != nil {
		return err
	}
	server, ok := config.MCPServers[serverName]
	if !ok {
		return fmt.Errorf("MCP server %q is missing after the write", serverName)
	}
	if server.Command != command || server.Cwd != projectRoot ||
		len(server.Args) != len(args) {
		return errors.New("written MCP server values do not match the approved plan")
	}
	for index := range args {
		if server.Args[index] != args[index] {
			return errors.New("written MCP server arguments do not match the approved plan")
		}
	}
	return nil
}

func appendMCPSection(content, serverName, command string, args []string, cwd string) string {
	content = strings.TrimRight(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	var builder strings.Builder
	if content != "" {
		builder.WriteString(content)
		builder.WriteString("\n\n")
	}
	builder.WriteString("[mcp_servers.")
	builder.WriteString(strconv.Quote(serverName))
	builder.WriteString("]\ncommand = ")
	builder.WriteString(strconv.Quote(command))
	builder.WriteString("\nargs = [")
	for index, arg := range args {
		if index > 0 {
			builder.WriteString(", ")
		}
		builder.WriteString(strconv.Quote(arg))
	}
	builder.WriteString("]\ncwd = ")
	builder.WriteString(strconv.Quote(cwd))
	builder.WriteString("\n")
	return builder.String()
}

func restoreMCPConfig(quarantineRoot, transactionID string, mutation mcpMutation) error {
	if mutation.ConfigPath == "" || mutation.AppliedHash == "" {
		return nil
	}
	currentHash, err := fileFingerprint(mutation.ConfigPath)
	if err != nil {
		return err
	}
	if currentHash != mutation.AppliedHash {
		return errors.New("Codex configuration changed after installation; automatic recovery refused to overwrite it")
	}
	if mutation.OriginalMissing {
		target := filepath.Join(
			quarantineRoot, "_config", transactionID, filepath.Base(mutation.ConfigPath),
		)
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		if err := os.Rename(mutation.ConfigPath, target); err != nil {
			return err
		}
		return quarantineManifest(mutation.ManifestPath, quarantineRoot, transactionID)
	}
	if mutation.BackupPath == "" {
		return errors.New("Codex configuration backup is missing")
	}
	data, err := os.ReadFile(mutation.BackupPath)
	if err != nil {
		return err
	}
	if err := atomicWriteFile(mutation.ConfigPath, data, 0o600); err != nil {
		return err
	}
	return quarantineManifest(mutation.ManifestPath, quarantineRoot, transactionID)
}

func quarantineManifest(path, quarantineRoot, transactionID string) error {
	if path == "" {
		return nil
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	target := filepath.Join(quarantineRoot, "_integrations", transactionID, filepath.Base(path))
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	return os.Rename(path, target)
}

func atomicWriteFile(path string, data []byte, mode os.FileMode) error {
	return atomicWriteFileIfUnchanged(path, data, mode, "")
}

func atomicWriteFileIfUnchanged(
	path string,
	data []byte,
	mode os.FileMode,
	expectedHash string,
) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(directory, "."+filepath.Base(path)+".csm-tmp-")
	if err != nil {
		return err
	}
	tmp := file.Name()
	keepTemporary := false
	defer func() {
		if !keepTemporary {
			_ = os.Remove(tmp)
		}
	}()
	if err := file.Chmod(mode); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if expectedHash != "" {
		currentHash, err := fileFingerprint(path)
		if err != nil {
			return err
		}
		if currentHash != expectedHash {
			return errors.New(
				"target changed immediately before the atomic replacement; no file was overwritten",
			)
		}
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	keepTemporary = true
	return nil
}

func rejectSymlinkIfPresent(path string, label string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s must not be a symbolic link", label)
	}
	return nil
}

func ensureResolvedWithin(root string, target string) error {
	if err := ensureWithinOrEqual(root, target); err != nil {
		return err
	}
	if err := rejectSymlinkIfPresent(root, "managed tools directory"); err != nil {
		return err
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	resolvedTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		return err
	}
	return ensureWithinOrEqual(resolvedRoot, resolvedTarget)
}

func copySingleFile(source, target string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return writeNewFile(target, data, 0o600)
}

func writeNewFile(target string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

func writeJSONAtomic(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(path, data, 0o600)
}
