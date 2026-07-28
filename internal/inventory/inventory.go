package inventory

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bme-lyh/Codex-Skill-Manager/internal/model"
	"gopkg.in/yaml.v3"
)

type frontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

func Discover(root string, lock model.SourcesLock) ([]model.Skill, []model.Group, []model.Relation, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, nil, nil, err
	}
	managed := lockIndex(lock)
	var skills []model.Skill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name())
		system := entry.Name() == ".system"
		if system {
			children, _ := os.ReadDir(path)
			for _, child := range children {
				if !child.IsDir() {
					continue
				}
				skill, err := readSkill(filepath.Join(path, child.Name()), managed, true)
				if err == nil {
					skills = append(skills, skill)
				}
			}
			continue
		}
		skill, err := readSkill(path, managed, false)
		if err == nil {
			skills = append(skills, skill)
		}
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
	groups := buildGroups(skills)
	relations := inferRelations(skills)
	return skills, groups, relations, nil
}

type managedRecord struct {
	GroupID   string
	GroupName string
	Package   model.PackageLock
	Skill     model.SkillLock
}

func lockIndex(lock model.SourcesLock) map[string]managedRecord {
	out := map[string]managedRecord{}
	for id, pkg := range lock.Packages {
		groupName := pkg.GroupName
		if groupName == "" {
			groupName = pkg.Repository
		}
		for name, skill := range pkg.Skills {
			out[name] = managedRecord{GroupID: id, GroupName: groupName, Package: pkg, Skill: skill}
		}
	}
	return out
}

func readSkill(path string, managed map[string]managedRecord, system bool) (model.Skill, error) {
	skillFile := filepath.Join(path, "SKILL.md")
	fm, err := ReadFrontmatter(skillFile)
	if err != nil {
		return model.Skill{}, err
	}
	files, err := HashTree(path)
	if err != nil {
		return model.Skill{}, err
	}
	name := fm.Name
	if name == "" {
		name = filepath.Base(path)
	}
	skill := model.Skill{
		Name: name, Description: strings.TrimSpace(fm.Description), Path: path,
		Files: files, System: system, SecurityStatus: "not-scanned", UpdateStatus: "unknown",
	}
	if system {
		skill.GroupID, skill.GroupName = "system", "Codex 系统 Skills"
		skill.SourceGroupID, skill.SourceGroupName = skill.GroupID, skill.GroupName
		skill.SourceProvider, skill.SourceConfidence, skill.SourceEvidence = "system", 1, "Codex 内置系统目录"
		skill.Managed = true
		return skill, nil
	}
	if rec, ok := managed[name]; ok {
		skill.Managed = true
		skill.GroupID, skill.GroupName = rec.GroupID, rec.GroupName
		skill.SourceGroupID, skill.SourceGroupName = rec.GroupID, rec.GroupName
		skill.SourceProvider, skill.SourceConfidence, skill.SourceEvidence = rec.Package.Provider, 1, "来源锁记录"
		skill.SourceRepository = rec.Package.Repository
		skill.SourcePath = rec.Skill.SourcePath
		skill.InstalledCommit = rec.Skill.ResolvedCommit
		if skill.InstalledCommit == "" {
			skill.InstalledCommit = rec.Package.ResolvedCommit
		}
		skill.LocalModified = filesChanged(files, rec.Skill.Files)
	} else {
		skill.GroupID, skill.GroupName = "unmanaged", "未管理 Skills"
		skill.SourceGroupID, skill.SourceGroupName = skill.GroupID, skill.GroupName
		skill.SourceProvider, skill.SourceConfidence, skill.SourceEvidence = "local", 0, "尚未分析来源"
	}
	return skill, nil
}

func ReadFrontmatter(path string) (frontmatter, error) {
	f, err := os.Open(path)
	if err != nil {
		return frontmatter{}, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(io.LimitReader(f, 1<<20))
	if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != "---" {
		return frontmatter{}, fmt.Errorf("%s: missing YAML frontmatter", path)
	}
	var lines []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			var fm frontmatter
			if err := yaml.Unmarshal([]byte(strings.Join(lines, "\n")), &fm); err != nil {
				return frontmatter{}, err
			}
			if strings.TrimSpace(fm.Name) == "" || strings.TrimSpace(fm.Description) == "" {
				return frontmatter{}, fmt.Errorf("%s: name and description are required", path)
			}
			return fm, nil
		}
		lines = append(lines, line)
	}
	return frontmatter{}, fmt.Errorf("%s: unterminated frontmatter", path)
}

func HashTree(root string) ([]model.FileRecord, error) {
	var files []model.FileRecord
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic link is not allowed: %s", path)
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		h := sha256.New()
		_, copyErr := io.Copy(h, f)
		closeErr := f.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, model.FileRecord{
			Path: filepath.ToSlash(rel), Size: info.Size(),
			SHA256: hex.EncodeToString(h.Sum(nil)), Kind: strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), "."),
		})
		return nil
	})
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, err
}

func filesChanged(files []model.FileRecord, baseline map[string]string) bool {
	if len(files) != len(baseline) {
		return true
	}
	for _, f := range files {
		if baseline[f.Path] != f.SHA256 {
			return true
		}
	}
	return false
}

func buildGroups(skills []model.Skill) []model.Group {
	m := map[string]*model.Group{}
	for _, skill := range skills {
		g, ok := m[skill.GroupID]
		if !ok {
			provider := skill.SourceProvider
			if provider == "" {
				provider = "github"
			}
			if skill.System {
				provider = "system"
			} else if !skill.Managed {
				provider = "local"
			}
			g = &model.Group{
				ID: skill.GroupID, Name: skill.GroupName, Provider: provider,
				Repository: skill.SourceRepository, ReadOnly: skill.System,
				Position: len(m), Status: "healthy",
			}
			m[skill.GroupID] = g
		}
		g.SkillNames = append(g.SkillNames, skill.Name)
	}
	var out []model.Group
	for _, g := range m {
		sort.Strings(g.SkillNames)
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func inferRelations(skills []model.Skill) []model.Relation {
	names := map[string]bool{}
	for _, skill := range skills {
		names[skill.Name] = true
	}
	seen := map[string]bool{}
	var out []model.Relation
	for _, skill := range skills {
		data, err := os.ReadFile(filepath.Join(skill.Path, "SKILL.md"))
		if err != nil {
			continue
		}
		text := strings.ToLower(string(data))
		for name := range names {
			if name == skill.Name || !strings.Contains(text, strings.ToLower(name)) {
				continue
			}
			key := skill.Name + "\x00" + name
			if !seen[key] {
				seen[key] = true
				out = append(out, model.Relation{From: skill.Name, To: name, Type: "workflow", Confidence: 0.8, Evidence: "SKILL.md 显式提及"})
			}
		}
	}
	return out
}
