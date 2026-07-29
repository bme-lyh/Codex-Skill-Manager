package config

import "testing"

func TestDefaultCodexReviewGroupParallelismIsValid(t *testing.T) {
	cfg, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CodexReview.MaxParallelBatches != 2 {
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
