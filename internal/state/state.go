package state

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/bme-lyh/Codex-Skill-Manager/internal/model"
)

type Store struct {
	db       *sql.DB
	dataRoot string
	lockPath string
	roots    []model.SkillRoot
}

func Open(dataRoot string) (*Store, error) {
	return OpenWithRoots(dataRoot, nil)
}

// OpenWithRoots supplies root paths used only for v1 source-lock migration.
// It never creates those roots; writes remain the manager's explicit target
// operation.
func OpenWithRoots(dataRoot string, roots []model.SkillRoot) (*Store, error) {
	if err := os.MkdirAll(dataRoot, 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.Join(dataRoot, "state.db"))
	if err != nil {
		return nil, err
	}
	s := &Store{db: db, dataRoot: dataRoot, lockPath: filepath.Join(dataRoot, "sources.lock.json"), roots: append([]model.SkillRoot(nil), roots...)}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
PRAGMA journal_mode=WAL;
CREATE TABLE IF NOT EXISTS transactions (
  id TEXT PRIMARY KEY, type TEXT NOT NULL, status TEXT NOT NULL,
  started_at TEXT NOT NULL, completed_at TEXT, payload_json TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS scan_reports (
  id TEXT PRIMARY KEY, target TEXT NOT NULL, severity TEXT NOT NULL,
  created_at TEXT NOT NULL, payload_json TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS approvals (
  id INTEGER PRIMARY KEY AUTOINCREMENT, plan_id TEXT NOT NULL,
  decision TEXT NOT NULL, reason TEXT, created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS finding_ignores (
  fingerprint TEXT PRIMARY KEY, rule_id TEXT NOT NULL, file TEXT NOT NULL,
  reason TEXT, created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS skill_groups (
  root_id TEXT NOT NULL DEFAULT '', id TEXT NOT NULL, name TEXT NOT NULL, position INTEGER NOT NULL,
  manual INTEGER NOT NULL, updated_at TEXT NOT NULL
  ,PRIMARY KEY(root_id,id)
);
CREATE TABLE IF NOT EXISTS skill_group_assignments (
  root_id TEXT NOT NULL DEFAULT '', skill_name TEXT NOT NULL, group_id TEXT NOT NULL,
  position INTEGER NOT NULL, updated_at TEXT NOT NULL
  ,PRIMARY KEY(root_id,skill_name)
);
CREATE TABLE IF NOT EXISTS update_statuses (
  root_id TEXT NOT NULL DEFAULT '', group_id TEXT NOT NULL, checked_at TEXT NOT NULL, payload_json TEXT NOT NULL,
  PRIMARY KEY(root_id,group_id)
);
CREATE TABLE IF NOT EXISTS skill_security_states (
  root_id TEXT NOT NULL DEFAULT '', skill_name TEXT NOT NULL, content_hash TEXT NOT NULL,
  report_id TEXT NOT NULL, checked_at TEXT NOT NULL
  ,PRIMARY KEY(root_id,skill_name)
);
`)
	if err != nil {
		return err
	}
	return s.migrateRootNamespaceTables()
}

// migrateRootNamespaceTables upgrades databases created by v0.10, whose
// security/layout tables used skill_name as a global key. The copy is purely
// database state; no Skill files are moved or rewritten.
func (s *Store) migrateRootNamespaceTables() error {
	if err := s.upgradeUpdateStatusesTable(); err != nil {
		return err
	}
	if err := s.upgradeSecurityStatesTable(); err != nil {
		return err
	}
	if err := s.upgradeGroupTables(); err != nil {
		return err
	}
	return nil
}

// upgradeUpdateStatusesTable namespaces persisted update results by root. v0.11
// stored the root-qualified package key in group_id, which made the dashboard
// unable to join it with the inventory's human-readable group ID. The migration
// keeps the payload and normalizes that key into root_id + group_id.
func (s *Store) upgradeUpdateStatusesTable() error {
	rows, err := s.db.Query(`PRAGMA table_info(update_statuses)`)
	if err != nil {
		return err
	}
	hasRoot := false
	for rows.Next() {
		var cid, notNull, pk int
		var name, typ string
		var def any
		if err := rows.Scan(&cid, &name, &typ, &notNull, &def, &pk); err != nil {
			rows.Close()
			return err
		}
		if name == "root_id" {
			hasRoot = true
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if hasRoot {
		return nil
	}
	type legacyUpdateStatus struct {
		groupID   string
		checkedAt string
		payload   string
	}
	legacyRows, err := s.db.Query(`SELECT group_id,checked_at,payload_json FROM update_statuses`)
	if err != nil {
		return err
	}
	legacy := []legacyUpdateStatus{}
	for legacyRows.Next() {
		var item legacyUpdateStatus
		if err := legacyRows.Scan(&item.groupID, &item.checkedAt, &item.payload); err != nil {
			legacyRows.Close()
			return err
		}
		legacy = append(legacy, item)
	}
	if err := legacyRows.Close(); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if _, err = tx.Exec(`CREATE TABLE update_statuses_v2 (
root_id TEXT NOT NULL DEFAULT '', group_id TEXT NOT NULL, checked_at TEXT NOT NULL,
payload_json TEXT NOT NULL, PRIMARY KEY(root_id,group_id))`); err != nil {
		_ = tx.Rollback()
		return err
	}
	for _, item := range legacy {
		rootID, groupID := "", item.groupID
		var status model.UpdateStatus
		if json.Unmarshal([]byte(item.payload), &status) == nil {
			rootID = strings.TrimSpace(status.RootID)
			groupID = strings.TrimSpace(status.GroupID)
		}
		if index := strings.IndexByte(groupID, '\x00'); index > 0 {
			if rootID == "" {
				rootID = groupID[:index]
			}
			groupID = groupID[index+1:]
		}
		if groupID == "" {
			groupID = item.groupID
		}
		if _, err = tx.Exec(`INSERT INTO update_statuses_v2(root_id,group_id,checked_at,payload_json) VALUES(?,?,?,?)`,
			rootID, groupID, item.checkedAt, item.payload); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	if _, err = tx.Exec(`DROP TABLE update_statuses`); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err = tx.Exec(`ALTER TABLE update_statuses_v2 RENAME TO update_statuses`); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (s *Store) upgradeSecurityStatesTable() error {
	rows, err := s.db.Query(`PRAGMA table_info(skill_security_states)`)
	if err != nil {
		return err
	}
	hasRoot := false
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull, pk int
		var def any
		if err := rows.Scan(&cid, &name, &typ, &notNull, &def, &pk); err != nil {
			rows.Close()
			return err
		}
		if name == "root_id" {
			hasRoot = true
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if hasRoot {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if _, err = tx.Exec(`CREATE TABLE skill_security_states_v2 (
root_id TEXT NOT NULL DEFAULT '', skill_name TEXT NOT NULL, content_hash TEXT NOT NULL,
report_id TEXT NOT NULL, checked_at TEXT NOT NULL, PRIMARY KEY(root_id,skill_name))`); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err = tx.Exec(`INSERT INTO skill_security_states_v2(root_id,skill_name,content_hash,report_id,checked_at)
SELECT 'codex-default',skill_name,content_hash,report_id,checked_at FROM skill_security_states`); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err = tx.Exec(`DROP TABLE skill_security_states`); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err = tx.Exec(`ALTER TABLE skill_security_states_v2 RENAME TO skill_security_states`); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (s *Store) upgradeGroupTables() error {
	// Layout tables are created with root-qualified primary keys for new DBs.
	// Older DBs are uncommon but can be upgraded transactionally in place.
	for _, table := range []string{"skill_groups", "skill_group_assignments"} {
		rows, err := s.db.Query(`PRAGMA table_info(` + table + `)`)
		if err != nil {
			return err
		}
		hasRoot := false
		for rows.Next() {
			var cid, notNull, pk int
			var name, typ string
			var def any
			if err := rows.Scan(&cid, &name, &typ, &notNull, &def, &pk); err != nil {
				rows.Close()
				return err
			}
			if name == "root_id" {
				hasRoot = true
			}
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if hasRoot {
			continue
		}
		if err := s.upgradeOneGroupTable(table); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) upgradeOneGroupTable(table string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	var create, copySQL string
	if table == "skill_groups" {
		create = `CREATE TABLE skill_groups_v2 (root_id TEXT NOT NULL DEFAULT '', id TEXT NOT NULL, name TEXT NOT NULL, position INTEGER NOT NULL, manual INTEGER NOT NULL, updated_at TEXT NOT NULL, PRIMARY KEY(root_id,id))`
		copySQL = `INSERT INTO skill_groups_v2(root_id,id,name,position,manual,updated_at) SELECT 'codex-default',id,name,position,manual,updated_at FROM skill_groups`
	} else {
		create = `CREATE TABLE skill_group_assignments_v2 (root_id TEXT NOT NULL DEFAULT '', skill_name TEXT NOT NULL, group_id TEXT NOT NULL, position INTEGER NOT NULL, updated_at TEXT NOT NULL, PRIMARY KEY(root_id,skill_name))`
		copySQL = `INSERT INTO skill_group_assignments_v2(root_id,skill_name,group_id,position,updated_at) SELECT 'codex-default',skill_name,group_id,position,updated_at FROM skill_group_assignments`
	}
	if _, err = tx.Exec(create); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err = tx.Exec(copySQL); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err = tx.Exec(`DROP TABLE ` + table); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err = tx.Exec(`ALTER TABLE ` + table + `_v2 RENAME TO ` + table); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (s *Store) SaveSkillSecurityStates(states []model.SkillSecurityState) error {
	if len(states) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	for _, state := range states {
		if state.SkillName == "" || state.ContentHash == "" || state.ReportID == "" || state.CheckedAt.IsZero() {
			_ = tx.Rollback()
			return errors.New("skill security state is incomplete")
		}
		if _, err = tx.Exec(`
INSERT INTO skill_security_states(root_id,skill_name,content_hash,report_id,checked_at) VALUES(?,?,?,?,?)
ON CONFLICT(root_id,skill_name) DO UPDATE SET
content_hash=excluded.content_hash,report_id=excluded.report_id,checked_at=excluded.checked_at`,
			state.RootID, state.SkillName, state.ContentHash, state.ReportID, state.CheckedAt.Format(time.RFC3339Nano)); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// ReplaceSkillSecurityStates restores the exact state for explicit Skills in
// one root. It is used only by transaction recovery: unspecified names are
// deleted, while supplied snapshots are restored atomically.
func (s *Store) ReplaceSkillSecurityStates(rootID string, names []string, states []model.SkillSecurityState) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	for _, name := range names {
		if _, err = tx.Exec(`DELETE FROM skill_security_states WHERE root_id=? AND skill_name=?`, rootID, name); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	for _, state := range states {
		if state.RootID != rootID || state.SkillName == "" || state.ContentHash == "" || state.ReportID == "" || state.CheckedAt.IsZero() {
			_ = tx.Rollback()
			return errors.New("skill security state snapshot is invalid")
		}
		if _, err = tx.Exec(`INSERT INTO skill_security_states(root_id,skill_name,content_hash,report_id,checked_at) VALUES(?,?,?,?,?)`,
			state.RootID, state.SkillName, state.ContentHash, state.ReportID, state.CheckedAt.Format(time.RFC3339Nano)); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) SkillSecurityStates() (map[string]model.SkillSecurityState, error) {
	rows, err := s.db.Query(`SELECT root_id,skill_name,content_hash,report_id,checked_at FROM skill_security_states`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]model.SkillSecurityState{}
	for rows.Next() {
		var state model.SkillSecurityState
		var checkedAt string
		if err := rows.Scan(&state.RootID, &state.SkillName, &state.ContentHash, &state.ReportID, &checkedAt); err != nil {
			return nil, err
		}
		state.CheckedAt, err = time.Parse(time.RFC3339Nano, checkedAt)
		if err != nil {
			return nil, fmt.Errorf("parse skill security check time: %w", err)
		}
		key := state.SkillName
		if state.RootID != "" {
			key = model.QualifiedSkillIdentity(state.RootID, state.SkillName)
		}
		out[key] = state
	}
	return out, rows.Err()
}

func (s *Store) SaveUpdateStatuses(statuses []model.UpdateStatus) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	for _, status := range statuses {
		data, marshalErr := json.Marshal(status)
		if marshalErr != nil {
			_ = tx.Rollback()
			return marshalErr
		}
		if _, err = tx.Exec(`
INSERT INTO update_statuses(root_id,group_id,checked_at,payload_json) VALUES(?,?,?,?)
ON CONFLICT(root_id,group_id) DO UPDATE SET checked_at=excluded.checked_at,payload_json=excluded.payload_json`,
			status.RootID, status.GroupID, status.CheckedAt.Format(time.RFC3339Nano), string(data)); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) LatestUpdateStatuses() ([]model.UpdateStatus, error) {
	rows, err := s.db.Query(`SELECT root_id,group_id,payload_json FROM update_statuses ORDER BY checked_at DESC,root_id,group_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	statuses := []model.UpdateStatus{}
	for rows.Next() {
		var rootID, groupID, raw string
		if err := rows.Scan(&rootID, &groupID, &raw); err != nil {
			return nil, err
		}
		var status model.UpdateStatus
		if json.Unmarshal([]byte(raw), &status) == nil {
			if status.RootID == "" {
				status.RootID = rootID
			}
			if index := strings.IndexByte(status.GroupID, '\x00'); index >= 0 {
				if status.RootID == "" {
					status.RootID = status.GroupID[:index]
				}
				status.GroupID = status.GroupID[index+1:]
			}
			if status.GroupID == "" {
				status.GroupID = groupID
			}
			if status.OutdatedSkills == nil {
				status.OutdatedSkills = []string{}
			}
			statuses = append(statuses, status)
		}
	}
	return statuses, rows.Err()
}

func (s *Store) LoadGroupLayout() (model.GroupLayoutState, error) {
	layout := model.GroupLayoutState{
		Groups:      []model.GroupPreference{},
		Assignments: []model.SkillGroupAssignment{},
	}
	rows, err := s.db.Query(`SELECT root_id,id,name,position,manual FROM skill_groups ORDER BY position,name,root_id,id`)
	if err != nil {
		return layout, err
	}
	for rows.Next() {
		var group model.GroupPreference
		var manual int
		if err := rows.Scan(&group.RootID, &group.ID, &group.Name, &group.Position, &manual); err != nil {
			rows.Close()
			return layout, err
		}
		group.Manual = manual != 0
		layout.Groups = append(layout.Groups, group)
	}
	if err := rows.Close(); err != nil {
		return layout, err
	}
	assignments, err := s.db.Query(`SELECT root_id,skill_name,group_id,position FROM skill_group_assignments ORDER BY root_id,group_id,position,skill_name`)
	if err != nil {
		return layout, err
	}
	defer assignments.Close()
	for assignments.Next() {
		var assignment model.SkillGroupAssignment
		if err := assignments.Scan(&assignment.RootID, &assignment.SkillName, &assignment.GroupID, &assignment.Position); err != nil {
			return layout, err
		}
		layout.Assignments = append(layout.Assignments, assignment)
	}
	return layout, assignments.Err()
}

func (s *Store) ReplaceGroupLayout(layout model.GroupLayoutState) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM skill_group_assignments`); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err = tx.Exec(`DELETE FROM skill_groups`); err != nil {
		_ = tx.Rollback()
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, group := range layout.Groups {
		manual := 0
		if group.Manual {
			manual = 1
		}
		if _, err = tx.Exec(`INSERT INTO skill_groups(root_id,id,name,position,manual,updated_at) VALUES(?,?,?,?,?,?)`,
			group.RootID, group.ID, group.Name, group.Position, manual, now); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	for _, assignment := range layout.Assignments {
		if _, err = tx.Exec(`INSERT INTO skill_group_assignments(root_id,skill_name,group_id,position,updated_at) VALUES(?,?,?,?,?)`,
			assignment.RootID, assignment.SkillName, assignment.GroupID, assignment.Position, now); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) LoadLock() (model.SourcesLock, error) {
	data, err := os.ReadFile(s.lockPath)
	if errors.Is(err, os.ErrNotExist) {
		return model.SourcesLock{SchemaVersion: model.SourcesLockSchemaVersion, Packages: map[string]model.PackageLock{}}, nil
	}
	if err != nil {
		return model.SourcesLock{}, err
	}
	var lock model.SourcesLock
	if err := json.Unmarshal(data, &lock); err != nil {
		return model.SourcesLock{}, err
	}
	if lock.Packages == nil {
		lock.Packages = map[string]model.PackageLock{}
	}
	if lock.SchemaVersion == 1 || lock.SchemaVersion == 0 {
		return s.migrateSourcesLockV1(lock)
	}
	if lock.SchemaVersion != model.SourcesLockSchemaVersion {
		return model.SourcesLock{}, fmt.Errorf("unsupported sources lock schema: %d", lock.SchemaVersion)
	}
	return normalizeSourcesLock(lock, s.roots)
}

func (s *Store) LoadSourcesLock() (model.SourcesLock, error) {
	return s.LoadLock()
}

func (s *Store) SaveLock(lock model.SourcesLock) error {
	lock, err := normalizeSourcesLock(lock, s.roots)
	if err != nil {
		return err
	}
	lock.SchemaVersion = model.SourcesLockSchemaVersion
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.lockPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.lockPath)
}

func (s *Store) SaveSourcesLock(lock model.SourcesLock) error {
	return s.SaveLock(lock)
}

// MigrateSourcesLockV1 migrates a legacy lock without touching the filesystem.
// The Store method supplies configured roots for path-based inference.
func MigrateSourcesLockV1(lock model.SourcesLock, roots []model.SkillRoot) (model.SourcesLock, error) {
	if lock.Packages == nil {
		lock.Packages = map[string]model.PackageLock{}
	}
	return normalizeSourcesLock(lock, roots)
}

func (s *Store) migrateSourcesLockV1(lock model.SourcesLock) (model.SourcesLock, error) {
	return MigrateSourcesLockV1(lock, s.roots)
}

func normalizeSourcesLock(lock model.SourcesLock, roots []model.SkillRoot) (model.SourcesLock, error) {
	packages := map[string]model.PackageLock{}
	for id, pkg := range lock.Packages {
		rootID := strings.TrimSpace(pkg.RootID)
		if rootID == "" {
			if index := strings.IndexByte(id, '\x00'); index > 0 {
				rootID = id[:index]
			}
		}
		if rootID == "" {
			inferredRootID, err := inferPackageRoot(pkg, roots)
			if err != nil {
				return model.SourcesLock{}, fmt.Errorf("migrate package %q: %w", id, err)
			}
			rootID = inferredRootID
		}
		if strings.TrimSpace(rootID) == "" {
			return model.SourcesLock{}, fmt.Errorf("migrate package %q: root identity is ambiguous", id)
		}
		pkg.RootID = rootID
		pkg.SourceAssociation = model.NormalizeSourceAssociation(pkg.Provider, pkg.SourceAssociation)
		// v1/v2 locks used resolvedCommit; newer records may also expose the
		// immutable ref explicitly.  Populate only from a full SHA so a branch
		// or tag can never be promoted into an immutable lock by migration.
		if canonical, err := model.CanonicalCommitSHA(pkg.ResolvedCommit); err == nil && canonical != "" {
			pkg.ResolvedCommit = canonical
		}
		if pkg.ResolvedRef == "" && model.IsImmutableCommitSHA(pkg.ResolvedCommit) {
			pkg.ResolvedRef = pkg.ResolvedCommit
		}
		if pkg.ResolvedCommit == "" && model.IsImmutableCommitSHA(pkg.ResolvedRef) {
			pkg.ResolvedCommit = strings.ToLower(strings.TrimSpace(pkg.ResolvedRef))
		}
		for name, skill := range pkg.Skills {
			if skill.RootID == "" {
				skill.RootID = rootID
			}
			skill.SourceAssociation = model.NormalizeSourceAssociation(pkg.Provider, skill.SourceAssociation)
			if canonical, err := model.CanonicalCommitSHA(skill.ResolvedCommit); err == nil && canonical != "" {
				skill.ResolvedCommit = canonical
			}
			if skill.ResolvedRef == "" && model.IsImmutableCommitSHA(skill.ResolvedCommit) {
				skill.ResolvedRef = skill.ResolvedCommit
			}
			if skill.ResolvedCommit == "" && model.IsImmutableCommitSHA(skill.ResolvedRef) {
				skill.ResolvedCommit = strings.ToLower(strings.TrimSpace(skill.ResolvedRef))
			}
			pkg.Skills[name] = skill
		}
		qualifiedID := model.QualifiedPackageID(rootID, stripQualifiedPackageID(id, rootID))
		if _, exists := packages[qualifiedID]; exists {
			return model.SourcesLock{}, fmt.Errorf("migrate package %q: duplicate root-qualified package", id)
		}
		packages[qualifiedID] = pkg
	}
	lock.SchemaVersion = model.SourcesLockSchemaVersion
	lock.Packages = packages
	return lock, nil
}

func stripQualifiedPackageID(id, rootID string) string {
	prefix := rootID + "\x00"
	if strings.HasPrefix(id, prefix) {
		return strings.TrimPrefix(id, prefix)
	}
	return id
}

func inferPackageRoot(pkg model.PackageLock, roots []model.SkillRoot) (string, error) {
	var candidates []string
	for _, skill := range pkg.Skills {
		if strings.TrimSpace(skill.LocalPath) == "" {
			continue
		}
		if !filepath.IsAbs(skill.LocalPath) {
			// v1 stored LocalPath relative to the single Skills root.
			candidates = append(candidates, model.RootIDCodexDefault)
			continue
		}
		matches := rootsContaining(skill.LocalPath, roots)
		if len(matches) == 0 {
			// With no configured paths, retain v1's historical default root. If
			// roots are supplied, an unknown path is unsafe to guess.
			if len(roots) == 0 {
				matches = []string{model.RootIDCodexDefault}
			} else {
				return "", fmt.Errorf("local path %q is outside configured roots", skill.LocalPath)
			}
		}
		if len(matches) > 1 {
			return "", fmt.Errorf("local path %q matches multiple roots", skill.LocalPath)
		}
		candidates = append(candidates, matches[0])
	}
	if len(candidates) == 0 {
		if len(roots) == 0 {
			return model.RootIDCodexDefault, nil
		}
		return "", errors.New("package has no local path to infer root")
	}
	rootID := candidates[0]
	for _, candidate := range candidates[1:] {
		if candidate != rootID {
			return "", errors.New("skills in one package span multiple roots")
		}
	}
	return rootID, nil
}

func rootsContaining(path string, roots []model.SkillRoot) []string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil
	}
	absolute = filepath.Clean(absolute)
	var matches []string
	for _, root := range roots {
		base, err := filepath.Abs(root.Path)
		if err != nil {
			continue
		}
		base = filepath.Clean(base)
		relative, err := filepath.Rel(base, absolute)
		if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative) {
			matches = append(matches, root.ID)
		}
	}
	return matches
}

func (s *Store) SaveTransaction(tx model.Transaction) error {
	data, _ := json.Marshal(tx)
	_, err := s.db.Exec(`
INSERT INTO transactions(id,type,status,started_at,completed_at,payload_json)
VALUES(?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET status=excluded.status,completed_at=excluded.completed_at,payload_json=excluded.payload_json`,
		tx.ID, tx.Type, tx.Status, tx.StartedAt.Format(time.RFC3339Nano), nullTime(tx.CompletedAt), string(data))
	return err
}

func (s *Store) RecentTransactions(limit int) ([]model.Transaction, error) {
	rows, err := s.db.Query(`SELECT payload_json FROM transactions ORDER BY started_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]model.Transaction, 0)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var tx model.Transaction
		if json.Unmarshal([]byte(raw), &tx) == nil {
			out = append(out, tx)
		}
	}
	return out, rows.Err()
}

// RecoverableAssistedTransactions returns every assisted-install transaction
// that still has an automatic recovery path. These records must not disappear
// merely because newer history entries pushed them past the recent-history
// limit used by the dashboard.
func (s *Store) RecoverableAssistedTransactions() ([]model.Transaction, error) {
	rows, err := s.db.Query(`
SELECT payload_json
FROM transactions
WHERE type = 'assisted-install'
ORDER BY started_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]model.Transaction, 0)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var tx model.Transaction
		if err := json.Unmarshal([]byte(raw), &tx); err != nil {
			continue
		}
		recovery := strings.ToLower(strings.TrimSpace(tx.RecoveryStatus))
		status := strings.ToLower(strings.TrimSpace(tx.Status))
		// A legacy assisted-install record may still be "running" with no
		// recovery status after an application exit. Return it so the manager
		// can reconcile the orphaned plan and transaction for rollback.
		if status == "running" || (recovery != "" && recovery != "completed") {
			out = append(out, tx)
		}
	}
	return out, rows.Err()
}

func (s *Store) Transaction(id string) (model.Transaction, error) {
	var raw string
	if err := s.db.QueryRow(`SELECT payload_json FROM transactions WHERE id=?`, id).Scan(&raw); err != nil {
		return model.Transaction{}, err
	}
	var tx model.Transaction
	if err := json.Unmarshal([]byte(raw), &tx); err != nil {
		return model.Transaction{}, err
	}
	return tx, nil
}

func (s *Store) SaveScan(report model.ScanReport) error {
	data, _ := json.Marshal(report)
	_, err := s.db.Exec(`INSERT OR REPLACE INTO scan_reports(id,target,severity,created_at,payload_json) VALUES(?,?,?,?,?)`,
		report.ID, report.Target, string(report.HighestSeverity), report.CompletedAt.Format(time.RFC3339Nano), string(data))
	return err
}

func (s *Store) RecentScans(limit int) ([]model.ScanReport, error) {
	rows, err := s.db.Query(`SELECT payload_json FROM scan_reports ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]model.ScanReport, 0)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var r model.ScanReport
		if json.Unmarshal([]byte(raw), &r) == nil {
			out = append(out, r)
		}
	}
	return out, rows.Err()
}

func (s *Store) Scan(id string) (model.ScanReport, error) {
	var raw string
	if err := s.db.QueryRow(`SELECT payload_json FROM scan_reports WHERE id=?`, id).Scan(&raw); err != nil {
		return model.ScanReport{}, err
	}
	var report model.ScanReport
	if err := json.Unmarshal([]byte(raw), &report); err != nil {
		return model.ScanReport{}, err
	}
	return report, nil
}

func (s *Store) LatestScansByTarget() ([]model.ScanReport, error) {
	rows, err := s.db.Query(`SELECT payload_json FROM scan_reports ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := map[string]bool{}
	out := make([]model.ScanReport, 0)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var report model.ScanReport
		if json.Unmarshal([]byte(raw), &report) != nil || seen[report.Target] {
			continue
		}
		seen[report.Target] = true
		out = append(out, report)
	}
	return out, rows.Err()
}

func (s *Store) IgnoredFindings() (map[string]string, error) {
	rows, err := s.db.Query(`SELECT fingerprint, COALESCE(reason, '') FROM finding_ignores`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var fingerprint, reason string
		if err := rows.Scan(&fingerprint, &reason); err != nil {
			return nil, err
		}
		out[fingerprint] = reason
	}
	return out, rows.Err()
}

func (s *Store) SetFindingIgnored(finding model.Finding, ignored bool, reason string) error {
	if finding.Fingerprint == "" {
		return errors.New("finding fingerprint is required")
	}
	if !ignored {
		_, err := s.db.Exec(`DELETE FROM finding_ignores WHERE fingerprint=?`, finding.Fingerprint)
		return err
	}
	_, err := s.db.Exec(`
INSERT INTO finding_ignores(fingerprint,rule_id,file,reason,created_at)
VALUES(?,?,?,?,?)
ON CONFLICT(fingerprint) DO UPDATE SET reason=excluded.reason`,
		finding.Fingerprint, finding.RuleID, finding.File, reason, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) SetClusterIgnored(cluster model.RiskCluster, ignored bool, reason string) error {
	return s.SetClustersIgnored([]model.RiskCluster{cluster}, ignored, reason)
}

func (s *Store) SetClustersIgnored(clusters []model.RiskCluster, ignored bool, reason string) error {
	if len(clusters) == 0 {
		return errors.New("at least one risk cluster is required")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	for _, cluster := range clusters {
		if cluster.ID == "" || len(cluster.Fingerprints) == 0 {
			_ = tx.Rollback()
			return errors.New("risk cluster and findings are required")
		}
		for _, fingerprint := range cluster.Fingerprints {
			if fingerprint == "" {
				_ = tx.Rollback()
				return errors.New("finding fingerprint is required")
			}
			if !ignored {
				if _, err = tx.Exec(`DELETE FROM finding_ignores WHERE fingerprint=?`, fingerprint); err != nil {
					_ = tx.Rollback()
					return err
				}
				continue
			}
			if _, err = tx.Exec(`
INSERT INTO finding_ignores(fingerprint,rule_id,file,reason,created_at)
VALUES(?,?,?,?,?)
ON CONFLICT(fingerprint) DO UPDATE SET reason=excluded.reason`,
				fingerprint, cluster.RuleID, cluster.FileClass, reason, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
				_ = tx.Rollback()
				return err
			}
		}
	}
	return tx.Commit()
}

func (s *Store) Approve(planID, decision, reason string) error {
	if planID == "" || decision == "" {
		return fmt.Errorf("plan and decision are required")
	}
	_, err := s.db.Exec(`INSERT INTO approvals(plan_id,decision,reason,created_at) VALUES(?,?,?,?)`,
		planID, decision, reason, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.Format(time.RFC3339Nano)
}
