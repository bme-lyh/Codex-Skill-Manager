package inventory

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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

// Discover keeps the v0.10 single-root contract and treats the legacy root as
// codex-default. New callers should use DiscoverRoots.
func Discover(root string, lock model.SourcesLock) ([]model.Skill, []model.Group, []model.Relation, error) {
	return DiscoverRoot(model.RootIDCodexDefault, root, lock)
}

// DiscoverRoot inventories one explicit root. Missing roots are valid before
// the first target write and therefore return empty slices rather than an
// error.
func DiscoverRoot(rootID, root string, lock model.SourcesLock) ([]model.Skill, []model.Group, []model.Relation, error) {
	return discoverRoot(rootID, root, ".system", lock)
}

// DiscoverSkillRoot applies the root's explicit system-directory policy.
func DiscoverSkillRoot(root model.SkillRoot, lock model.SourcesLock) ([]model.Skill, []model.Group, []model.Relation, error) {
	return discoverRoot(root.ID, root.Path, model.RootSystemDir(root), lock)
}

func discoverRoot(rootID, root, systemDir string, lock model.SourcesLock) ([]model.Skill, []model.Group, []model.Relation, error) {
	if strings.TrimSpace(rootID) == "" {
		rootID = model.RootIDCodexDefault
	}
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return []model.Skill{}, []model.Group{}, []model.Relation{}, nil
	}
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
		system := strings.EqualFold(entry.Name(), systemDir)
		if system {
			children, _ := os.ReadDir(path)
			for _, child := range children {
				if !child.IsDir() {
					continue
				}
				skill, err := readSkill(rootID, filepath.Join(path, child.Name()), managed, true)
				if err == nil {
					skills = append(skills, skill)
				}
			}
			continue
		}
		skill, err := readSkill(rootID, path, managed, false)
		if err == nil {
			skills = append(skills, skill)
		}
	}
	sort.Slice(skills, func(i, j int) bool { return skillSortKey(skills[i]) < skillSortKey(skills[j]) })
	groups := buildGroups(skills)
	relations := inferRelations(skills)
	return skills, groups, relations, nil
}

// DiscoverRoots inventories all enabled roots while preserving root-qualified
// Skill identity, allowing equal names under different roots.
func DiscoverRoots(roots []model.SkillRoot, lock model.SourcesLock) ([]model.Skill, []model.Group, []model.Relation, error) {
	var skills []model.Skill
	for _, root := range roots {
		if !root.Enabled {
			continue
		}
		found, _, _, err := DiscoverSkillRoot(root, lock)
		if err != nil {
			return nil, nil, nil, err
		}
		skills = append(skills, found...)
	}
	sort.Slice(skills, func(i, j int) bool { return skillSortKey(skills[i]) < skillSortKey(skills[j]) })
	return skills, buildGroups(skills), inferRelations(skills), nil
}

func DiscoverAll(roots []model.SkillRoot, lock model.SourcesLock) ([]model.Skill, []model.Group, []model.Relation, error) {
	return DiscoverRoots(roots, lock)
}

func DiscoverConfigured(roots []model.SkillRoot, lock model.SourcesLock) ([]model.Skill, []model.Group, []model.Relation, error) {
	return DiscoverRoots(roots, lock)
}

// FindManaged resolves one root-qualified Skill from a v2 lock. The bool is
// false for ambiguous legacy name-only entries, enforcing fail-closed lookup.
func FindManaged(lock model.SourcesLock, rootID, name string) (model.PackageLock, model.SkillLock, bool) {
	managed := lockIndex(lock)
	rec, ok := managed[model.QualifiedSkillIdentity(rootID, name)]
	if !ok && rootID == "" {
		rec, ok = managed[name]
	}
	if !ok {
		return model.PackageLock{}, model.SkillLock{}, false
	}
	return rec.Package, rec.Skill, true
}

type managedRecord struct {
	RootID    string
	GroupID   string
	GroupName string
	Package   model.PackageLock
	Skill     model.SkillLock
}

func lockIndex(lock model.SourcesLock) map[string]managedRecord {
	out := map[string]managedRecord{}
	legacy := map[string]managedRecord{}
	ambiguous := map[string]bool{}
	for id, pkg := range lock.Packages {
		rootID := strings.TrimSpace(pkg.RootID)
		groupID := id
		if rootID == "" {
			if index := strings.IndexByte(id, '\x00'); index > 0 {
				rootID = id[:index]
				groupID = id[index+1:]
			}
		} else if prefix := rootID + "\x00"; strings.HasPrefix(id, prefix) {
			groupID = strings.TrimPrefix(id, prefix)
		}
		groupName := pkg.GroupName
		if groupName == "" {
			groupName = pkg.Repository
		}
		for name, skill := range pkg.Skills {
			rec := managedRecord{RootID: rootID, GroupID: groupID, GroupName: groupName, Package: pkg, Skill: skill}
			if rootID != "" {
				out[model.QualifiedSkillIdentity(rootID, name)] = rec
				continue
			}
			if _, exists := legacy[name]; exists {
				ambiguous[name] = true
				continue
			}
			legacy[name] = rec
		}
	}
	for name, rec := range legacy {
		if !ambiguous[name] {
			out[name] = rec
		}
	}
	return out
}

func readSkill(rootID, path string, managed map[string]managedRecord, system bool) (model.Skill, error) {
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
		Name: name, RootID: rootID, Identity: model.SkillIdentity(rootID, name),
		Description: strings.TrimSpace(fm.Description), Path: path,
		Files: files, System: system, SecurityStatus: "not-scanned", UpdateStatus: "unknown",
	}
	if system {
		skill.GroupID, skill.GroupName = "system", "Codex 系统 Skills"
		skill.SourceGroupID, skill.SourceGroupName = skill.GroupID, skill.GroupName
		skill.SourceProvider, skill.SourceConfidence, skill.SourceEvidence = "system", 1, "Codex 内置系统目录"
		skill.Managed = true
		return skill, nil
	}
	rec, ok := managed[model.QualifiedSkillIdentity(rootID, name)]
	if !ok {
		rec, ok = managed[name]
	}
	if ok {
		skill.Managed = true
		skill.GroupID, skill.GroupName = rec.GroupID, rec.GroupName
		skill.SourceGroupID, skill.SourceGroupName = rec.GroupID, rec.GroupName
		skill.SourceProvider, skill.SourceConfidence, skill.SourceEvidence = rec.Package.Provider, 1, "来源锁记录"
		skill.SourceAssociation = model.NormalizeSourceAssociation(rec.Package.Provider, rec.Package.SourceAssociation)
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
		skill.SourceAssociation = model.SourceAssociationUnlinked
	}
	return skill, nil
}

func skillSortKey(skill model.Skill) string {
	if skill.Identity != "" {
		return skill.Identity
	}
	return model.SkillIdentity(skill.RootID, skill.Name)
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
		groupKey := model.QualifiedSkillIdentity(skill.RootID, skill.GroupID)
		g, ok := m[groupKey]
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
				ID: skill.GroupID, RootID: skill.RootID, Name: skill.GroupName, Provider: provider,
				Repository: skill.SourceRepository, ReadOnly: skill.System,
				Position: len(m), Status: "healthy",
			}
			m[groupKey] = g
		}
		g.SkillNames = append(g.SkillNames, skill.Name)
	}
	var out []model.Group
	for _, g := range m {
		sort.Strings(g.SkillNames)
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].RootID < out[j].RootID
	})
	return out
}

func inferRelations(skills []model.Skill) []model.Relation {
	names := map[string]bool{}
	for _, skill := range skills {
		names[model.QualifiedSkillIdentity(skill.RootID, skill.Name)] = true
	}
	seen := map[string]bool{}
	var out []model.Relation
	for _, skill := range skills {
		data, err := os.ReadFile(filepath.Join(skill.Path, "SKILL.md"))
		if err != nil {
			continue
		}
		text := strings.ToLower(string(data))
		for identity := range names {
			parts := strings.SplitN(identity, "\x00", 2)
			if len(parts) != 2 {
				continue
			}
			rootID, name := parts[0], parts[1]
			if rootID != skill.RootID || name == skill.Name || !strings.Contains(text, strings.ToLower(name)) {
				continue
			}
			key := model.QualifiedSkillIdentity(skill.RootID, skill.Name) + "\x00" + identity
			if !seen[key] {
				seen[key] = true
				out = append(out, model.Relation{From: skill.Name, To: name, Type: "workflow", Confidence: 0.8, Evidence: "SKILL.md 显式提及"})
			}
		}
	}
	return out
}
