package provenance

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/bme-lyh/Codex-Skill-Manager/internal/model"
)

type catalogSource struct {
	repository string
	sourcePath string
}

var knownSources = map[string]catalogSource{
	"build-graph":      {"tirth8205/code-review-graph", "skills/build-graph"},
	"debug-issue":      {"tirth8205/code-review-graph", "skills/debug-issue"},
	"explore-codebase": {"tirth8205/code-review-graph", "skills/explore-codebase"},
	"refactor-safely":  {"tirth8205/code-review-graph", "skills/refactor-safely"},
	"review-changes":   {"tirth8205/code-review-graph", "skills/review-changes"},
	"review-delta":     {"tirth8205/code-review-graph", "skills/review-delta"},
	"review-pr":        {"tirth8205/code-review-graph", "skills/review-pr"},
	"job-hunt":         {"rebecha1227-a11y/CareerForge", "skills/job-hunt"},
	"resume-match":     {"rebecha1227-a11y/CareerForge", "skills/resume-match"},
	"resume-craft":     {"rebecha1227-a11y/CareerForge", "skills/resume-craft"},
	"cover-letter":     {"rebecha1227-a11y/CareerForge", "skills/cover-letter"},
	"mock-interview":   {"rebecha1227-a11y/CareerForge", "skills/mock-interview"},
	"offer-decision":   {"rebecha1227-a11y/CareerForge", "skills/offer-decision"},
}

var (
	githubURLPattern = regexp.MustCompile(`(?i)https://github\.com/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)([^\s\)\]>"']*)`)
	metadataPattern  = regexp.MustCompile(`(?im)^\s*(repository|source|homepage)\s*:\s*['"]?(https://github\.com/[^\s'"]+)`)
	gitRemotePattern = regexp.MustCompile(`(?im)^\s*url\s*=\s*(?:https://github\.com/|git@github\.com:)([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+?)(?:\.git)?\s*$`)
)

func Detect(skill model.Skill) model.DetectedSource {
	return detectInRoot("", skill)
}

func DetectInRoot(rootID string, skill model.Skill) model.DetectedSource {
	return detectInRoot(strings.TrimSpace(rootID), skill)
}

func DetectRoot(rootID string, skill model.Skill) model.DetectedSource {
	return DetectInRoot(rootID, skill)
}

func DetectRoots(skills []model.Skill) []model.DetectedSource {
	out := make([]model.DetectedSource, 0, len(skills))
	for _, skill := range skills {
		out = append(out, DetectInRoot(skill.RootID, skill))
	}
	return out
}

func detectInRoot(rootID string, skill model.Skill) model.DetectedSource {
	return qualifyRoot(rootID, detectRaw(skill))
}

func detectRaw(skill model.Skill) model.DetectedSource {
	if known, ok := knownSources[skill.Name]; ok {
		return githubSource(skill.Name, known.repository, "main", known.sourcePath, 1,
			"匹配内置可信来源目录")
	}

	skillFile := filepath.Join(skill.Path, "SKILL.md")
	content, _ := os.ReadFile(skillFile)
	if match := metadataPattern.FindSubmatch(content); len(match) == 3 {
		if source, ok := fromGitHubURL(skill.Name, string(match[2]), .95,
			"读取 SKILL.md 中的显式来源字段"); ok {
			return source
		}
	}

	gitConfig := filepath.Join(skill.Path, ".git", "config")
	if data, err := os.ReadFile(gitConfig); err == nil {
		if match := gitRemotePattern.FindSubmatch(data); len(match) == 3 {
			repository := string(match[1]) + "/" + strings.TrimSuffix(string(match[2]), ".git")
			return githubSource(skill.Name, repository, "main", "", .9,
				"读取 Skill 目录的 Git origin")
		}
	}

	if match := githubURLPattern.Find(content); len(match) > 0 {
		if source, ok := fromGitHubURL(skill.Name, string(match), .65,
			"从 SKILL.md 中的 GitHub 链接推断；请在确认管理前核对"); ok {
			return source
		}
	}

	sum := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(skill.Path))))
	return model.DetectedSource{
		SkillName: skill.Name, Provider: "local", Repository: skill.Path,
		SourceURL: skill.Path, SourcePath: skill.Name,
		GroupID: "local:" + fmt.Sprintf("%x", sum[:8]), GroupName: "本地 · " + skill.Name,
		Confidence: .3, Evidence: "未发现可验证的远程来源，按独立本地 Skill 管理",
	}
}

func fromGitHubURL(skillName, raw string, confidence float64, evidence string) (model.DetectedSource, bool) {
	match := githubURLPattern.FindStringSubmatch(strings.TrimSpace(raw))
	if len(match) != 4 {
		return model.DetectedSource{}, false
	}
	repository := match[1] + "/" + strings.TrimSuffix(match[2], ".git")
	ref := "main"
	sourcePath := ""
	parts := strings.Split(strings.Trim(match[3], "/"), "/")
	if len(parts) >= 2 && (parts[0] == "tree" || parts[0] == "blob") {
		ref = parts[1]
		if len(parts) > 2 {
			sourcePath = strings.Join(parts[2:], "/")
			if parts[0] == "blob" && strings.EqualFold(filepath.Base(sourcePath), "SKILL.md") {
				sourcePath = filepath.ToSlash(filepath.Dir(sourcePath))
				if sourcePath == "." {
					sourcePath = ""
				}
			}
		}
	}
	return githubSource(skillName, repository, ref, sourcePath, confidence, evidence), true
}

func githubSource(skillName, repository, ref, sourcePath string, confidence float64, evidence string) model.DetectedSource {
	return model.DetectedSource{
		SkillName: skillName, Provider: "github", Repository: repository,
		SourceURL: "https://github.com/" + repository, RequestedRef: ref, SourcePath: sourcePath,
		GroupID: "github:" + strings.ToLower(repository), GroupName: repository,
		Confidence: confidence, Evidence: evidence,
	}
}

func qualifyRoot(rootID string, source model.DetectedSource) model.DetectedSource {
	if rootID == "" {
		return source
	}
	source.RootID = rootID
	if source.GroupID != "" && !strings.HasPrefix(source.GroupID, rootID+":") {
		source.GroupID = rootID + ":" + source.GroupID
	}
	return source
}
