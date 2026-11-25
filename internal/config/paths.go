// Package config defines local paths and small configuration records for the MVP.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

const (
	GatewayPort = 3888
	LiteLLMPort = 3889
	ControlPort = 8369
	Alias       = "nenifix-coder"
)

// Paths holds all per-user file locations. Nothing is written outside this tree.
type Paths struct {
	Root       string
	DB         string
	State      string
	Generated  string
	LastGood   string
	RunDir     string
	LockFile   string
	StopFlag   string
	LogDir     string
	Supervisor string
	Gateway    string
	Control    string
	LiteLLM    string
	LitellmExe string
}

// DefaultPaths returns the Windows per-user data directory. OT_USER_DATA can
// override it for tests; the override is still a local directory supplied by
// the caller, never a network path.
func DefaultPaths() (Paths, error) {
	root := os.Getenv("OT_USER_DATA")
	if root == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return Paths{}, err
		}
		root = filepath.Join(base, "Nenifix", "OpenTrinity")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return Paths{}, err
	}
	return PathsFromRoot(root), nil
}

// PathsFromRoot builds the path set for an explicit root.
func PathsFromRoot(root string) Paths {
	p := Paths{Root: root}
	p.DB = filepath.Join(root, "opentrinity.db")
	p.State = filepath.Join(root, "state.json")
	p.Generated = filepath.Join(root, "generated", "litellm.yaml")
	p.LastGood = filepath.Join(root, "generated", "litellm.last-good.yaml")
	p.RunDir = filepath.Join(root, "run")
	p.LockFile = filepath.Join(p.RunDir, "supervisor.lock")
	p.StopFlag = filepath.Join(p.RunDir, "stop.flag")
	p.LogDir = filepath.Join(root, "logs")
	p.Supervisor = filepath.Join(p.LogDir, "supervisor.log")
	p.Gateway = filepath.Join(p.LogDir, "gateway.log")
	p.Control = filepath.Join(p.LogDir, "control.log")
	p.LiteLLM = filepath.Join(p.LogDir, "litellm.log")
	p.LitellmExe = os.Getenv("OT_LITELLM_EXE")
	return p
}

// Ensure creates the local directory tree with per-user permissions.
func (p Paths) Ensure() error {
	for _, dir := range []string{p.Root, filepath.Dir(p.Generated), p.RunDir, p.LogDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	return nil
}

// ProcessState is intentionally free of secrets and prompts.
type ProcessState struct {
	Version      string    `json:"version"`
	SupervisorPID int      `json:"supervisor_pid"`
	GatewayPID   int       `json:"gateway_pid"`
	ControlPID   int       `json:"control_pid"`
	LiteLLMPID   int       `json:"litellm_pid"`
	GatewayCreation uint64 `json:"gateway_creation"`
	ControlCreation uint64 `json:"control_creation"`
	LiteLLMCreation uint64 `json:"litellm_creation"`
	GenerationID int64     `json:"generation_id"`
	LiteLLMConfig string   `json:"litellm_config"`
	StartedAt    time.Time `json:"started_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func LoadState(path string) (ProcessState, error) {
	var s ProcessState
	b, err := os.ReadFile(path)
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return s, err
	}
	return s, nil
}

func SaveState(path string, s ProcessState) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	return nil
}

// ErrNotInitialized is returned when the user data tree is missing.
var ErrNotInitialized = errors.New("opentrinity is not initialized; run `opentrinity init` first")
