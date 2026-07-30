package codexreview

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type verifiedContextRoot struct {
	path string
	info os.FileInfo
}

type verifiedContextFile struct {
	file         *os.File
	sourcePath   string
	resolvedPath string
	root         verifiedContextRoot
	initialInfo  os.FileInfo
}

func resolveVerifiedContextRoot(root string) (verifiedContextRoot, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return verifiedContextRoot{}, err
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return verifiedContextRoot{}, fmt.Errorf("resolve context root: %w", err)
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return verifiedContextRoot{}, err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return verifiedContextRoot{}, fmt.Errorf("stat resolved context root: %w", err)
	}
	if !info.IsDir() {
		return verifiedContextRoot{}, errors.New("resolved context root is not a directory")
	}
	return verifiedContextRoot{path: filepath.Clean(resolved), info: info}, nil
}

func openVerifiedContextFile(
	root verifiedContextRoot,
	path string,
	walkInfo os.FileInfo,
	beforeOpen func(),
) (*verifiedContextFile, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	path = filepath.Clean(path)
	preOpenInfo, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("lstat context file before open: %w", err)
	}
	if preOpenInfo.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("context file became a symbolic link before open: %s", path)
	}
	if !preOpenInfo.Mode().IsRegular() {
		return nil, fmt.Errorf("context file is not regular before open: %s", path)
	}
	if walkInfo != nil && !os.SameFile(walkInfo, preOpenInfo) {
		return nil, fmt.Errorf("context file changed after enumeration: %s", path)
	}

	resolvedBefore, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, fmt.Errorf("resolve context file before open: %w", err)
	}
	resolvedBefore, err = filepath.Abs(resolvedBefore)
	if err != nil {
		return nil, err
	}
	resolvedBefore = filepath.Clean(resolvedBefore)
	if err := ensureResolvedWithinRoot(root.path, resolvedBefore); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	resolvedInfo, err := os.Stat(resolvedBefore)
	if err != nil {
		return nil, fmt.Errorf("stat resolved context file before open: %w", err)
	}
	if !os.SameFile(preOpenInfo, resolvedInfo) {
		return nil, fmt.Errorf("context file resolution changed before open: %s", path)
	}

	if beforeOpen != nil {
		beforeOpen()
	}
	file, err := os.Open(resolvedBefore)
	if err != nil {
		return nil, err
	}
	opened := &verifiedContextFile{
		file: file, sourcePath: path, resolvedPath: resolvedBefore,
		root: root, initialInfo: resolvedInfo,
	}
	if err := opened.verify(); err != nil {
		_ = file.Close()
		return nil, err
	}
	return opened, nil
}

func (opened *verifiedContextFile) verify() error {
	rootInfo, err := os.Stat(opened.root.path)
	if err != nil {
		return fmt.Errorf("revalidate context root: %w", err)
	}
	if !rootInfo.IsDir() || !os.SameFile(opened.root.info, rootInfo) {
		return errors.New("context root changed while being inventoried")
	}

	currentLstat, err := os.Lstat(opened.sourcePath)
	if err != nil {
		return fmt.Errorf("revalidate context source path: %w", err)
	}
	if currentLstat.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("context file became a symbolic link: %s", opened.sourcePath)
	}
	if !currentLstat.Mode().IsRegular() {
		return fmt.Errorf("context file is no longer regular: %s", opened.sourcePath)
	}

	resolvedAfter, err := filepath.EvalSymlinks(opened.sourcePath)
	if err != nil {
		return fmt.Errorf("resolve context file after open: %w", err)
	}
	resolvedAfter, err = filepath.Abs(resolvedAfter)
	if err != nil {
		return err
	}
	resolvedAfter = filepath.Clean(resolvedAfter)
	if err := ensureResolvedWithinRoot(opened.root.path, resolvedAfter); err != nil {
		return fmt.Errorf("%s: %w", opened.sourcePath, err)
	}
	if !sameResolvedPath(opened.resolvedPath, resolvedAfter) {
		return fmt.Errorf("context file resolved path changed while opening: %s", opened.sourcePath)
	}

	resolvedInfo, err := os.Stat(resolvedAfter)
	if err != nil {
		return fmt.Errorf("stat resolved context file after open: %w", err)
	}
	handleInfo, err := opened.file.Stat()
	if err != nil {
		return fmt.Errorf("stat opened context file: %w", err)
	}
	if !handleInfo.Mode().IsRegular() ||
		!os.SameFile(opened.initialInfo, handleInfo) ||
		!os.SameFile(currentLstat, handleInfo) ||
		!os.SameFile(resolvedInfo, handleInfo) {
		return fmt.Errorf("context file identity changed while opening: %s", opened.sourcePath)
	}
	if !sameStableFileMetadata(opened.initialInfo, handleInfo) {
		return fmt.Errorf("context file metadata changed while opening: %s", opened.sourcePath)
	}
	return nil
}

func (opened *verifiedContextFile) closeAfterRead() error {
	verifyErr := opened.verify()
	closeErr := opened.file.Close()
	if verifyErr != nil {
		return verifyErr
	}
	return closeErr
}

func ensureResolvedWithinRoot(root, candidate string) error {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return err
	}
	if filepath.IsAbs(relative) || relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("resolved context path escapes the canonical root")
	}
	return nil
}

func sameResolvedPath(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func sameStableFileMetadata(left, right os.FileInfo) bool {
	return left.Size() == right.Size() &&
		left.Mode() == right.Mode() &&
		left.ModTime().Equal(right.ModTime())
}
