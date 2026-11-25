// Package database implements the minimal SQLite configuration store.
// It never stores secret values: only Credential Manager target references.
package database

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Store serializes all writes through one connection and one mutex.
type Store struct {
	db *sql.DB
	mu sync.Mutex
}

type Provider struct {
	ID       int64
	Name     string
	APIBase  string
	Kind     string
	Enabled  bool
}

type Credential struct {
	ID         int64
	ProviderID int64
	Label      string
	SecretRef  string
	Enabled    bool
}

type Model struct {
	ID         int64
	ProviderID int64
	ModelID    string
	Label      string
	Enabled    bool
}

type Deployment struct {
	Slot       string
	ModelID    int64
	ModelName  string
	ProviderID int64
	Enabled    bool
}

type Generation struct {
	ID        int64
	SHA256    string
	YAML      string
	CreatedAt string
	Status    string
}

type ApplyRun struct {
	ID           int64
	GenerationID int64
	Outcome      string
	CreatedAt    string
	Detail       string
}

// Open initializes the database and runs migrations.
func Open(path string) (*Store, error) {
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS schema_migrations (
  version INTEGER PRIMARY KEY,
  applied_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS providers (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL UNIQUE,
  api_base TEXT NOT NULL,
  kind TEXT NOT NULL DEFAULT 'openai-compatible',
  enabled INTEGER NOT NULL DEFAULT 1
);
CREATE TABLE IF NOT EXISTS credentials (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  provider_id INTEGER NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
  label TEXT NOT NULL,
  secret_ref TEXT NOT NULL UNIQUE,
  enabled INTEGER NOT NULL DEFAULT 1
);
CREATE TABLE IF NOT EXISTS models (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  provider_id INTEGER NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
  model_id TEXT NOT NULL,
  label TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 1,
  UNIQUE(provider_id, model_id)
);
CREATE TABLE IF NOT EXISTS deployments (
  slot TEXT PRIMARY KEY CHECK(slot IN ('A','B','C')),
  model_id INTEGER REFERENCES models(id) ON DELETE SET NULL,
  enabled INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS config_generations (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  content_sha256 TEXT NOT NULL,
  config_yaml TEXT NOT NULL,
  created_at TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'candidate'
);
CREATE TABLE IF NOT EXISTS apply_runs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  generation_id INTEGER REFERENCES config_generations(id) ON DELETE SET NULL,
  outcome TEXT NOT NULL,
  created_at TEXT NOT NULL,
  detail TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_credentials_provider ON credentials(provider_id);
CREATE INDEX IF NOT EXISTS idx_models_provider ON models(provider_id);
`
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}
	_, err := s.db.Exec(`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(1, ?)`, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return err
	}
	for _, slot := range []string{"A", "B", "C"} {
		if _, err := s.db.Exec(`INSERT OR IGNORE INTO deployments(slot, enabled) VALUES(?, 0)`, slot); err != nil {
			return err
		}
	}
	return nil
}

// Integrity runs SQLite's built-in consistency check.
func (s *Store) Integrity() error {
	var result string
	if err := s.db.QueryRow(`PRAGMA integrity_check`).Scan(&result); err != nil {
		return err
	}
	if result != "ok" {
		return fmt.Errorf("integrity check failed: %s", result)
	}
	return nil
}

func (s *Store) Setting(key string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

func (s *Store) SetSetting(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

func (s *Store) AddProvider(name, apiBase, kind string) (int64, error) {
	if name == "" || apiBase == "" {
		return 0, errors.New("provider name and API base are required")
	}
	if kind == "" {
		kind = "openai-compatible"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(`INSERT INTO providers(name, api_base, kind, enabled) VALUES(?,?,?,1)`, name, apiBase, kind)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) ListProviders() ([]Provider, error) {
	rows, err := s.db.Query(`SELECT id,name,api_base,kind,enabled FROM providers ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Provider
	for rows.Next() {
		var p Provider
		var enabled int
		if err := rows.Scan(&p.ID, &p.Name, &p.APIBase, &p.Kind, &enabled); err != nil {
			return nil, err
		}
		p.Enabled = enabled != 0
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) AddCredential(providerID int64, label, secretRef string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(`INSERT INTO credentials(provider_id,label,secret_ref,enabled) VALUES(?,?,?,1)`, providerID, label, secretRef)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) ListCredentials() ([]Credential, error) {
	rows, err := s.db.Query(`SELECT id,provider_id,label,secret_ref,enabled FROM credentials ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Credential
	for rows.Next() {
		var c Credential
		var enabled int
		if err := rows.Scan(&c.ID, &c.ProviderID, &c.Label, &c.SecretRef, &enabled); err != nil {
			return nil, err
		}
		c.Enabled = enabled != 0
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) GetCredential(id int64) (Credential, error) {
	var c Credential
	var enabled int
	err := s.db.QueryRow(`SELECT id,provider_id,label,secret_ref,enabled FROM credentials WHERE id=?`, id).
		Scan(&c.ID, &c.ProviderID, &c.Label, &c.SecretRef, &enabled)
	if err != nil {
		return c, err
	}
	c.Enabled = enabled != 0
	return c, nil
}

func (s *Store) DeleteCredential(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(`DELETE FROM credentials WHERE id=?`, id)
	if err != nil {
		return err
	}
	return requireAffected(res, "credential")
}

func (s *Store) SetCredentialEnabled(id int64, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := 0
	if enabled {
		v = 1
	}
	res, err := s.db.Exec(`UPDATE credentials SET enabled=? WHERE id=?`, v, id)
	if err != nil {
		return err
	}
	return requireAffected(res, "credential")
}

func (s *Store) AddModel(providerID int64, modelID, label string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(`INSERT INTO models(provider_id,model_id,label,enabled) VALUES(?,?,?,1)`, providerID, modelID, label)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) ListModels() ([]Model, error) {
	rows, err := s.db.Query(`SELECT id,provider_id,model_id,label,enabled FROM models ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Model
	for rows.Next() {
		var m Model
		var enabled int
		if err := rows.Scan(&m.ID, &m.ProviderID, &m.ModelID, &m.Label, &enabled); err != nil {
			return nil, err
		}
		m.Enabled = enabled != 0
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) SetDeployment(slot string, modelID int64, enabled bool) error {
	valid := map[string]bool{"A": true, "B": true, "C": true}
	if !valid[slot] {
		return errors.New("slot must be A, B or C")
	}
	v := 0
	if enabled {
		v = 1
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(`UPDATE deployments SET model_id=?, enabled=? WHERE slot=?`, modelID, v, slot)
	if err != nil {
		return err
	}
	return requireAffected(res, "deployment slot")
}

func (s *Store) ListDeployments() ([]Deployment, error) {
	rows, err := s.db.Query(`
SELECT d.slot, COALESCE(d.model_id,0), COALESCE(m.model_id,''), COALESCE(m.provider_id,0), d.enabled
FROM deployments d LEFT JOIN models m ON m.id=d.model_id ORDER BY d.slot`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Deployment
	for rows.Next() {
		var d Deployment
		var enabled int
		if err := rows.Scan(&d.Slot, &d.ModelID, &d.ModelName, &d.ProviderID, &enabled); err != nil {
			return nil, err
		}
		d.Enabled = enabled != 0
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) CreateGeneration(yaml string) (Generation, error) {
	sum := sha256.Sum256([]byte(yaml))
	g := Generation{SHA256: hex.EncodeToString(sum[:]), YAML: yaml, CreatedAt: time.Now().UTC().Format(time.RFC3339), Status: "candidate"}
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(`INSERT INTO config_generations(content_sha256,config_yaml,created_at,status) VALUES(?,?,?,?)`, g.SHA256, g.YAML, g.CreatedAt, g.Status)
	if err != nil {
		return g, err
	}
	g.ID, err = res.LastInsertId()
	return g, err
}

func (s *Store) GetGeneration(id int64) (Generation, error) {
	var g Generation
	err := s.db.QueryRow(`SELECT id,content_sha256,config_yaml,created_at,status FROM config_generations WHERE id=?`, id).
		Scan(&g.ID, &g.SHA256, &g.YAML, &g.CreatedAt, &g.Status)
	return g, err
}

func (s *Store) ListGenerations(limit int) ([]Generation, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.db.Query(`SELECT id,content_sha256,config_yaml,created_at,status FROM config_generations ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Generation
	for rows.Next() {
		var g Generation
		if err := rows.Scan(&g.ID, &g.SHA256, &g.YAML, &g.CreatedAt, &g.Status); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) SetGenerationStatus(id int64, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(`UPDATE config_generations SET status=? WHERE id=?`, status, id)
	if err != nil {
		return err
	}
	return requireAffected(res, "generation")
}

func (s *Store) RecordApply(generationID int64, outcome, detail string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`INSERT INTO apply_runs(generation_id,outcome,created_at,detail) VALUES(?,?,?,?)`,
		nullableInt(generationID), outcome, time.Now().UTC().Format(time.RFC3339), detail)
	return err
}

func (s *Store) ListApplyRuns(limit int) ([]ApplyRun, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.db.Query(`SELECT id,COALESCE(generation_id,0),outcome,created_at,detail FROM apply_runs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ApplyRun
	for rows.Next() {
		var r ApplyRun
		if err := rows.Scan(&r.ID, &r.GenerationID, &r.Outcome, &r.CreatedAt, &r.Detail); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func nullableInt(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

func requireAffected(res sql.Result, label string) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%s not found", label)
	}
	return nil
}

// Tx runs fn inside a bounded write transaction. It is used by apply.
func (s *Store) Tx(ctx context.Context, fn func(*sql.Tx) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
