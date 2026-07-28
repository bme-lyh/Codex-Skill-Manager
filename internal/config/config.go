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
	return model.Config{
		SchemaVersion: 1,
		Paths: model.Paths{
			SkillsRoot:     filepath.Join(home, ".codex", "skills"),
			DataRoot:       data,
			LogsRoot:       filepath.Join(data, "logs"),
			ReportsRoot:    filepath.Join(data, "reports"),
			BackupsRoot:    filepath.Join(data, "backups"),
			QuarantineRoot: filepath.Join(data, "quarantine"),
			CacheRoot:      filepath.Join(data, "cache"),
			StagingRoot:    filepath.Join(data, "staging"),
		},
		Schedule:     model.Schedule{Enabled: false, Frequency: "weekly", Time: "09:00"},
		Locale:       "zh-CN",
		GitHubHost:   "github.com",
		MaxFileBytes: 20 << 20,
		MaxFiles:     2000,
		CodexReview: model.CodexReviewConfig{
			Enabled: false, Model: "default", ReasoningEffort: "medium",
			TimeoutSeconds: 300, MaxSamplePerRisk: 8,
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
	if cfg.Paths.LogsRoot == "" && cfg.Paths.DataRoot != "" {
		cfg.Paths.LogsRoot = filepath.Join(cfg.Paths.DataRoot, "logs")
	}
	normalize(&cfg)
	if err := Validate(cfg); err != nil {
		return model.Config{}, err
	}
	return cfg, EnsureDirs(cfg)
}

func Save(path string, cfg model.Config) error {
	normalize(&cfg)
	if err := Validate(cfg); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
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
	if cfg.SchemaVersion != 1 {
		return fmt.Errorf("unsupported config schema: %d", cfg.SchemaVersion)
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
	case "minimal", "low", "medium", "high", "xhigh":
	default:
		return errors.New("codexReview.reasoningEffort must be minimal, low, medium, high or xhigh")
	}
	if cfg.CodexReview.TimeoutSeconds < 30 || cfg.CodexReview.TimeoutSeconds > 1800 {
		return errors.New("codexReview.timeoutSeconds must be between 30 and 1800")
	}
	if cfg.CodexReview.MaxSamplePerRisk < 1 || cfg.CodexReview.MaxSamplePerRisk > 20 {
		return errors.New("codexReview.maxSamplePerRisk must be between 1 and 20")
	}
	return nil
}

func normalize(cfg *model.Config) {
	if strings.TrimSpace(cfg.CodexReview.Model) == "" {
		cfg.CodexReview.Model = "default"
	}
	if strings.TrimSpace(cfg.CodexReview.ReasoningEffort) == "" {
		cfg.CodexReview.ReasoningEffort = "medium"
	}
	if cfg.CodexReview.TimeoutSeconds == 0 {
		cfg.CodexReview.TimeoutSeconds = 300
	}
	if cfg.CodexReview.MaxSamplePerRisk == 0 {
		cfg.CodexReview.MaxSamplePerRisk = 8
	}
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
