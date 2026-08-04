package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bme-lyh/Codex-Skill-Manager/internal/model"
	"gopkg.in/yaml.v3"
)

const appDirName = "skill-manager"

func Default() (model.Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return model.Config{}, err
	}
	data := filepath.Join(home, ".codex", appDirName)
	codexSkills := filepath.Join(home, ".codex", "skills")
	agentsSkills := filepath.Join(home, ".agents", "skills")
	return model.Config{
		SchemaVersion: 2,
		Paths: model.Paths{
			// SkillsRoot is retained as a compatibility view of the default root.
			SkillsRoot:     codexSkills,
			DataRoot:       data,
			LogsRoot:       filepath.Join(data, "logs"),
			ReportsRoot:    filepath.Join(data, "reports"),
			BackupsRoot:    filepath.Join(data, "backups"),
			QuarantineRoot: filepath.Join(data, "quarantine"),
			CacheRoot:      filepath.Join(data, "cache"),
			StagingRoot:    filepath.Join(data, "staging"),
		},
		SkillRoots:    model.DefaultSkillRoots(codexSkills, agentsSkills),
		DefaultRootID: model.DefaultRootID,
		Schedule:      model.Schedule{Enabled: false, Frequency: "weekly", Time: "09:00"},
		Locale:        "zh-CN",
		Theme:         "system",
		GitHubHost:    "github.com",
		MaxFileBytes:  20 << 20,
		MaxFiles:      2000,
		CodexReview: model.CodexReviewConfig{
			Enabled: false, Model: "gpt-5.6-luna", ReasoningEffort: "xhigh",
			TimeoutSeconds: 300, MaxSamplePerRisk: 8,
			MaxParallelBatches: 1,
		},
	}, nil
}

func Load(path string) (model.Config, error) {
	if strings.TrimSpace(path) == "" {
		var err error
		cfg, err := Default()
		if err != nil {
			return model.Config{}, err
		}
		path = filepath.Join(cfg.Paths.DataRoot, "config.yaml")
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		cfg, derr := Default()
		if derr != nil {
			return model.Config{}, derr
		}
		if err := EnsureDirs(cfg); err != nil {
			return model.Config{}, err
		}
		if err := Save(path, cfg); err != nil {
			return model.Config{}, err
		}
		return cfg, nil
	}
	if err != nil {
		return model.Config{}, err
	}
	var cfg model.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return model.Config{}, fmt.Errorf("parse config: %w", err)
	}
	if err := migrate(&cfg); err != nil {
		return model.Config{}, err
	}
	if err := Validate(cfg); err != nil {
		return model.Config{}, err
	}
	return cfg, EnsureDirs(cfg)
}

func Save(path string, cfg model.Config) error {
	if err := migrate(&cfg); err != nil {
		return err
	}
	if err := Validate(cfg); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	persist := cfg
	// `Roots` is an in-memory compatibility alias. Keep the on-disk schema
	// canonical and avoid writing two independently editable root arrays.
	persist.Roots = nil
	data, err := yaml.Marshal(persist)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func Validate(cfg model.Config) error {
	if cfg.SchemaVersion != 1 && cfg.SchemaVersion != 2 {
		return fmt.Errorf("unsupported config schema: %d", cfg.SchemaVersion)
	}
	switch cfg.Locale {
	case "zh-CN", "en-US":
	default:
		return errors.New("locale must be zh-CN or en-US")
	}
	switch cfg.Theme {
	case "system", "light", "dark":
	default:
		return errors.New("theme must be system, light or dark")
	}
	for label, p := range map[string]string{
		"skillsRoot": cfg.Paths.SkillsRoot, "dataRoot": cfg.Paths.DataRoot,
		"logsRoot":    cfg.Paths.LogsRoot,
		"reportsRoot": cfg.Paths.ReportsRoot, "backupsRoot": cfg.Paths.BackupsRoot,
		"quarantineRoot": cfg.Paths.QuarantineRoot, "cacheRoot": cfg.Paths.CacheRoot,
		"stagingRoot": cfg.Paths.StagingRoot,
	} {
		if !filepath.IsAbs(p) {
			return fmt.Errorf("%s must be an absolute path", label)
		}
	}
	if cfg.SchemaVersion >= 2 {
		if err := validateRoots(cfg); err != nil {
			return err
		}
	}
	if strings.EqualFold(filepath.Clean(cfg.Paths.SkillsRoot), filepath.Clean(cfg.Paths.DataRoot)) {
		return errors.New("skillsRoot and dataRoot must differ")
	}
	if cfg.MaxFiles < 1 || cfg.MaxFileBytes < 1 {
		return errors.New("file limits must be positive")
	}
	if cfg.GitHubHost != "github.com" {
		return errors.New("v1 supports github.com only")
	}
	switch cfg.CodexReview.ReasoningEffort {
	case "minimal", "low", "medium", "high", "xhigh", "max", "ultra":
	default:
		return errors.New("codexReview.reasoningEffort must be minimal, low, medium, high, xhigh, max or ultra")
	}
	if cfg.CodexReview.TimeoutSeconds < 30 || cfg.CodexReview.TimeoutSeconds > 1800 {
		return errors.New("codexReview.timeoutSeconds must be between 30 and 1800")
	}
	if cfg.CodexReview.MaxSamplePerRisk < 1 || cfg.CodexReview.MaxSamplePerRisk > 20 {
		return errors.New("codexReview.maxSamplePerRisk must be between 1 and 20")
	}
	if cfg.CodexReview.MaxParallelBatches < 1 || cfg.CodexReview.MaxParallelBatches > 4 {
		return errors.New("codexReview.maxParallelBatches must be between 1 and 4")
	}
	return nil
}

func normalize(cfg *model.Config) {
	if strings.TrimSpace(cfg.Locale) == "" {
		cfg.Locale = "zh-CN"
	}
	if strings.TrimSpace(cfg.Theme) == "" {
		cfg.Theme = "system"
	}
	if strings.TrimSpace(cfg.CodexReview.Model) == "" {
		cfg.CodexReview.Model = "gpt-5.6-luna"
	}
	if strings.TrimSpace(cfg.CodexReview.ReasoningEffort) == "" {
		cfg.CodexReview.ReasoningEffort = "xhigh"
	}
	if cfg.CodexReview.TimeoutSeconds == 0 {
		cfg.CodexReview.TimeoutSeconds = 300
	}
	if cfg.CodexReview.MaxSamplePerRisk == 0 {
		cfg.CodexReview.MaxSamplePerRisk = 8
	}
	if cfg.CodexReview.MaxParallelBatches == 0 {
		cfg.CodexReview.MaxParallelBatches = 1
	}
}

// Roots returns the normalized, enabled/disabled root definitions.  It is a
// value-returning helper so callers cannot mutate config through a shared
// slice.  The compatibility SkillsRoot path is reflected as codex-default.
func Roots(cfg model.Config) []model.SkillRoot {
	copyRoots := cfg.SkillRoots
	if len(copyRoots) == 0 {
		copyRoots = cfg.Roots
	}
	out := make([]model.SkillRoot, len(copyRoots))
	copy(out, copyRoots)
	return out
}

func Root(cfg model.Config, rootID string) (model.SkillRoot, bool) {
	for _, root := range Roots(cfg) {
		if root.ID == rootID {
			return root, true
		}
	}
	return model.SkillRoot{}, false
}

// EnsureSkillRoot is the explicit mutation boundary for root creation.  Load,
// Save, and EnsureDirs intentionally do not create any configured Skill root.
func EnsureSkillRoot(cfg model.Config, rootID string) (model.SkillRoot, error) {
	root, ok := Root(cfg, rootID)
	if !ok {
		return model.SkillRoot{}, fmt.Errorf("unknown skill root: %s", rootID)
	}
	if !root.Enabled {
		return model.SkillRoot{}, fmt.Errorf("skill root is disabled: %s", rootID)
	}
	if root.ReadOnly {
		return model.SkillRoot{}, fmt.Errorf("skill root is read-only: %s", rootID)
	}
	if err := os.MkdirAll(root.Path, 0o700); err != nil {
		return model.SkillRoot{}, err
	}
	return root, nil
}

// EnsureRoot is a concise alias used by manager integrations.
func EnsureRoot(cfg model.Config, rootID string) (model.SkillRoot, error) {
	return EnsureSkillRoot(cfg, rootID)
}

// SkillTarget resolves one explicit child target without creating it. It is
// the companion validation used by manager mutation plans.
func SkillTarget(root model.SkillRoot, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name || strings.ContainsAny(name, `/\\`) {
		return "", errors.New("skill target name must be one explicit child name")
	}
	if strings.EqualFold(name, filepath.Base(filepath.Clean(model.RootSystemDir(root)))) || root.ReadOnly {
		return "", errors.New("system skill targets are read-only")
	}
	return filepath.Join(root.Path, name), nil
}

func ResolveSkillTarget(cfg model.Config, rootID, name string) (string, error) {
	root, ok := Root(cfg, rootID)
	if !ok {
		return "", fmt.Errorf("unknown skill root: %s", rootID)
	}
	return SkillTarget(root, name)
}

func migrate(cfg *model.Config) error {
	if cfg == nil {
		return errors.New("config is nil")
	}
	legacy := cfg.SchemaVersion == 0 || cfg.SchemaVersion == 1
	if cfg.Paths.LogsRoot == "" && cfg.Paths.DataRoot != "" {
		cfg.Paths.LogsRoot = filepath.Join(cfg.Paths.DataRoot, "logs")
	}
	if legacy {
		// The old SkillsRoot is retained as codex-default.  No Skill content is
		// moved or rewritten; only the configuration representation changes.
		if len(cfg.SkillRoots) == 0 && len(cfg.Roots) == 0 {
			codexPath := strings.TrimSpace(cfg.Paths.SkillsRoot)
			if codexPath == "" {
				defaults, err := Default()
				if err != nil {
					return err
				}
				codexPath = defaults.Paths.SkillsRoot
			}
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			defaultCodexPath := filepath.Join(home, ".codex", "skills")
			if strings.EqualFold(filepath.Clean(codexPath), filepath.Clean(defaultCodexPath)) {
				cfg.SkillRoots = model.DefaultSkillRoots(codexPath, filepath.Join(home, ".agents", "skills"))
			} else {
				// A legacy custom root must not silently add a live user directory.
				// Register the Agents root for schema stability, but leave it disabled
				// until the user explicitly enables it in this custom configuration.
				cfg.SkillRoots = model.DefaultSkillRoots(codexPath, filepath.Join(home, ".agents", "skills"))
				cfg.SkillRoots[1].Enabled = false
			}
		}
		cfg.SchemaVersion = 2
	}
	normalize(cfg)
	if len(cfg.SkillRoots) == 0 && len(cfg.Roots) == 0 {
		defaults, err := Default()
		if err != nil {
			return err
		}
		cfg.SkillRoots = defaults.SkillRoots
	}
	if len(cfg.SkillRoots) == 0 {
		cfg.SkillRoots = append([]model.SkillRoot(nil), cfg.Roots...)
	}
	if len(cfg.Roots) == 0 {
		cfg.Roots = append([]model.SkillRoot(nil), cfg.SkillRoots...)
	}
	if cfg.DefaultRootID == "" {
		cfg.DefaultRootID = model.DefaultRootID
	}
	for i := range cfg.SkillRoots {
		if cfg.SkillRoots[i].ID == model.RootIDCodexDefault && cfg.Paths.SkillsRoot == "" {
			cfg.Paths.SkillsRoot = cfg.SkillRoots[i].Path
		}
	}
	if cfg.Paths.SkillsRoot == "" {
		if root, ok := Root(*cfg, cfg.DefaultRootID); ok {
			cfg.Paths.SkillsRoot = root.Path
		}
	}
	for i := range cfg.SkillRoots {
		if strings.TrimSpace(cfg.SkillRoots[i].SystemDir) == "" {
			cfg.SkillRoots[i].SystemDir = ".system"
		}
		if strings.TrimSpace(cfg.SkillRoots[i].Kind) == "" {
			switch cfg.SkillRoots[i].ID {
			case model.RootIDAgents:
				cfg.SkillRoots[i].Kind = "agents"
			default:
				cfg.SkillRoots[i].Kind = "codex"
			}
		}
		if strings.TrimSpace(cfg.SkillRoots[i].Name) == "" {
			cfg.SkillRoots[i].Name = cfg.SkillRoots[i].ID
		}
	}
	// Keep the two in-memory spellings synchronized.  Save emits only the
	// canonical SkillRoots field to avoid duplicate YAML sections.
	cfg.Roots = append(cfg.Roots[:0], cfg.SkillRoots...)
	return nil
}

func validateRoots(cfg model.Config) error {
	roots := Roots(cfg)
	if len(roots) < 2 {
		return errors.New("schema v2 requires codex-default and agents skill roots")
	}
	if cfg.DefaultRootID == "" {
		return errors.New("defaultRootID is required")
	}
	seenID := map[string]bool{}
	seenPath := map[string]string{}
	foundDefault := false
	foundCodex, foundAgents := false, false
	for _, root := range roots {
		id := strings.TrimSpace(root.ID)
		if id == "" {
			return errors.New("skill root id is required")
		}
		if seenID[id] {
			return fmt.Errorf("duplicate skill root id: %s", id)
		}
		seenID[id] = true
		if !filepath.IsAbs(root.Path) {
			return fmt.Errorf("skill root %s path must be absolute", id)
		}
		if strings.TrimSpace(root.SystemDir) == "" {
			return fmt.Errorf("skill root %s systemDir is required", id)
		}
		if !strings.EqualFold(filepath.Clean(root.SystemDir), ".system") {
			return fmt.Errorf("skill root %s systemDir must be .system", id)
		}
		clean := filepath.Clean(root.Path)
		key := strings.ToLower(clean)
		if prior, ok := seenPath[key]; ok && prior != id {
			return fmt.Errorf("skill root paths are ambiguous: %s and %s", prior, id)
		}
		seenPath[key] = id
		switch id {
		case model.RootIDCodexDefault:
			foundCodex = true
		case model.RootIDAgents:
			foundAgents = true
		}
		if id == cfg.DefaultRootID {
			foundDefault = true
		}
	}
	if !foundCodex || !foundAgents {
		return errors.New("schema v2 requires codex-default and agents skill roots")
	}
	if !foundDefault {
		return fmt.Errorf("defaultRootID %q is not configured", cfg.DefaultRootID)
	}
	for i := range roots {
		for j := i + 1; j < len(roots); j++ {
			if pathsOverlap(roots[i].Path, roots[j].Path) {
				return fmt.Errorf("skill root paths overlap: %s and %s", roots[i].ID, roots[j].ID)
			}
		}
		managedPaths := map[string]string{
			"dataRoot": cfg.Paths.DataRoot, "logsRoot": cfg.Paths.LogsRoot,
			"reportsRoot": cfg.Paths.ReportsRoot, "backupsRoot": cfg.Paths.BackupsRoot,
			"quarantineRoot": cfg.Paths.QuarantineRoot, "cacheRoot": cfg.Paths.CacheRoot,
			"stagingRoot": cfg.Paths.StagingRoot,
		}
		for label, managedPath := range managedPaths {
			if pathsOverlap(roots[i].Path, managedPath) {
				return fmt.Errorf("skill root %s overlaps %s", roots[i].ID, label)
			}
		}
	}
	return nil
}

func pathsOverlap(left, right string) bool {
	left, right = filepath.Clean(left), filepath.Clean(right)
	for _, pair := range [][2]string{{left, right}, {right, left}} {
		rel, err := filepath.Rel(pair[0], pair[1])
		if err == nil && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))) {
			return true
		}
	}
	return false
}

func isSystemPath(path string) bool {
	return strings.EqualFold(filepath.Base(filepath.Clean(path)), ".system")
}

func EnsureDirs(cfg model.Config) error {
	for _, p := range []string{
		cfg.Paths.DataRoot, cfg.Paths.LogsRoot, cfg.Paths.ReportsRoot, cfg.Paths.BackupsRoot,
		cfg.Paths.QuarantineRoot, cfg.Paths.CacheRoot, cfg.Paths.StagingRoot,
	} {
		if err := os.MkdirAll(p, 0o700); err != nil {
			return err
		}
	}
	return nil
}
