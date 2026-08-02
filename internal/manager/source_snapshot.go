package manager

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/bme-lyh/Codex-Skill-Manager/internal/model"
)

var immutableGitHubSHA = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

// snapshotLocalSource copies a local source into managed staging without
// following links or accepting special files. Partial snapshots are left in
// staging on failure for explicit, inspectable recovery; they are never used by
// a persisted plan.
func snapshotLocalSource(source, destination string, maxFiles int, maxFileBytes int64) error {
	if maxFiles < 1 || maxFileBytes < 1 {
		return errors.New("local snapshot limits must be positive")
	}
	if err := os.MkdirAll(destination, 0o700); err != nil {
		return err
	}
	count := 0
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == source {
			return nil
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if relative == "." || filepath.IsAbs(relative) || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return errors.New("local snapshot path escaped the source root")
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("local snapshot rejects links and reparse points: %s", filepath.ToSlash(relative))
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if err := ensureWithinOrEqual(destination, target); err != nil {
			return err
		}
		if info.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("local snapshot rejects special file: %s", filepath.ToSlash(relative))
		}
		count++
		if count > maxFiles {
			return fmt.Errorf("local source exceeds file count limit %d", maxFiles)
		}
		if info.Size() > maxFileBytes {
			return fmt.Errorf("local source file exceeds size limit: %s", filepath.ToSlash(relative))
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		openedInfo, err := input.Stat()
		if err != nil {
			_ = input.Close()
			return err
		}
		if !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) || openedInfo.Size() != info.Size() {
			_ = input.Close()
			return fmt.Errorf("local source changed while snapshotting: %s", filepath.ToSlash(relative))
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			_ = input.Close()
			return err
		}
		copied, copyErr := io.Copy(output, io.LimitReader(input, maxFileBytes+1))
		afterInfo, statErr := input.Stat()
		inputCloseErr := input.Close()
		outputCloseErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		if statErr != nil {
			return statErr
		}
		if !os.SameFile(openedInfo, afterInfo) || afterInfo.Size() != openedInfo.Size() || copied != openedInfo.Size() {
			return fmt.Errorf("local source changed while snapshotting: %s", filepath.ToSlash(relative))
		}
		if inputCloseErr != nil {
			return inputCloseErr
		}
		if outputCloseErr != nil {
			return outputCloseErr
		}
		return nil
	})
}

func installPreviewDigest(preview model.InstallPreview) (string, error) {
	copyValue := preview
	copyValue.PreviewDigest = ""
	data, err := json.Marshal(copyValue)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func sealInstallPreview(preview *model.InstallPreview) error {
	if preview == nil {
		return errors.New("install preview is required")
	}
	digest, err := installPreviewDigest(*preview)
	if err != nil {
		return err
	}
	preview.PreviewDigest = digest
	return nil
}

func (m *Manager) verifyInstallPreviewMetadata(preview model.InstallPreview, requestedID string) error {
	requestedID = strings.TrimSpace(requestedID)
	if requestedID == "" || filepath.Base(requestedID) != requestedID || !strings.HasPrefix(requestedID, "plan-") {
		return errors.New("invalid install preview ID")
	}
	if preview.ID != requestedID {
		return errors.New("install preview identity mismatch")
	}
	if preview.CreatedAt.IsZero() || preview.ExpiresAt.IsZero() || !preview.ExpiresAt.After(preview.CreatedAt) {
		return errors.New("install preview has invalid timestamps")
	}
	if time.Now().UTC().After(preview.ExpiresAt) {
		return errors.New("install plan has expired")
	}
	if err := ensureResolvedWithinOrEqual(m.Config.Paths.StagingRoot, preview.StagingPath); err != nil {
		return fmt.Errorf("install staging path is not managed by this application: %w", err)
	}
	if preview.Repository.Provider == "github" && !immutableGitHubSHA.MatchString(preview.Repository.CommitSHA) {
		return errors.New("GitHub install plan is not pinned to a full immutable commit SHA")
	}
	if preview.Repository.Provider != "github" && preview.Repository.Provider != "local" {
		return errors.New("unsupported install preview provider")
	}
	expected, err := installPreviewDigest(preview)
	if err != nil {
		return err
	}
	if preview.PreviewDigest == "" || !strings.EqualFold(preview.PreviewDigest, expected) {
		return errors.New("install preview integrity digest mismatch")
	}
	return nil
}
