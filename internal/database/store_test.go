package database

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, path
}

func TestFreshMigrationAndForeignKeys(t *testing.T) {
	s, _ := newTestStore(t)
	if err := s.Integrity(); err != nil {
		t.Fatal(err)
	}
	pid, err := s.AddProvider("fake", "http://127.0.0.1:10001/v1", "openai-compatible")
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.AddCredential(pid, "primary", "OT_TEST_KEY")
	if err != nil {
		t.Fatal(err)
	}
	creds, err := s.ListCredentials()
	if err != nil || len(creds) != 1 || creds[0].ID != id {
		t.Fatalf("creds=%v err=%v", creds, err)
	}
	if err := s.DeleteCredential(id); err != nil {
		t.Fatal(err)
	}
}

func TestWALAndNoSecretValues(t *testing.T) {
	s, path := newTestStore(t)
	var journal string
	if err := s.db.QueryRow(`PRAGMA journal_mode`).Scan(&journal); err != nil {
		t.Fatal(err)
	}
	if journal != "wal" {
		t.Fatalf("journal_mode=%s", journal)
	}
	pid, _ := s.AddProvider("fake", "http://127.0.0.1:10001/v1", "openai-compatible")
	_, _ = s.AddCredential(pid, "primary", "OT_CANARY_REF")
	data, err := readFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "CANARY_SECRET_VALUE") {
		t.Fatal("secret canary found in database")
	}
}

func readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}
