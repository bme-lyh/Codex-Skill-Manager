package config

import "testing"

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
