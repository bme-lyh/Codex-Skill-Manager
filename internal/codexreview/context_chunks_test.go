package codexreview

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/bme-lyh/Codex-Skill-Manager/internal/model"
)

func TestBuildPackagedContextChunksCoversLargeGroupExactlyOnce(t *testing.T) {
	const fileCount = 1140
	files := make([]installAnalysisFile, 0, fileCount)
	expectedContent := make(map[string]string, fileCount)
	var rawBytes int
	for index := 0; index < fileCount; index++ {
		content := strings.Repeat(
			fmt.Sprintf("academic research context file %04d\n", index),
			330,
		)
		sum := sha256.Sum256([]byte(content))
		path := fmt.Sprintf("academic-research-suite/references/file-%04d.md", index)
		files = append(files, installAnalysisFile{
			Path: path, Size: int64(len(content)), SHA256: hex.EncodeToString(sum[:]),
			Kind: "text", Encoding: "utf-8", Content: content,
		})
		expectedContent[path] = content
		rawBytes += len(content)
	}
	if rawBytes <= 12<<20 {
		t.Fatalf("fixture must exceed 12 MiB, got %d bytes", rawBytes)
	}

	subjects := []contextSubject{{
		Name: "academic-research-suite", SourcePath: "academic-research-suite",
	}}
	chunks, err := buildPackagedContextChunks(
		"security-review", "en-US", "research", "Research", subjects, files,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) < 2 || len(chunks) > maxContextChunks {
		t.Fatalf("unexpected deterministic chunk count: %d", len(chunks))
	}
	seen := make(map[string]int, fileCount)
	for index, chunk := range chunks {
		if chunk.Index != index+1 || chunk.Count != len(chunks) {
			t.Fatalf("invalid chunk sequence at %d: %#v", index, chunk)
		}
		if len(chunk.Payload) > maxContextChunkPayloadBytes {
			t.Fatalf("chunk %d payload exceeds limit: %d", chunk.Index, len(chunk.Payload))
		}
		var input contextChunkInput
		if err := json.Unmarshal(chunk.Payload, &input); err != nil {
			t.Fatal(err)
		}
		if input.ContextMode != "packaged-no-tools-context-chunk" ||
			input.ChunkIndex != chunk.Index || input.ChunkCount != chunk.Count ||
			input.ChunkDigest != chunk.Digest {
			t.Fatalf("unexpected chunk envelope: %#v", input)
		}
		for _, file := range input.Files {
			seen[file.Path]++
			if file.Content != expectedContent[file.Path] {
				t.Fatalf("file %q was not included verbatim", file.Path)
			}
		}
	}
	if len(seen) != fileCount {
		t.Fatalf("covered %d files, want %d", len(seen), fileCount)
	}
	for path, count := range seen {
		if count != 1 {
			t.Fatalf("file %q appeared in %d chunks, want exactly one", path, count)
		}
	}

	repeated, err := buildPackagedContextChunks(
		"security-review", "en-US", "research", "Research", subjects, files,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(repeated) != len(chunks) {
		t.Fatalf("chunk count is not deterministic: %d != %d", len(repeated), len(chunks))
	}
	for index := range chunks {
		if repeated[index].Digest != chunks[index].Digest ||
			string(repeated[index].Payload) != string(chunks[index].Payload) {
			t.Fatalf("chunk %d is not deterministic", index+1)
		}
	}

	summaries := make([]contextChunkSummary, len(chunks))
	for index, chunk := range chunks {
		summaries[index] = contextChunkSummary{
			ChunkIndex: chunk.Index, ChunkDigest: chunk.Digest,
			ReviewedFileCount: len(chunk.Files), Summary: "bounded summary",
			Signals: []contextChunkSignal{},
		}
	}
	finalPayload, err := buildReviewSynthesisPayload(
		"en-US",
		0,
		1,
		1,
		reviewBatch{
			GroupID: "research", GroupName: "Research",
			Skills: []reviewSkill{{
				Name: "academic-research-suite", SourcePath: "academic-research-suite",
				FileCount: fileCount,
			}},
		},
		packagedContextMetadata(files),
		summaries,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(finalPayload) > maxAssistedPromptBytes {
		t.Fatalf("final synthesis payload exceeds limit: %d", len(finalPayload))
	}
	if strings.Contains(string(finalPayload), "academic research context file") {
		t.Fatal("final synthesis payload must contain metadata and summaries, not duplicate raw text")
	}
	var synthesis batchInput
	if err := json.Unmarshal(finalPayload, &synthesis); err != nil {
		t.Fatal(err)
	}
	if synthesis.ContextFileCount != fileCount || synthesis.OmittedFileCount != fileCount-len(synthesis.Files) {
		t.Fatalf("unexpected synthesis coverage: files=%d omitted=%d total=%d",
			len(synthesis.Files), synthesis.OmittedFileCount, synthesis.ContextFileCount)
	}
	if len(synthesis.Files) > 400 {
		t.Fatalf("final synthesis retained %d metadata files, want at most 400", len(synthesis.Files))
	}
}

func TestCodexContextBudgetStaysBelowTurnCharacterLimit(t *testing.T) {
	if maxContextChunkPayloadBytes >= 1_048_576 {
		t.Fatalf("chunk budget %d must remain below Codex turn limit", maxContextChunkPayloadBytes)
	}
	if maxCodexInputBytes != maxContextChunkPayloadBytes {
		t.Fatalf("final and chunk budgets diverged: %d != %d", maxCodexInputBytes, maxContextChunkPayloadBytes)
	}
}

func TestSafeCodexContextFilesRedactsCredentialLikePaths(t *testing.T) {
	files := []installAnalysisFile{
		{Path: ".env", Kind: "text", Encoding: "utf-8", Content: "TOKEN=secret", Size: 12, SHA256: "a"},
		{Path: "README.md", Kind: "text", Encoding: "utf-8", Content: "safe", Size: 4, SHA256: "b"},
	}
	safe := safeCodexContextFiles(files)
	if !safe[0].Redacted || safe[0].Content != "" || safe[0].Encoding != "redacted" {
		t.Fatalf("credential-like file was not converted to metadata-only input: %#v", safe[0])
	}
	if safe[0].SHA256 != files[0].SHA256 || safe[0].Size != files[0].Size {
		t.Fatal("redaction must preserve immutable metadata")
	}
	if safe[1].Redacted || safe[1].Content != "safe" {
		t.Fatalf("ordinary documentation was unexpectedly redacted: %#v", safe[1])
	}
	if files[0].Content != "TOKEN=secret" {
		t.Fatal("redaction mutated the local inventory")
	}
}

func TestValidateContextChunkSummaryRejectsMissingChunkAndSpoofedPath(t *testing.T) {
	content := "trusted repository text"
	sum := sha256.Sum256([]byte(content))
	chunk := packagedContextChunk{
		Index: 1, Count: 1,
		Files: []installAnalysisFile{{
			Path: "skills/alpha/SKILL.md", Size: int64(len(content)),
			SHA256: hex.EncodeToString(sum[:]), Kind: "text", Encoding: "utf-8",
			Content: content,
		}},
	}
	chunk.Digest = packagedFilesDigest(chunk.Files)

	valid := contextChunkSummary{
		ChunkIndex: 1, ChunkDigest: chunk.Digest, ReviewedFileCount: 1,
		Summary: "The chunk contains one Skill.",
		Signals: []contextChunkSignal{{
			Category: "security", Title: "Review script behavior",
			Description:   "A script is described by the Skill.",
			EvidenceFiles: []string{"skills/alpha/SKILL.md"},
		}},
	}
	if _, err := validateContextChunkSummary(chunk, valid); err != nil {
		t.Fatalf("valid bounded summary was rejected: %v", err)
	}

	missing := valid
	missing.ChunkIndex = 2
	if _, err := validateContextChunkSummary(chunk, missing); err == nil {
		t.Fatal("expected a missing/reordered chunk index to be rejected")
	}
	spoofed := valid
	spoofed.Signals = append([]contextChunkSignal(nil), valid.Signals...)
	spoofed.Signals[0].EvidenceFiles = []string{"../../host-secret.txt"}
	if _, err := validateContextChunkSummary(chunk, spoofed); err == nil {
		t.Fatal("expected a path outside the bounded chunk to be rejected")
	}
}

func TestContextChunkOutputSchemaIsValidJSON(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal([]byte(contextChunkOutputSchema), &schema); err != nil {
		t.Fatalf("invalid context chunk output schema: %v", err)
	}
}

func TestInstallChunkSynthesisKeepsMetadataAndSummaries(t *testing.T) {
	files := []installAnalysisFile{{
		Path: "SKILL.md", Size: 10,
		SHA256: strings.Repeat("a", 64), Kind: "text", Encoding: "utf-8",
	}}
	chunks := []contextChunkSummary{{
		ChunkIndex: 1, ChunkDigest: strings.Repeat("b", 64),
		ReviewedFileCount: 1, Summary: "Install by copying the Skill.",
		Signals: []contextChunkSignal{},
	}}
	preview := model.InstallPreview{
		Repository: model.Repository{Provider: "local", FullName: "local:test"},
		Skills: []model.CandidateSkill{{
			Name: "alpha", SourcePath: ".", Files: []model.FileRecord{{Path: "SKILL.md"}},
		}},
	}
	payload, err := buildInstallAnalysisInputWithChunks(preview, "en-US", files, chunks)
	if err != nil {
		t.Fatal(err)
	}
	var input installAnalysisInput
	if err := json.Unmarshal(payload, &input); err != nil {
		t.Fatal(err)
	}
	if input.ContextMode != "full-repository-chunk-summaries-no-tools" ||
		len(input.ContextChunks) != 1 || len(input.Files) != 1 ||
		input.Files[0].Content != "" {
		t.Fatalf("unexpected install synthesis input: %#v", input)
	}
}

func TestLockedPyPIWheelPermissionIsOfflinePackageApproval(t *testing.T) {
	title, description, kind, risk := assistedPermissionDescription(
		model.AssistedInstallPermissionPyPIWheelLock,
		"en-US",
	)
	if title != "Use locked PyPI Wheels" {
		t.Fatalf("unexpected locked-Wheel permission title: %q", title)
	}
	if kind != "package-approval" || risk != "standard" {
		t.Fatalf("unexpected locked-Wheel permission classification: kind=%q risk=%q", kind, risk)
	}
	for _, expected := range []string{"offline", "names", "versions", "filenames", "SHA256"} {
		if !strings.Contains(description, expected) {
			t.Fatalf("locked-Wheel permission description is missing %q: %q", expected, description)
		}
	}
}

func TestBoundedDiagnosticBufferKeepsOnlyTheLatestBytes(t *testing.T) {
	buffer := newBoundedDiagnosticBuffer(8)
	for _, value := range []string{"abcd", "efgh", "ijkl"} {
		written, err := buffer.Write([]byte(value))
		if err != nil {
			t.Fatal(err)
		}
		if written != len(value) {
			t.Fatalf("short diagnostic write: %d", written)
		}
	}
	if got := buffer.String(); got != "efghijkl" {
		t.Fatalf("unexpected diagnostic tail: %q", got)
	}

	written, err := buffer.Write([]byte("0123456789"))
	if err != nil {
		t.Fatal(err)
	}
	if written != 10 || buffer.String() != "23456789" {
		t.Fatalf("oversized diagnostic write was not bounded: %q", buffer.String())
	}
}
