// Package apply implements candidate validation, atomic activation and
// last-known-good rollback for generated LiteLLM configuration.
package apply

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/nenifix/opentrinity/internal/config"
	"github.com/nenifix/opentrinity/internal/database"
	"github.com/nenifix/opentrinity/internal/routingconfig"
)

// Manager coordinates the database, generated files and a runtime activator.
// Activate must restart the private LiteLLM process with the supplied config
// path and return an error if health or smoke checks fail.
type Manager struct {
	Paths    config.Paths
	Store    *database.Store
	Activate func(configPath string) error
}

func (m *Manager) snapshot() (routingconfig.Snapshot, error) {
	providers, err := m.Store.ListProviders()
	if err != nil {
		return routingconfig.Snapshot{}, err
	}
	credentials, err := m.Store.ListCredentials()
	if err != nil {
		return routingconfig.Snapshot{}, err
	}
	models, err := m.Store.ListModels()
	if err != nil {
		return routingconfig.Snapshot{}, err
	}
	deployments, err := m.Store.ListDeployments()
	if err != nil {
		return routingconfig.Snapshot{}, err
	}
	return routingconfig.Snapshot{Providers: providers, Credentials: credentials, Models: models, Deployments: deployments}, nil
}

// Validate generates a candidate and stores it as an immutable generation.
// It never replaces the active configuration.
func (m *Manager) Validate() (database.Generation, error) {
	snap, err := m.snapshot()
	if err != nil {
		return database.Generation{}, err
	}
	yamlText, err := routingconfig.Generate(snap)
	if err != nil {
		return database.Generation{}, err
	}
	return m.Store.CreateGeneration(yamlText)
}

// Run activates a candidate generation. If generationID is zero, the current
// state is validated and a new generation is created first.
func (m *Manager) Run(generationID int64) (database.Generation, error) {
	var g database.Generation
	var err error
	if generationID == 0 {
		g, err = m.Validate()
	} else {
		g, err = m.Store.GetGeneration(generationID)
	}
	if err != nil {
		return g, err
	}
	if g.Status == "active" || g.Status == "last-good" {
		return g, fmt.Errorf("generation %d is already applied", g.ID)
	}

	oldYAML, oldErr := os.ReadFile(m.Paths.Generated)
	if oldErr != nil && !errors.Is(oldErr, os.ErrNotExist) {
		return g, oldErr
	}

	candidatePath, err := writePrivateTemp(filepath.Dir(m.Paths.Generated), "candidate-*.yaml", []byte(g.YAML))
	if err != nil {
		return g, err
	}
	defer os.Remove(candidatePath)

	if err := m.activate(candidatePath); err != nil {
		if oldErr == nil {
			_ = os.WriteFile(m.Paths.Generated, oldYAML, 0o600)
			_ = m.activate(m.Paths.Generated)
		}
		_ = m.Store.RecordApply(g.ID, "failed", "activation or health check failed")
		return g, fmt.Errorf("apply failed; active configuration preserved: %w", err)
	}

	if err := atomicReplace(m.Paths.Generated, []byte(g.YAML)); err != nil {
		return g, err
	}
	if err := atomicReplace(m.Paths.LastGood, []byte(g.YAML)); err != nil {
		return g, err
	}
	if err := m.Store.SetGenerationStatus(g.ID, "active"); err != nil {
		return g, err
	}
	if err := m.Store.SetSetting("active_generation_id", strconv.FormatInt(g.ID, 10)); err != nil {
		return g, err
	}
	if err := m.Store.SetSetting("last_good_generation_id", strconv.FormatInt(g.ID, 10)); err != nil {
		return g, err
	}
	if err := m.Store.RecordApply(g.ID, "success", "activated after health check"); err != nil {
		return g, err
	}
	if err := m.activate(m.Paths.Generated); err != nil {
		return g, err
	}
	return g, nil
}

// Rollback reactivates the last known good generation.
func (m *Manager) Rollback() (database.Generation, error) {
	idRaw, err := m.Store.Setting("last_good_generation_id")
	if err != nil {
		return database.Generation{}, err
	}
	id, err := strconv.ParseInt(idRaw, 10, 64)
	if err != nil || id == 0 {
		return database.Generation{}, errors.New("no last known good generation exists")
	}
	g, err := m.Store.GetGeneration(id)
	if err != nil {
		return g, err
	}
	tmp, err := writePrivateTemp(filepath.Dir(m.Paths.Generated), "rollback-*.yaml", []byte(g.YAML))
	if err != nil {
		return g, err
	}
	defer os.Remove(tmp)
	if err := m.activate(tmp); err != nil {
		_ = m.Store.RecordApply(g.ID, "failed", "rollback health check failed")
		return g, err
	}
	if err := atomicReplace(m.Paths.Generated, []byte(g.YAML)); err != nil {
		return g, err
	}
	if err := m.Store.SetSetting("active_generation_id", strconv.FormatInt(g.ID, 10)); err != nil {
		return g, err
	}
	if err := m.Store.SetGenerationStatus(g.ID, "active"); err != nil {
		return g, err
	}
	if err := m.Store.RecordApply(g.ID, "rollback", "restored last known good"); err != nil {
		return g, err
	}
	return g, m.activate(m.Paths.Generated)
}

func (m *Manager) activate(path string) error {
	if m.Activate == nil {
		return nil
	}
	return m.Activate(path)
}

func writePrivateTemp(dir, pattern string, data []byte) (string, error) {
	f, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", err
	}
	name := f.Name()
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		_ = os.Remove(name)
		return "", err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(name)
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	return name, nil
}

func atomicReplace(dest string, data []byte) error {
	tmp, err := writePrivateTemp(filepath.Dir(dest), "replace-*.yaml", data)
	if err != nil {
		return err
	}
	if err := os.Remove(dest); err != nil && !errors.Is(err, os.ErrNotExist) {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
