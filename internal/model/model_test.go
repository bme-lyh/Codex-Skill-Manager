package model

import (
	"encoding/json"
	"testing"
)

func TestSourceAssociationNormalizationFailsClosed(t *testing.T) {
	if got := NormalizeSourceAssociation("github", ""); got != SourceAssociationRemote {
		t.Fatalf("github fallback was not remote-associated: %q", got)
	}
	if got := NormalizeSourceAssociation("github", "unknown"); got != SourceAssociationUnlinked {
		t.Fatalf("explicit unknown association was trusted: %q", got)
	}
	if got := NormalizeSourceAssociation("local", ""); got != SourceAssociationUnlinked {
		t.Fatalf("legacy local source was not kept unlinked: %q", got)
	}
	if got := NormalizeSourceAssociation("other", ""); got != SourceAssociationUnlinked {
		t.Fatalf("unknown provider was not unlinked: %q", got)
	}
}

func TestImmutableCommitHelpersRequireFullSHA(t *testing.T) {
	valid := "0123456789abcdef0123456789abcdef01234567"
	if !IsImmutableCommitSHA(valid) || !IsImmutableGitHubCommit(valid) {
		t.Fatal("full commit SHA was rejected")
	}
	for _, value := range []string{"", "main", valid[:39], valid + "0", "0123456789abcdef0123456789abcdef0123456g"} {
		if IsImmutableCommitSHA(value) {
			t.Fatalf("mutable or malformed ref was accepted: %q", value)
		}
	}
	canonical, err := CanonicalCommitSHA("ABCDEF0123456789ABCDEF0123456789ABCDEF01")
	if err != nil || canonical != "abcdef0123456789abcdef0123456789abcdef01" {
		t.Fatalf("commit was not canonicalized: %q, %v", canonical, err)
	}
}

func TestNewSourceFieldsDoNotBreakLegacyJSON(t *testing.T) {
	legacy := []byte(`{"provider":"github","repository":"owner/repo","requestedRef":"main","resolvedCommit":"0123456789abcdef0123456789abcdef01234567","skills":{}}`)
	var lock PackageLock
	if err := json.Unmarshal(legacy, &lock); err != nil {
		t.Fatal(err)
	}
	if lock.Provider != "github" || lock.SourceAssociation != "" || lock.ResolvedRef != "" {
		t.Fatalf("legacy lock unexpectedly changed during decode: %#v", lock)
	}
	data, err := json.Marshal(lock)
	if err != nil {
		t.Fatal(err)
	}
	var roundTrip PackageLock
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Fatal(err)
	}
	if roundTrip.ResolvedCommit != lock.ResolvedCommit {
		t.Fatalf("legacy immutable commit was not preserved: %#v", roundTrip)
	}
}
