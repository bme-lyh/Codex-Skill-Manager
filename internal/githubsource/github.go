package githubsource

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
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
	parts := strings.Split(strings.Trim(strings.TrimSuffix(u.Path, ".git"), "/"), "/")
	if len(parts) < 2 || !safePart.MatchString(parts[0]) || !safePart.MatchString(parts[1]) {
		return ParsedURL{}, errors.New("invalid GitHub repository URL")
	}
	out := ParsedURL{Owner: parts[0], Repo: parts[1]}
	if len(parts) >= 4 && (parts[2] == "tree" || parts[2] == "blob") {
		out.Ref = parts[3]
		if len(parts) > 4 {
			out.SourcePath = strings.Join(parts[4:], "/")
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("https://api.github.com/repos/%s/zipball/%s", repo.FullName, repo.CommitSHA), nil)
	if err != nil {
		return "", err
	}
	c.authorize(req)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("GitHub archive: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	limit := maxBytes * 10
	if limit < 100<<20 {
		limit = 100 << 20
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return "", err
	}
	if int64(len(data)) > limit {
		return "", errors.New("repository archive exceeds configured limit")
	}
	if err := os.MkdirAll(dest, 0o700); err != nil {
		return "", err
	}
	return extractZip(data, dest)
}

func Discover(root, sourcePath string) ([]model.CandidateSkill, error) {
	base := root
	if sourcePath != "" {
		base = filepath.Join(root, filepath.FromSlash(sourcePath))
	}
	var out []model.CandidateSkill
	err := filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
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
	byName := map[string]model.CandidateSkill{}
	for _, candidate := range out {
		existing, duplicate := byName[candidate.Name]
		if !duplicate {
			byName[candidate.Name] = candidate
			continue
		}
		if !sameCandidateFiles(existing.Files, candidate.Files) {
			return nil, fmt.Errorf("multiple different Skills use the name %q; install a specific repository path", candidate.Name)
		}
		if preferredCandidatePath(candidate.SourcePath, existing.SourcePath) {
			byName[candidate.Name] = candidate
		}
	}
	out = out[:0]
	for _, candidate := range byName {
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
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", err
	}
	base, err := filepath.Abs(dest)
	if err != nil {
		return "", err
	}
	base = filepath.Clean(base)
	var top string
	for _, f := range zr.File {
		// Reject traversal syntax in the untrusted archive name before it
		// reaches filepath.Join or any filesystem operation. The resolved-path
		// check below remains a second, independent containment boundary.
		if strings.Contains(f.Name, "..") {
			return "", fmt.Errorf("archive path traversal: %s", f.Name)
		}
		clean := filepath.Clean(filepath.FromSlash(f.Name))
		if clean == "" || clean == "." || filepath.IsAbs(clean) || filepath.VolumeName(clean) != "" {
			return "", fmt.Errorf("archive path traversal: %s", f.Name)
		}
		parts := strings.Split(filepath.ToSlash(clean), "/")
		if len(parts) == 0 || parts[0] == "" {
			return "", fmt.Errorf("invalid archive entry: %s", f.Name)
		}
		if top == "" {
			top = parts[0]
		} else if top != parts[0] {
			return "", errors.New("archive has multiple top-level roots")
		}
		if f.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("symbolic link is forbidden: %s", f.Name)
		}
		target := filepath.Join(base, clean)
		resolved, err := filepath.Abs(target)
		if err != nil {
			return "", err
		}
		relative, err := filepath.Rel(base, resolved)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
			return "", fmt.Errorf("archive escaped staging root: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o700); err != nil {
				return "", err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return "", err
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			rc.Close()
			return "", err
		}
		_, copyErr := io.Copy(out, rc)
		closeErr := out.Close()
		rcErr := rc.Close()
		if copyErr != nil {
			return "", copyErr
		}
		if closeErr != nil {
			return "", closeErr
		}
		if rcErr != nil {
			return "", rcErr
		}
	}
	if top == "" {
		return "", errors.New("empty repository archive")
	}
	return filepath.Join(base, top), nil
}
