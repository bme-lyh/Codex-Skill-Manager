package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bme-lyh/Codex-Skill-Manager/internal/model"
)

func TestDefaultCodexReviewGroupParallelismIsValid(t *testing.T) {
	cfg, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CodexReview.MaxParallelBatches != 1 {
		t.Fatalf("unexpected Codex review group parallelism: %#v", cfg.CodexReview)
	}
	if err := Validate(cfg); err != nil {
		t.Fatalf("default configuration must validate: %v", err)
	}
}

func TestValidateRejectsUnsafeCodexReviewGroupParallelism(t *testing.T) {
	cfg, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	cfg.CodexReview.MaxParallelBatches = 5
	if err := Validate(cfg); err == nil {
		t.Fatal("expected excessive parallel batches to fail validation")
	}
}

func TestDefaultLocaleIsSimplifiedChinese(t *testing.T) {
	cfg, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Locale != "zh-CN" {
		t.Fatalf("unexpected default locale: %q", cfg.Locale)
	}
}

func TestNormalizeLegacyConfigDefaultsLocale(t *testing.T) {
	cfg, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Locale = ""
	normalize(&cfg)
	if cfg.Locale != "zh-CN" {
		t.Fatalf("legacy config locale was not normalized: %q", cfg.Locale)
	}
}

func TestValidateRejectsUnsupportedLocale(t *testing.T) {
	cfg, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Locale = "fr-FR"
	if err := Validate(cfg); err == nil {
		t.Fatal("expected unsupported locale to fail validation")
	}
}

func TestValidateAcceptsCatalogReasoningTiers(t *testing.T) {
	cfg, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	for _, effort := range []string{"xhigh", "max", "ultra"} {
		cfg.CodexReview.ReasoningEffort = effort
		if err := Validate(cfg); err != nil {
			t.Fatalf("catalog tier %q should validate: %v", effort, err)
		}
	}
}

func TestDefaultUsesLunaExtraHighReview(t *testing.T) {
	cfg, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Theme != "system" || cfg.CodexReview.Model != "gpt-5.6-luna" || cfg.CodexReview.ReasoningEffort != "xhigh" {
		t.Fatalf("unexpected appearance or Codex defaults: %#v", cfg)
	}
}

func TestDefaultHasTwoRootsWithoutCreatingSkillDirectories(t *testing.T) {
	cfg, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SchemaVersion != 2 || cfg.DefaultRootID != model.RootIDCodexDefault || len(cfg.SkillRoots) != 2 {
		t.Fatalf("unexpected v2 defaults: %#v", cfg)
	}
	for _, root := range cfg.SkillRoots {
		if root.ID == "" || root.Path == "" || !root.Enabled || root.SystemDir != ".system" {
			t.Fatalf("invalid root default: %#v", root)
		}
	}
	data := t.TempDir()
	cfg.SkillRoots[0].Path = filepath.Join(data, "codex-skills")
	cfg.SkillRoots[1].Path = filepath.Join(data, "agents-skills")
	cfg.Paths.DataRoot = filepath.Join(data, "data")
	cfg.Paths.LogsRoot = filepath.Join(cfg.Paths.DataRoot, "logs")
	cfg.Paths.ReportsRoot = filepath.Join(cfg.Paths.DataRoot, "reports")
	cfg.Paths.BackupsRoot = filepath.Join(cfg.Paths.DataRoot, "backups")
	cfg.Paths.QuarantineRoot = filepath.Join(cfg.Paths.DataRoot, "quarantine")
	cfg.Paths.CacheRoot = filepath.Join(cfg.Paths.DataRoot, "cache")
	cfg.Paths.StagingRoot = filepath.Join(cfg.Paths.DataRoot, "staging")
	if err := EnsureDirs(cfg); err != nil {
		t.Fatal(err)
	}
	for _, root := range cfg.SkillRoots {
		if _, err := os.Stat(root.Path); !os.IsNotExist(err) {
			t.Fatalf("config setup unexpectedly created skill root %q: %v", root.Path, err)
		}
	}
}

func TestSkillTargetRejectsSystemDirectory(t *testing.T) {
	root := model.SkillRoot{ID: model.RootIDAgents, Path: t.TempDir(), Enabled: true, SystemDir: ".system"}
	if _, err := SkillTarget(root, ".system"); err == nil {
		t.Fatal("expected system target to be read-only")
	}
}

func TestSaveMigratesV1WithoutChangingSkillRootPath(t *testing.T) {
	base := t.TempDir()
	legacyRoot := filepath.Join(base, "legacy-skills")
	cfg := model.Config{
		SchemaVersion: 1,
		Paths:         model.Paths{SkillsRoot: legacyRoot, DataRoot: filepath.Join(base, "data"), LogsRoot: filepath.Join(base, "data", "logs"), ReportsRoot: filepath.Join(base, "data", "reports"), BackupsRoot: filepath.Join(base, "data", "backups"), QuarantineRoot: filepath.Join(base, "data", "quarantine"), CacheRoot: filepath.Join(base, "data", "cache"), StagingRoot: filepath.Join(base, "data", "staging")},
		Locale:        "zh-CN", GitHubHost: "github.com", MaxFileBytes: 1, MaxFiles: 1,
	}
	path := filepath.Join(base, "config.yaml")
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.SchemaVersion != 2 || loaded.Paths.SkillsRoot != legacyRoot || loaded.DefaultRootID != model.RootIDCodexDefault {
		t.Fatalf("v1 migration changed root identity: %#v", loaded)
	}
	if len(loaded.SkillRoots) != 2 || loaded.SkillRoots[0].Path != legacyRoot || loaded.SkillRoots[1].Enabled {
		t.Fatalf("custom v1 root unexpectedly enabled a live user root: %#v", loaded.SkillRoots)
	}
}
