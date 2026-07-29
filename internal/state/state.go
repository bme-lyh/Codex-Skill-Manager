package state

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"github.com/bme-lyh/Codex-Skill-Manager/internal/model"
)

type Store struct {
	db       *sql.DB
	dataRoot string
	lockPath string
}

func Open(dataRoot string) (*Store, error) {
	if err := os.MkdirAll(dataRoot, 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.Join(dataRoot, "state.db"))
	if err != nil {
		return nil, err
	}
	s := &Store{db: db, dataRoot: dataRoot, lockPath: filepath.Join(dataRoot, "sources.lock.json")}
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
  id TEXT PRIMARY KEY, name TEXT NOT NULL, position INTEGER NOT NULL,
  manual INTEGER NOT NULL, updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS skill_group_assignments (
  skill_name TEXT PRIMARY KEY, group_id TEXT NOT NULL,
  position INTEGER NOT NULL, updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS update_statuses (
  group_id TEXT PRIMARY KEY, checked_at TEXT NOT NULL, payload_json TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS skill_security_states (
  skill_name TEXT PRIMARY KEY, content_hash TEXT NOT NULL,
  report_id TEXT NOT NULL, checked_at TEXT NOT NULL
);
`)
	return err
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
INSERT INTO skill_security_states(skill_name,content_hash,report_id,checked_at) VALUES(?,?,?,?)
ON CONFLICT(skill_name) DO UPDATE SET
content_hash=excluded.content_hash,report_id=excluded.report_id,checked_at=excluded.checked_at`,
			state.SkillName, state.ContentHash, state.ReportID, state.CheckedAt.Format(time.RFC3339Nano)); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) SkillSecurityStates() (map[string]model.SkillSecurityState, error) {
	rows, err := s.db.Query(`SELECT skill_name,content_hash,report_id,checked_at FROM skill_security_states`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]model.SkillSecurityState{}
	for rows.Next() {
		var state model.SkillSecurityState
		var checkedAt string
		if err := rows.Scan(&state.SkillName, &state.ContentHash, &state.ReportID, &checkedAt); err != nil {
			return nil, err
		}
		state.CheckedAt, err = time.Parse(time.RFC3339Nano, checkedAt)
		if err != nil {
			return nil, fmt.Errorf("parse skill security check time: %w", err)
		}
		out[state.SkillName] = state
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
INSERT INTO update_statuses(group_id,checked_at,payload_json) VALUES(?,?,?)
ON CONFLICT(group_id) DO UPDATE SET checked_at=excluded.checked_at,payload_json=excluded.payload_json`,
			status.GroupID, status.CheckedAt.Format(time.RFC3339Nano), string(data)); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) LatestUpdateStatuses() ([]model.UpdateStatus, error) {
	rows, err := s.db.Query(`SELECT payload_json FROM update_statuses ORDER BY checked_at DESC,group_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	statuses := []model.UpdateStatus{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var status model.UpdateStatus
		if json.Unmarshal([]byte(raw), &status) == nil {
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
	rows, err := s.db.Query(`SELECT id,name,position,manual FROM skill_groups ORDER BY position,name`)
	if err != nil {
		return layout, err
	}
	for rows.Next() {
		var group model.GroupPreference
		var manual int
		if err := rows.Scan(&group.ID, &group.Name, &group.Position, &manual); err != nil {
			rows.Close()
			return layout, err
		}
		group.Manual = manual != 0
		layout.Groups = append(layout.Groups, group)
	}
	if err := rows.Close(); err != nil {
		return layout, err
	}
	assignments, err := s.db.Query(`SELECT skill_name,group_id,position FROM skill_group_assignments ORDER BY group_id,position,skill_name`)
	if err != nil {
		return layout, err
	}
	defer assignments.Close()
	for assignments.Next() {
		var assignment model.SkillGroupAssignment
		if err := assignments.Scan(&assignment.SkillName, &assignment.GroupID, &assignment.Position); err != nil {
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
		if _, err = tx.Exec(`INSERT INTO skill_groups(id,name,position,manual,updated_at) VALUES(?,?,?,?,?)`,
			group.ID, group.Name, group.Position, manual, now); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	for _, assignment := range layout.Assignments {
		if _, err = tx.Exec(`INSERT INTO skill_group_assignments(skill_name,group_id,position,updated_at) VALUES(?,?,?,?)`,
			assignment.SkillName, assignment.GroupID, assignment.Position, now); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) LoadLock() (model.SourcesLock, error) {
	data, err := os.ReadFile(s.lockPath)
	if errors.Is(err, os.ErrNotExist) {
		return model.SourcesLock{SchemaVersion: 1, Packages: map[string]model.PackageLock{}}, nil
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
	return lock, nil
}

func (s *Store) SaveLock(lock model.SourcesLock) error {
	lock.SchemaVersion = 1
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
