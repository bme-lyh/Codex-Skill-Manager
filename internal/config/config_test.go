package config

import "testing"

func TestDefaultCodexReviewBatchLimitsAreValid(t *testing.T) {
	cfg, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CodexReview.SkillsPerBatch != 4 || cfg.CodexReview.MaxParallelBatches != 2 {
		t.Fatalf("unexpected Codex review batching defaults: %#v", cfg.CodexReview)
	}
	if err := Validate(cfg); err != nil {
		t.Fatalf("default configuration must validate: %v", err)
	}
}

func TestValidateRejectsUnsafeCodexReviewBatchLimits(t *testing.T) {
	cfg, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	cfg.CodexReview.SkillsPerBatch = 13
	if err := Validate(cfg); err == nil {
		t.Fatal("expected oversized Skill batch to fail validation")
	}
	cfg, err = Default()
	if err != nil {
		t.Fatal(err)
	}
	cfg.CodexReview.MaxParallelBatches = 5
	if err := Validate(cfg); err == nil {
		t.Fatal("expected excessive parallel batches to fail validation")
	}
}
