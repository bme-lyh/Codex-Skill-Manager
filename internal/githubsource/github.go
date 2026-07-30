package githubsource

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bme-lyh/Codex-Skill-Manager/internal/auth"
	"github.com/bme-lyh/Codex-Skill-Manager/internal/inventory"
	"github.com/bme-lyh/Codex-Skill-Manager/internal/model"
	"github.com/bme-lyh/Codex-Skill-Manager/internal/processutil"
)

var safePart = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

const (
	maxArchiveEntries             = 20_000
	defaultMaxArchiveEntryBytes   = int64(20 << 20)
	minArchiveDownloadBytes       = int64(100 << 20)
	minArchiveUncompressedBytes   = int64(200 << 20)
	archiveDownloadMultiplier     = int64(10)
	archiveUncompressedMultiplier = int64(20)
	maxArchiveDownloadAttempts    = 3
	baseArchiveRetryDelay         = 100 * time.Millisecond
	maxArchiveRetryDelay          = 2 * time.Second
)

var skippedDiscoveryDirectories = map[string]struct{}{
	".git":         {},
	".hg":          {},
	".nox":         {},
	".svn":         {},
	".tox":         {},
	".venv":        {},
	"__pycache__":  {},
	"build":        {},
	"dist":         {},
	"env":          {},
	"node_modules": {},
	"venv":         {},
	"vendor":       {},
}

type zipExtractionLimits struct {
	MaxEntries    int
	MaxEntryBytes int64
	MaxTotalBytes int64
}

type preparedZipEntry struct {
	file   *zip.File
	target string
	key    string
	isDir  bool
}

type ParsedURL struct {
	Owner, Repo, Ref, SourcePath string
}

type Client struct {
	http  *http.Client
	mu    sync.Mutex
	cache map[string]cacheEntry
}

type cacheEntry struct {
	Body      []byte
	ETag      string
	ExpiresAt time.Time
}

type ResolveMeta struct {
	FromCache bool
}

type APIError struct {
	StatusCode int
	Status     string
	Message    string
	Limit      int
	Remaining  int
	ResetAt    *time.Time
	RetryAt    *time.Time
}

func (e *APIError) Error() string {
	if e.RetryAt != nil {
		return fmt.Sprintf("GitHub API %s：请求额度已用尽，可在 %s 后重试", e.Status, e.RetryAt.Local().Format("2006-01-02 15:04:05"))
	}
	if e.Message != "" {
		return fmt.Sprintf("GitHub API %s：%s", e.Status, e.Message)
	}
	return "GitHub API " + e.Status
}

func New() *Client {
	return &Client{http: &http.Client{Timeout: 90 * time.Second}, cache: map[string]cacheEntry{}}
}

func Parse(raw string) (ParsedURL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ParsedURL{}, err
	}
	if !strings.EqualFold(u.Scheme, "https") || !strings.EqualFold(u.Hostname(), "github.com") {
		return ParsedURL{}, errors.New("only https://github.com URLs are supported")
	}
	repositoryPath := strings.Trim(u.Path, "/")
	repositoryPath = strings.TrimSuffix(repositoryPath, ".git")
	parts := strings.Split(repositoryPath, "/")
	if len(parts) < 2 || !validGitHubRepositoryPart(parts[0]) ||
		!validGitHubRepositoryPart(parts[1]) {
		return ParsedURL{}, errors.New("invalid GitHub repository URL")
	}
	if len(parts) > 2 &&
		(len(parts) < 4 || (parts[2] != "tree" && parts[2] != "blob")) {
		return ParsedURL{}, errors.New("unsupported GitHub repository URL path")
	}
	out := ParsedURL{Owner: parts[0], Repo: parts[1]}
	if len(parts) >= 4 && (parts[2] == "tree" || parts[2] == "blob") {
		if err := validateGitHubRef(parts[3]); err != nil {
			return ParsedURL{}, err
		}
		out.Ref = parts[3]
		if len(parts) > 4 {
			out.SourcePath, err = normalizePortableRelativePath(strings.Join(parts[4:], "/"))
			if err != nil {
				return ParsedURL{}, fmt.Errorf("invalid GitHub repository source path: %w", err)
			}
			if parts[2] == "blob" && strings.EqualFold(filepath.Base(out.SourcePath), "SKILL.md") {
				out.SourcePath = filepath.ToSlash(filepath.Dir(out.SourcePath))
				if out.SourcePath == "." {
					out.SourcePath = ""
				}
			}
		}
	}
	return out, nil
}

func validGitHubRepositoryPart(value string) bool {
	return value != "." && value != ".." && safePart.MatchString(value)
}

func validateGitHubRef(ref string) error {
	if ref == "" || ref == "." || ref == ".." || strings.Contains(ref, "..") ||
		strings.ContainsRune(ref, '\\') {
		return errors.New("invalid GitHub repository ref")
	}
	for _, char := range ref {
		if char == 0 || char < 0x20 || char == 0x7f {
			return errors.New("invalid GitHub repository ref")
		}
	}
	return nil
}

func normalizePortableRelativePath(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if strings.ContainsRune(value, '\x00') || strings.ContainsRune(value, '\\') ||
		pathpkg.IsAbs(value) {
		return "", errors.New("path must be a portable local relative path")
	}
	parts := strings.Split(value, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", errors.New("path contains an empty, current, or parent segment")
		}
		if err := validatePortablePathElement(part); err != nil {
			return "", err
		}
	}
	clean := pathpkg.Clean(value)
	if clean != value {
		return "", errors.New("path must already be normalized")
	}
	local := filepath.FromSlash(clean)
	if !filepath.IsLocal(local) || filepath.IsAbs(local) || filepath.VolumeName(local) != "" {
		return "", errors.New("path must stay relative to its managed root")
	}
	return clean, nil
}

func validatePortablePathElement(value string) error {
	if strings.HasSuffix(value, ".") || strings.HasSuffix(value, " ") {
		return errors.New("Windows path elements must not end with a dot or space")
	}
	for _, char := range value {
		if char < 0x20 || char == 0x7f || strings.ContainsRune(`<>:"|?*`, char) {
			return errors.New("path contains a character that is unsafe on Windows")
		}
	}
	base := value
	if index := strings.IndexAny(base, ".:"); index >= 0 {
		base = base[:index]
	}
	base = strings.ToUpper(strings.TrimRight(base, " "))
	switch base {
	case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$", "CLOCK$":
		return errors.New("path contains a reserved Windows device name")
	}
	if len(base) == 4 &&
		(strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) &&
		base[3] >= '1' && base[3] <= '9' {
		return errors.New("path contains a reserved Windows device name")
	}
	if strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT") {
		switch strings.TrimPrefix(strings.TrimPrefix(base, "COM"), "LPT") {
		case "\u00b2", "\u00b3", "\u00b9":
			return errors.New("path contains a reserved Windows device name")
		}
	}
	return nil
}

func (c *Client) Resolve(ctx context.Context, raw, overrideRef string) (model.Repository, error) {
	repo, _, err := c.ResolveCached(ctx, raw, overrideRef, false)
	return repo, err
}

func (c *Client) ResolveCached(ctx context.Context, raw, overrideRef string, force bool) (model.Repository, ResolveMeta, error) {
	p, err := Parse(raw)
	if err != nil {
		return model.Repository{}, ResolveMeta{}, err
	}
	fromCache := true
	var repoResponse struct {
		FullName      string    `json:"full_name"`
		Private       bool      `json:"private"`
		DefaultBranch string    `json:"default_branch"`
		UpdatedAt     time.Time `json:"updated_at"`
		License       *struct {
			SPDXID string `json:"spdx_id"`
		} `json:"license"`
	}
	cached, err := c.getJSON(ctx, fmt.Sprintf("https://api.github.com/repos/%s/%s", p.Owner, p.Repo), &repoResponse, force)
	if err != nil {
		return model.Repository{}, ResolveMeta{}, err
	}
	fromCache = fromCache && cached
	ref := overrideRef
	if ref == "" {
		ref = p.Ref
	}
	if ref == "" {
		ref = repoResponse.DefaultBranch
	}
	var commit struct {
		SHA string `json:"sha"`
	}
	cached, err = c.getJSON(ctx, fmt.Sprintf("https://api.github.com/repos/%s/%s/commits/%s", p.Owner, p.Repo, url.PathEscape(ref)), &commit, force)
	if err != nil {
		return model.Repository{}, ResolveMeta{}, fmt.Errorf("resolve ref %q: %w", ref, err)
	}
	fromCache = fromCache && cached
	license := ""
	if repoResponse.License != nil {
		license = repoResponse.License.SPDXID
	}
	return model.Repository{
		Provider: "github",
		Owner:    p.Owner, Name: p.Repo, FullName: repoResponse.FullName,
		Private: repoResponse.Private, DefaultBranch: repoResponse.DefaultBranch,
		UpdatedAt: repoResponse.UpdatedAt, License: license,
		ResolvedRef: ref, CommitSHA: commit.SHA, SourcePath: p.SourcePath,
	}, ResolveMeta{FromCache: fromCache}, nil
}

func (c *Client) Download(ctx context.Context, repo model.Repository, dest string, maxBytes int64) (string, error) {
	resp, err := c.downloadArchiveResponse(ctx, repo)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	downloadLimit, extractionLimits := archiveLimits(maxBytes)
	if resp.ContentLength > downloadLimit {
		return "", errors.New("repository archive exceeds configured limit")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limitWithSentinel(downloadLimit)))
	if err != nil {
		return "", err
	}
	if int64(len(data)) > downloadLimit {
		return "", errors.New("repository archive exceeds configured limit")
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return "", err
	}
	if err := os.Mkdir(dest, 0o700); err != nil {
		return "", fmt.Errorf("create exclusive repository staging directory: %w", err)
	}
	return extractZipWithLimits(data, dest, extractionLimits)
}

func (c *Client) downloadArchiveResponse(
	ctx context.Context,
	repo model.Repository,
) (*http.Response, error) {
	endpoint := fmt.Sprintf(
		"https://api.github.com/repos/%s/zipball/%s",
		repo.FullName,
		repo.CommitSHA,
	)
	var lastErr error
	for attempt := 1; attempt <= maxArchiveDownloadAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		c.authorize(req)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		resp, err := c.http.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			lastErr = err
			if attempt == maxArchiveDownloadAttempts {
				break
			}
			if err := waitForArchiveRetry(ctx, archiveBackoff(attempt)); err != nil {
				return nil, err
			}
			continue
		}
		if resp.StatusCode == http.StatusOK {
			return resp, nil
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		responseErr := apiError(resp, body)
		lastErr = responseErr
		delay, retry := archiveResponseRetry(responseErr, attempt)
		if !retry || attempt == maxArchiveDownloadAttempts {
			return nil, responseErr
		}
		if err := waitForArchiveRetry(ctx, delay); err != nil {
			return nil, err
		}
	}
	return nil, lastErr
}

func archiveResponseRetry(err error, attempt int) (time.Duration, bool) {
	var responseErr *APIError
	if !errors.As(err, &responseErr) {
		return 0, false
	}
	switch responseErr.StatusCode {
	case http.StatusRequestTimeout,
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
	case http.StatusForbidden:
		if responseErr.RetryAt == nil {
			return 0, false
		}
	default:
		return 0, false
	}
	delay := archiveBackoff(attempt)
	if responseErr.RetryAt != nil {
		delay = time.Until(*responseErr.RetryAt)
		if delay < 0 {
			delay = 0
		}
	}
	if delay > maxArchiveRetryDelay {
		return 0, false
	}
	return delay, true
}

func archiveBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := baseArchiveRetryDelay
	for index := 1; index < attempt && delay < maxArchiveRetryDelay; index++ {
		delay *= 2
	}
	if delay > maxArchiveRetryDelay {
		return maxArchiveRetryDelay
	}
	return delay
}

func waitForArchiveRetry(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func Discover(root, sourcePath string) ([]model.CandidateSkill, error) {
	root, base, err := secureDiscoveryBase(root, sourcePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("source path does not exist: %s", sourcePath)
		}
		return nil, err
	}
	var out []model.CandidateSkill
	err = filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("repository discovery refuses symbolic links: %s", path)
		}
		if d.IsDir() && filepath.Clean(path) != filepath.Clean(base) {
			if _, skip := skippedDiscoveryDirectories[strings.ToLower(d.Name())]; skip {
				return filepath.SkipDir
			}
		}
		if !d.IsDir() {
			return nil
		}
		skillFile := filepath.Join(path, "SKILL.md")
		fm, err := inventory.ReadFrontmatter(skillFile)
		if err != nil {
			return nil
		}
		files, err := inventory.HashTree(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		out = append(out, model.CandidateSkill{
			Name: fm.Name, Description: fm.Description,
			SourcePath: filepath.ToSlash(rel), Files: files,
		})
		return filepath.SkipDir
	})
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("source path does not exist: %s", sourcePath)
	}
	if err != nil {
		return nil, err
	}
	byName := map[string][]model.CandidateSkill{}
	for _, candidate := range out {
		byName[candidate.Name] = append(byName[candidate.Name], candidate)
	}
	out = out[:0]
	for name, variants := range byName {
		candidate := variants[0]
		conflictingPaths := []string{candidate.SourcePath}
		for _, variant := range variants[1:] {
			if !sameCandidateFiles(candidate.Files, variant.Files) {
				conflictingPaths = append(conflictingPaths, variant.SourcePath)
				continue
			}
			if preferredCandidatePath(variant.SourcePath, candidate.SourcePath) {
				candidate = variant
				conflictingPaths[0] = variant.SourcePath
			}
		}
		if len(conflictingPaths) > 1 {
			for _, variant := range variants[1:] {
				alreadyIncluded := false
				for _, path := range conflictingPaths {
					if path == variant.SourcePath {
						alreadyIncluded = true
						break
					}
				}
				if !alreadyIncluded && !sameCandidateFiles(candidate.Files, variant.Files) {
					conflictingPaths = append(conflictingPaths, variant.SourcePath)
				}
			}
			sort.Strings(conflictingPaths)
			return nil, &SkillNameConflictError{
				Name:                name,
				Paths:               conflictingPaths,
				SuggestedSourcePath: suggestedCodexSourcePath(conflictingPaths),
			}
		}
		out = append(out, candidate)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == out[j].Name {
			return out[i].SourcePath < out[j].SourcePath
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

type SkillNameConflictError struct {
	Name                string
	Paths               []string
	SuggestedSourcePath string
}

func (err *SkillNameConflictError) Error() string {
	message := fmt.Sprintf(
		"multiple different Skills use the name %q; conflicting repository paths: %s; install a specific repository path",
		err.Name,
		strings.Join(err.Paths, ", "),
	)
	if err.SuggestedSourcePath != "" {
		message += "; suggested Codex source path: " + err.SuggestedSourcePath
	}
	return message
}

func suggestedCodexSourcePath(paths []string) string {
	for _, path := range paths {
		parts := strings.Split(filepath.ToSlash(path), "/")
		for index, part := range parts {
			if strings.EqualFold(part, "skills-codex") {
				return strings.Join(parts[:index+1], "/")
			}
		}
	}
	return ""
}

func secureDiscoveryBase(root, sourcePath string) (string, string, error) {
	normalized, err := normalizePortableRelativePath(sourcePath)
	if err != nil {
		return "", "", fmt.Errorf("invalid repository source path: %w", err)
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return "", "", err
	}
	absoluteRoot = filepath.Clean(absoluteRoot)
	rootInfo, err := os.Lstat(absoluteRoot)
	if err != nil {
		return "", "", err
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return "", "", errors.New("repository root must be a real directory, not a symbolic link")
	}
	base := absoluteRoot
	if normalized != "" {
		base = filepath.Join(absoluteRoot, filepath.FromSlash(normalized))
	}
	if err := ensurePathWithin(absoluteRoot, base); err != nil {
		return "", "", err
	}
	current := absoluteRoot
	for _, part := range strings.Split(normalized, "/") {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return "", "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", "", fmt.Errorf("repository source path contains a symbolic link: %s", current)
		}
	}
	baseInfo, err := os.Lstat(base)
	if err != nil {
		return "", "", err
	}
	if baseInfo.Mode()&os.ModeSymlink != 0 || !baseInfo.IsDir() {
		return "", "", errors.New("repository source path must be a real directory")
	}
	resolvedRoot, err := filepath.EvalSymlinks(absoluteRoot)
	if err != nil {
		return "", "", err
	}
	resolvedBase, err := filepath.EvalSymlinks(base)
	if err != nil {
		return "", "", err
	}
	if err := ensurePathWithin(resolvedRoot, resolvedBase); err != nil {
		return "", "", errors.New("repository source path resolves outside the repository root")
	}
	return absoluteRoot, base, nil
}

func ensurePathWithin(root, target string) error {
	relative, err := filepath.Rel(root, target)
	if err != nil {
		return err
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) ||
		filepath.IsAbs(relative) {
		return errors.New("repository source path is outside the repository root")
	}
	return nil
}

func sameCandidateFiles(a, b []model.FileRecord) bool {
	if len(a) != len(b) {
		return false
	}
	hashes := make(map[string]string, len(a))
	for _, file := range a {
		hashes[file.Path] = file.SHA256
	}
	for _, file := range b {
		if hashes[file.Path] != file.SHA256 {
			return false
		}
	}
	return true
}

func preferredCandidatePath(candidate, current string) bool {
	candidateDepth := strings.Count(filepath.ToSlash(candidate), "/")
	currentDepth := strings.Count(filepath.ToSlash(current), "/")
	if candidateDepth != currentDepth {
		return candidateDepth < currentDepth
	}
	return len(candidate) < len(current) || (len(candidate) == len(current) && candidate < current)
}

func (c *Client) getJSON(ctx context.Context, endpoint string, out any, force bool) (bool, error) {
	c.mu.Lock()
	cached, hasCache := c.cache[endpoint]
	if hasCache && !force && time.Now().Before(cached.ExpiresAt) {
		body := append([]byte(nil), cached.Body...)
		c.mu.Unlock()
		return true, json.Unmarshal(body, out)
	}
	c.mu.Unlock()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, err
	}
	c.authorize(req)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if hasCache && cached.ETag != "" && !force {
		req.Header.Set("If-None-Match", cached.ETag)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified && hasCache {
		c.mu.Lock()
		cached.ExpiresAt = time.Now().Add(5 * time.Minute)
		c.cache[endpoint] = cached
		c.mu.Unlock()
		return true, json.Unmarshal(cached.Body, out)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return false, apiError(resp, body)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(body, out); err != nil {
		return false, err
	}
	c.mu.Lock()
	c.cache[endpoint] = cacheEntry{Body: body, ETag: resp.Header.Get("ETag"), ExpiresAt: time.Now().Add(5 * time.Minute)}
	c.mu.Unlock()
	return false, nil
}

func (c *Client) authorize(req *http.Request) {
	if token := githubToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

func githubToken() string {
	token, _ := githubTokenWithSource()
	return token
}

func githubTokenWithSource() (string, string) {
	for _, name := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value, name
		}
	}
	if path, err := exec.LookPath("gh"); err == nil {
		cmd := exec.Command(path, "auth", "token")
		processutil.ConfigureBackground(cmd)
		cmd.Env = os.Environ()
		if data, err := cmd.Output(); err == nil {
			if token := strings.TrimSpace(string(data)); token != "" {
				return token, "gh"
			}
		}
	}
	token, _ := auth.ReadGitHubToken()
	if token = strings.TrimSpace(token); token != "" {
		return token, "windows-credential-manager"
	}
	return "", "none"
}

func (c *Client) ValidateCredentials(ctx context.Context) model.GitHubCredentialStatus {
	token, source := githubTokenWithSource()
	status := model.GitHubCredentialStatus{Configured: token != "", Source: source}
	if token == "" {
		status.Message = "未配置 GitHub 凭据；公共请求将使用共享 IP 限额"
		return status
	}
	var user struct {
		Login string `json:"login"`
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
	if err != nil {
		status.Message = err.Error()
		return status
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := c.http.Do(req)
	if err != nil {
		status.Message = err.Error()
		return status
	}
	defer resp.Body.Close()
	applyRateHeaders(resp, &status.Limit, &status.Remaining, &status.ResetAt)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		status.Message = apiError(resp, body).Error()
		return status
	}
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		status.Message = err.Error()
		return status
	}
	status.Authenticated = true
	status.Login = user.Login
	status.Message = "GitHub 凭据有效"
	return status
}

func apiError(resp *http.Response, body []byte) error {
	var payload struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal(body, &payload)
	err := &APIError{StatusCode: resp.StatusCode, Status: resp.Status, Message: strings.TrimSpace(payload.Message)}
	applyRateHeaders(resp, &err.Limit, &err.Remaining, &err.ResetAt)
	if retry := strings.TrimSpace(resp.Header.Get("Retry-After")); retry != "" {
		if seconds, parseErr := strconv.Atoi(retry); parseErr == nil {
			at := time.Now().UTC().Add(time.Duration(seconds) * time.Second)
			err.RetryAt = &at
		} else if at, parseErr := http.ParseTime(retry); parseErr == nil {
			value := at.UTC()
			err.RetryAt = &value
		}
	}
	if err.RetryAt == nil && err.Remaining == 0 && err.ResetAt != nil {
		err.RetryAt = err.ResetAt
	}
	return err
}

func applyRateHeaders(resp *http.Response, limit, remaining *int, resetAt **time.Time) {
	*limit, _ = strconv.Atoi(resp.Header.Get("X-RateLimit-Limit"))
	*remaining, _ = strconv.Atoi(resp.Header.Get("X-RateLimit-Remaining"))
	if epoch, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil && epoch > 0 {
		value := time.Unix(epoch, 0).UTC()
		*resetAt = &value
	}
}

func extractZip(data []byte, dest string) (string, error) {
	_, limits := archiveLimits(defaultMaxArchiveEntryBytes)
	return extractZipWithLimits(data, dest, limits)
}

func archiveLimits(maxBytes int64) (int64, zipExtractionLimits) {
	if maxBytes < 1 {
		maxBytes = defaultMaxArchiveEntryBytes
	}
	return scaledByteLimit(maxBytes, archiveDownloadMultiplier, minArchiveDownloadBytes),
		zipExtractionLimits{
			MaxEntries:    maxArchiveEntries,
			MaxEntryBytes: maxBytes,
			MaxTotalBytes: scaledByteLimit(
				maxBytes,
				archiveUncompressedMultiplier,
				minArchiveUncompressedBytes,
			),
		}
}

func scaledByteLimit(value, multiplier, minimum int64) int64 {
	if value > math.MaxInt64/multiplier {
		return math.MaxInt64
	}
	scaled := value * multiplier
	if scaled < minimum {
		return minimum
	}
	return scaled
}

func limitWithSentinel(limit int64) int64 {
	if limit == math.MaxInt64 {
		return limit
	}
	return limit + 1
}

func extractZipWithLimits(data []byte, dest string, limits zipExtractionLimits) (string, error) {
	if limits.MaxEntries < 1 || limits.MaxEntryBytes < 1 || limits.MaxTotalBytes < 1 {
		return "", errors.New("invalid archive extraction limits")
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", err
	}
	if len(zr.File) > limits.MaxEntries {
		return "", fmt.Errorf("archive entry count exceeds limit %d", limits.MaxEntries)
	}
	base, err := filepath.Abs(dest)
	if err != nil {
		return "", err
	}
	base = filepath.Clean(base)
	entries, top, err := prepareZipEntries(zr.File, base, limits)
	if err != nil {
		return "", err
	}
	var totalBytes int64
	for _, entry := range entries {
		f := entry.file
		target := entry.target
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o700); err != nil {
				return "", err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return "", err
		}
		written, err := extractZipFile(f, target, totalBytes, limits)
		if err != nil {
			return "", err
		}
		totalBytes += written
	}
	return filepath.Join(base, top), nil
}

func prepareZipEntries(files []*zip.File, base string, limits zipExtractionLimits) ([]preparedZipEntry, string, error) {
	entries := make([]preparedZipEntry, 0, len(files))
	entryKinds := make(map[string]bool, len(files))
	var top string
	var declaredTotal int64
	for _, f := range files {
		rawName := f.Name
		if f.FileInfo().IsDir() {
			rawName = strings.TrimSuffix(rawName, "/")
		}
		normalized, err := normalizePortableRelativePath(rawName)
		if err != nil {
			return nil, "", fmt.Errorf("unsafe archive path %q: %w", f.Name, err)
		}
		clean := filepath.Clean(filepath.FromSlash(normalized))
		parts := strings.Split(normalized, "/")
		if len(parts) == 0 || parts[0] == "" {
			return nil, "", fmt.Errorf("invalid archive entry: %s", f.Name)
		}
		if top == "" {
			top = parts[0]
		} else if top != parts[0] {
			return nil, "", errors.New("archive has multiple top-level roots")
		}
		if f.Mode()&os.ModeSymlink != 0 {
			return nil, "", fmt.Errorf("symbolic link is forbidden: %s", f.Name)
		}
		isDir := f.FileInfo().IsDir()
		if !isDir && !f.Mode().IsRegular() {
			return nil, "", fmt.Errorf("non-regular archive entry is forbidden: %s", f.Name)
		}
		key := strings.ToLower(normalized)
		if _, duplicate := entryKinds[key]; duplicate {
			return nil, "", fmt.Errorf(
				"archive contains a duplicate or case-conflicting path: %s",
				f.Name,
			)
		}
		entryKinds[key] = isDir
		if !isDir {
			if f.UncompressedSize64 > uint64(limits.MaxEntryBytes) {
				return nil, "", fmt.Errorf(
					"archive entry %q exceeds uncompressed size limit %d bytes",
					f.Name,
					limits.MaxEntryBytes,
				)
			}
			if f.UncompressedSize64 > uint64(limits.MaxTotalBytes-declaredTotal) {
				return nil, "", fmt.Errorf(
					"archive uncompressed size exceeds total limit %d bytes",
					limits.MaxTotalBytes,
				)
			}
			declaredTotal += int64(f.UncompressedSize64)
		}
		target := filepath.Join(base, clean)
		resolved, err := filepath.Abs(target)
		if err != nil {
			return nil, "", err
		}
		relative, err := filepath.Rel(base, resolved)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
			return nil, "", fmt.Errorf("archive escaped staging root: %s", f.Name)
		}
		entries = append(entries, preparedZipEntry{
			file: f, target: resolved, key: key, isDir: isDir,
		})
	}
	if top == "" {
		return nil, "", errors.New("empty repository archive")
	}
	for _, entry := range entries {
		for ancestor := pathpkg.Dir(entry.key); ancestor != "."; ancestor = pathpkg.Dir(ancestor) {
			if isDir, exists := entryKinds[ancestor]; exists && !isDir {
				return nil, "", fmt.Errorf(
					"archive path conflicts with a file entry: %s",
					entry.file.Name,
				)
			}
		}
	}
	return entries, top, nil
}

func extractZipFile(f *zip.File, target string, totalBytes int64, limits zipExtractionLimits) (int64, error) {
	remainingTotal := limits.MaxTotalBytes - totalBytes
	streamLimit := limits.MaxEntryBytes
	if remainingTotal < streamLimit {
		streamLimit = remainingTotal
	}
	rc, err := f.Open()
	if err != nil {
		return 0, err
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		_ = rc.Close()
		return 0, err
	}
	written, copyErr := io.Copy(out, io.LimitReader(rc, limitWithSentinel(streamLimit)))
	closeErr := out.Close()
	rcErr := rc.Close()

	var resultErr error
	switch {
	case written > limits.MaxEntryBytes:
		resultErr = fmt.Errorf(
			"archive entry %q exceeds uncompressed size limit %d bytes while extracting",
			f.Name,
			limits.MaxEntryBytes,
		)
	case written > remainingTotal:
		resultErr = fmt.Errorf(
			"archive uncompressed size exceeds total limit %d bytes while extracting",
			limits.MaxTotalBytes,
		)
	case copyErr != nil:
		resultErr = copyErr
	case closeErr != nil:
		resultErr = closeErr
	case rcErr != nil:
		resultErr = rcErr
	}
	if resultErr != nil {
		// A failed extraction may leave one partial regular file. Remove only
		// that exact contained target; never recursively clean the staging tree.
		_ = os.Remove(target)
		return 0, resultErr
	}
	return written, nil
}
