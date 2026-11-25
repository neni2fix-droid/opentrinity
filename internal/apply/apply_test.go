package apply

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/nenifix/opentrinity/internal/config"
	"github.com/nenifix/opentrinity/internal/database"
)

func setup(t *testing.T) (*Manager, string) {
	t.Helper()
	root := t.TempDir()
	paths := config.PathsFromRoot(root)
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	store, err := database.Open(paths.DB)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	pid, _ := store.AddProvider("fake", "http://127.0.0.1:10001/v1", "openai-compatible")
	_, _ = store.AddCredential(pid, "primary", "OT_CANARY_REF")
	mid, _ := store.AddModel(pid, "fake-model", "fake")
	if err := store.SetDeployment("A", mid, true); err != nil {
		t.Fatal(err)
	}
	return &Manager{Paths: paths, Store: store, Activate: func(string) error { return nil }}, root
}

func TestValidateDoesNotReplaceActiveConfig(t *testing.T) {
	m, _ := setup(t)
	if err := os.WriteFile(m.Paths.Generated, []byte("active-config"), 0o600); err != nil {
		t.Fatal(err)
	}
	g, err := m.Validate()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(m.Paths.Generated)
	if string(b) != "active-config" {
		t.Fatal("validate replaced active config")
	}
	if g.ID == 0 || len(g.YAML) == 0 {
		t.Fatal("generation not persisted")
	}
}

func TestFailedActivationPreservesActiveFile(t *testing.T) {
	m, _ := setup(t)
	active := []byte("last-known-good")
	if err := os.WriteFile(m.Paths.Generated, active, 0o600); err != nil {
		t.Fatal(err)
	}
	m.Activate = func(path string) error {
		b, _ := os.ReadFile(path)
		if string(b) == string(active) {
			return nil
		}
		return errors.New("health check failed")
	}
	if _, err := m.Run(0); err == nil {
		t.Fatal("expected apply failure")
	}
	b, _ := os.ReadFile(m.Paths.Generated)
	if string(b) != string(active) {
		t.Fatalf("active config changed after failed apply: %q", string(b))
	}
}

func TestSuccessfulApplySetsLastGoodAndRollback(t *testing.T) {
	m, _ := setup(t)
	g, err := m.Run(0)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(m.Paths.Generated)
	if string(b) != g.YAML {
		t.Fatal("generated config not activated")
	}
	lastGood, _ := os.ReadFile(m.Paths.LastGood)
	if string(lastGood) != g.YAML {
		t.Fatal("last good copy missing")
	}
	value, err := m.Store.Setting("last_good_generation_id")
	if err != nil || value == "" {
		t.Fatalf("last_good_generation_id=%q err=%v", value, err)
	}
	if _, err := m.Rollback(); err != nil {
		t.Fatal(err)
	}
	_ = filepath.Base(m.Paths.Generated)
}
