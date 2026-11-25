// Package supervisor owns process lifecycle, singleton protection and
// Windows Job Object containment. It never routes inference traffic.
package supervisor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/nenifix/opentrinity/internal/config"
	"github.com/nenifix/opentrinity/internal/database"
	win "github.com/nenifix/opentrinity/internal/platform/windows"
	"github.com/nenifix/opentrinity/internal/secrets"
)

type child struct {
	name    string
	cmd     *exec.Cmd
	created uint64
	starts  int
	lastTry time.Time
	done    chan struct{}
}

// Supervisor runs the three-process topology on loopback only.
type Supervisor struct {
	paths  config.Paths
	store  *database.Store
	logger *log.Logger
	job    *win.Job

	mu       sync.Mutex
	children map[string]*child
	exited   chan *child
	stopping bool
	configPath string
}

// New creates a supervisor.
func New(paths config.Paths, store *database.Store, logger *log.Logger) *Supervisor {
	if paths.LitellmExe == "" {
		paths.LitellmExe = os.Getenv("OT_LITELLM_EXE")
	}
	return &Supervisor{
		paths:    paths,
		store:    store,
		logger:   logger,
		children: map[string]*child{},
		exited:   make(chan *child, 8),
	}
}

// Running reports whether a live supervisor lock exists.
func Running(paths config.Paths) bool {
	b, err := os.ReadFile(paths.LockFile)
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 0 {
		return false
	}
	_, err = win.CreationTime(uint32(pid))
	return err == nil
}

// Run starts all children and blocks until stop is requested.
func (s *Supervisor) Run(configPath string) error {
	if Running(s.paths) {
		return errors.New("supervisor is already running")
	}
	if err := s.paths.Ensure(); err != nil {
		return err
	}
	if err := os.WriteFile(s.paths.LockFile, []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		return err
	}
	defer os.Remove(s.paths.LockFile)

	job, err := win.NewKillOnCloseJob()
	if err != nil {
		return fmt.Errorf("create job object: %w", err)
	}
	s.job = job
	defer job.Close()

	if configPath == "" {
		configPath = s.paths.Generated
	}
	if _, err := os.Stat(configPath); err != nil {
		return fmt.Errorf("active LiteLLM config is missing: %w", err)
	}
	s.configPath = configPath

	state := config.ProcessState{
		Version: "mvp-0.1.0", SupervisorPID: os.Getpid(),
		GenerationID: activeGenerationID(s.store),
		LiteLLMConfig: configPath,
		StartedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := config.SaveState(s.paths.State, state); err != nil {
		return err
	}

	if err := s.startLiteLLM(configPath); err != nil {
		return err
	}
	if err := s.startChild("gateway", s.selfArgs("serve-gateway"), s.commonEnv(), s.paths.Gateway); err != nil {
		s.stopAll()
		return err
	}
	if err := s.startChild("control", s.selfArgs("serve-control"), s.commonEnv(), s.paths.Control); err != nil {
		s.stopAll()
		return err
	}

	ctx, stopSignals := signal.NotifyContext(contextBackground(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.shutdown()
			return nil
		case <-ticker.C:
			if _, err := os.Stat(s.paths.StopFlag); err == nil {
				_ = os.Remove(s.paths.StopFlag)
				s.shutdown()
				return nil
			}
			s.handleActivationRequest()
		case exited := <-s.exited:
			s.handleExit(exited)
		}
	}
}

func contextBackground() context.Context { return context.Background() }

func (s *Supervisor) commonEnv() map[string]string {
	return map[string]string{"OT_USER_DATA": s.paths.Root}
}

func (s *Supervisor) selfArgs(role string) []string {
	return []string{role}
}

func (s *Supervisor) startLiteLLM(configPath string) error {
	exe := s.paths.LitellmExe
	if exe == "" {
		exe = os.Getenv("OT_LITELLM_EXE")
	}
	if exe == "" {
		return errors.New("OT_LITELLM_EXE is not set; point it at the pinned LiteLLM executable")
	}
	if _, err := os.Stat(exe); err != nil {
		return fmt.Errorf("LiteLLM executable not found: %w", err)
	}
	env, err := s.litellmEnv()
	if err != nil {
		return err
	}
	args := []string{"--config", configPath, "--port", strconv.Itoa(config.LiteLLMPort), "--host", "127.0.0.1"}
	return s.startChildWithArgs("litellm", exe, args, env, s.paths.LiteLLM)
}

func (s *Supervisor) litellmEnv() (map[string]string, error) {
	env := map[string]string{
		"OT_LITELLM_KEY":  os.Getenv("OT_LITELLM_KEY"),
		"PYTHONIOENCODING": "utf-8",
		"PYTHONUTF8":       "1",
	}
	if env["OT_LITELLM_KEY"] == "" {
		return nil, errors.New("OT_LITELLM_KEY is missing from supervisor environment")
	}
	creds, err := s.store.ListCredentials()
	if err != nil {
		return nil, err
	}
	for _, c := range creds {
		if !c.Enabled {
			continue
		}
		// The value is read from Windows Credential Manager and passed only
		// through the child process environment, never argv, YAML or files.
		secret, err := secrets.Get(secrets.CredentialTarget(c.SecretRef))
		if err != nil {
			return nil, fmt.Errorf("credential reference %s is unavailable: %w", c.SecretRef, err)
		}
		env[c.SecretRef] = secret
	}
	return env, nil
}

func (s *Supervisor) startChild(name string, args []string, env map[string]string, logPath string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return s.startChildWithArgs(name, exe, args, env, logPath)
}

func (s *Supervisor) startChildWithArgs(name, exe string, args []string, env map[string]string, logPath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopping {
		return errors.New("supervisor is stopping")
	}
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()

	cmd := exec.Command(exe, args...)
	cmd.Env = mergeEnv(os.Environ(), env)
	cmd.Dir = s.paths.Root
	cmd.Stdout = f
	cmd.Stderr = f
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: win.CREATE_NEW_PROCESS_GROUP}
	if err := cmd.Start(); err != nil {
		return err
	}
	if err := s.job.Assign(uint32(cmd.Process.Pid)); err != nil {
		_ = cmd.Process.Kill()
		return fmt.Errorf("assign %s to job object: %w", name, err)
	}
	created, err := win.CreationTime(uint32(cmd.Process.Pid))
	if err != nil {
		_ = cmd.Process.Kill()
		return err
	}
	c := &child{name: name, cmd: cmd, created: created, lastTry: time.Now(), done: make(chan struct{})}
	if old := s.children[name]; old != nil {
		c.starts = old.starts + 1
	}
	s.children[name] = c
	if err := s.persistStateLocked(); err != nil {
		return err
	}
	s.logger.Printf("%s started pid=%d", name, cmd.Process.Pid)
	go func() {
		_ = cmd.Wait()
		close(c.done)
		select {
		case s.exited <- c:
		default:
		}
	}()
	return nil
}

func (s *Supervisor) handleExit(c *child) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopping || s.children[c.name] != c {
		return
	}
	s.logger.Printf("%s exited unexpectedly pid=%d", c.name, c.cmd.Process.Pid)
	delete(s.children, c.name)
	_ = s.persistStateLocked()
	delay := time.Duration(0)
	if time.Since(c.lastTry) < 10*time.Second {
		c.starts++
		if c.starts > 5 {
			delay = 30 * time.Second
		} else {
			delay = time.Duration(c.starts) * time.Second
		}
	} else {
		c.starts = 1
	}
	if delay > 0 {
		s.logger.Printf("%s restart backoff %s", c.name, delay)
		time.Sleep(delay)
	}
	if s.stopping {
		return
	}
	var err error
	switch c.name {
	case "gateway":
		err = s.startChildLocked("gateway", "", s.selfArgs("serve-gateway"), s.commonEnv(), s.paths.Gateway, c)
	case "control":
		err = s.startChildLocked("control", "", s.selfArgs("serve-control"), s.commonEnv(), s.paths.Control, c)
	case "litellm":
		env, envErr := s.litellmEnv()
		if envErr != nil {
			s.logger.Printf("litellm restart blocked: credential unavailable")
			return
		}
		args := []string{"--config", s.configPath, "--port", strconv.Itoa(config.LiteLLMPort), "--host", "127.0.0.1"}
		err = s.startChildLocked("litellm", s.paths.LitellmExe, args, env, s.paths.LiteLLM, c)
	}
	if err != nil {
		s.logger.Printf("%s restart failed: %v", c.name, sanitizeErr(err))
	}
}

// startChildLocked is a variant that accepts an existing child restart counter.
func (s *Supervisor) startChildLocked(name, exe string, args []string, env map[string]string, logPath string, previous *child) error {
	if exe == "" {
		exe = os.Getenv("OT_LITELLM_EXE")
	}
	if exe == "" || name == "gateway" || name == "control" {
		self, err := os.Executable()
		if err != nil {
			return err
		}
		exe = self
	}
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	cmd := exec.Command(exe, args...)
	cmd.Env = mergeEnv(os.Environ(), env)
	cmd.Dir = s.paths.Root
	cmd.Stdout = f
	cmd.Stderr = f
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: win.CREATE_NEW_PROCESS_GROUP}
	if err := cmd.Start(); err != nil {
		return err
	}
	if err := s.job.Assign(uint32(cmd.Process.Pid)); err != nil {
		_ = cmd.Process.Kill()
		return err
	}
	created, err := win.CreationTime(uint32(cmd.Process.Pid))
	if err != nil {
		_ = cmd.Process.Kill()
		return err
	}
	c := &child{name: name, cmd: cmd, created: created, starts: previous.starts, lastTry: time.Now(), done: make(chan struct{})}
	s.children[name] = c
	_ = s.persistStateLocked()
	s.logger.Printf("%s restarted pid=%d", name, cmd.Process.Pid)
	go func() {
		_ = cmd.Wait()
		close(c.done)
		select {
		case s.exited <- c:
		default:
		}
	}()
	return nil
}

func (s *Supervisor) shutdown() {
	s.mu.Lock()
	s.stopping = true
	s.mu.Unlock()
	s.stopAll()
	if s.job != nil {
		_ = s.job.Close()
	}
	state, err := config.LoadState(s.paths.State)
	if err == nil {
		state.GatewayPID, state.ControlPID, state.LiteLLMPID = 0, 0, 0
		state.UpdatedAt = time.Now().UTC()
		_ = config.SaveState(s.paths.State, state)
	}
	s.logger.Printf("supervisor stopped")
}

func (s *Supervisor) stopAll() {
	s.mu.Lock()
	children := make([]*child, 0, len(s.children))
	for _, c := range s.children {
		children = append(children, c)
	}
	s.mu.Unlock()
	for _, c := range children {
		stopChild(c, s.logger)
	}
}

func stopChild(c *child, logger *log.Logger) {
	if c == nil || c.cmd == nil || c.cmd.Process == nil {
		return
	}
	pid := uint32(c.cmd.Process.Pid)
	if c.created != 0 && !win.SameProcess(pid, c.created) {
		logger.Printf("%s pid reuse detected; refusing to signal pid=%d", c.name, pid)
		return
	}
	done := c.done
	if done == nil {
		done = make(chan struct{})
	}
	_ = win.SignalGraceful(c.cmd.Process)
	select {
	case <-done:
	case <-time.After(4 * time.Second):
		_ = c.cmd.Process.Kill()
		<-done
	}
	logger.Printf("%s stopped pid=%d", c.name, pid)
}

func (s *Supervisor) persistStateLocked() error {
	state, err := config.LoadState(s.paths.State)
	if err != nil {
		state = config.ProcessState{Version: "mvp-0.1.0", SupervisorPID: os.Getpid(), StartedAt: time.Now().UTC()}
	}
	for name, c := range s.children {
		pid := 0
		if c.cmd != nil && c.cmd.Process != nil {
			pid = c.cmd.Process.Pid
		}
		switch name {
		case "gateway":
			state.GatewayPID, state.GatewayCreation = pid, c.created
		case "control":
			state.ControlPID, state.ControlCreation = pid, c.created
		case "litellm":
			state.LiteLLMPID, state.LiteLLMCreation = pid, c.created
		}
	}
	state.GenerationID = activeGenerationID(s.store)
	state.LiteLLMConfig = s.configPath
	state.UpdatedAt = time.Now().UTC()
	return config.SaveState(s.paths.State, state)
}

func activeGenerationID(store *database.Store) int64 {
	if store == nil {
		return 0
	}
	v, err := store.Setting("active_generation_id")
	if err != nil || v == "" {
		return 0
	}
	id, _ := strconv.ParseInt(v, 10, 64)
	return id
}

func mergeEnv(base []string, overrides map[string]string) []string {
	out := make([]string, 0, len(base)+len(overrides))
	seen := map[string]bool{}
	for _, e := range base {
		key := strings.SplitN(e, "=", 2)[0]
		if _, ok := overrides[key]; ok {
			continue
		}
		out = append(out, e)
		seen[key] = true
	}
	for k, v := range overrides {
		out = append(out, k+"="+v)
	}
	return out
}

func sanitizeErr(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if len(msg) > 160 {
		msg = msg[:160]
	}
	return strings.ReplaceAll(strings.ReplaceAll(msg, "\n", " "), "\r", " ")
}

type activationRequest struct {
	ID     string `json:"id"`
	Config string `json:"config_path"`
}

type activationResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// handleActivationRequest applies a config reload request from the CLI.
func (s *Supervisor) handleActivationRequest() {
	raw, err := os.ReadFile(filepath.Join(s.paths.RunDir, "activate.request"))
	if err != nil {
		return
	}
	_ = os.Remove(filepath.Join(s.paths.RunDir, "activate.request"))
	var req activationRequest
	resp := activationResponse{}
	if err := json.Unmarshal(raw, &req); err != nil || req.Config == "" {
		resp.Error = "invalid activation request"
	} else {
		resp.OK, resp.Error = s.activateConfig(req.Config)
	}
	out, _ := json.Marshal(resp)
	_ = os.WriteFile(filepath.Join(s.paths.RunDir, "activate."+req.ID+".response"), out, 0o600)
}

func (s *Supervisor) activateConfig(path string) (bool, string) {
	if _, err := os.Stat(path); err != nil {
		return false, "config file not found"
	}
	s.mu.Lock()
	old := s.configPath
	s.configPath = path
	s.mu.Unlock()
	if err := s.restartLiteLLMForActivation(path); err != nil {
		s.mu.Lock()
		s.configPath = old
		s.mu.Unlock()
		return false, sanitizeErr(err)
	}
	return true, ""
}

func (s *Supervisor) restartLiteLLMForActivation(path string) error {
	s.mu.Lock()
	c := s.children["litellm"]
	s.mu.Unlock()
	if c != nil && c.cmd != nil && c.cmd.Process != nil {
		stopChild(c, s.logger)
		s.mu.Lock()
		delete(s.children, "litellm")
		_ = s.persistStateLocked()
		s.mu.Unlock()
	}
	env, err := s.litellmEnv()
	if err != nil {
		return err
	}
	args := []string{"--config", path, "--port", strconv.Itoa(config.LiteLLMPort), "--host", "127.0.0.1"}
	exe := s.paths.LitellmExe
	if exe == "" {
		exe = os.Getenv("OT_LITELLM_EXE")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.startChildWithArgsLocked("litellm", exe, args, env, s.paths.LiteLLM); err != nil {
		return err
	}
	return waitHTTP("http://127.0.0.1:"+strconv.Itoa(config.LiteLLMPort)+"/health/liveliness", 20*time.Second)
}

func (s *Supervisor) startChildWithArgsLocked(name, exe string, args []string, env map[string]string, logPath string) error {
	// Caller holds s.mu; startChildWithArgs locks, so inline the start here.
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	cmd := exec.Command(exe, args...)
	cmd.Env = mergeEnv(os.Environ(), env)
	cmd.Dir = s.paths.Root
	cmd.Stdout = f
	cmd.Stderr = f
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: win.CREATE_NEW_PROCESS_GROUP}
	if err := cmd.Start(); err != nil {
		return err
	}
	if err := s.job.Assign(uint32(cmd.Process.Pid)); err != nil {
		_ = cmd.Process.Kill()
		return err
	}
	created, err := win.CreationTime(uint32(cmd.Process.Pid))
	if err != nil {
		_ = cmd.Process.Kill()
		return err
	}
	c := &child{name: name, cmd: cmd, created: created, lastTry: time.Now(), done: make(chan struct{})}
	s.children[name] = c
	_ = s.persistStateLocked()
	go func() {
		_ = cmd.Wait()
		close(c.done)
	}()
	return nil
}

func waitHTTP(url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 1 * time.Second}
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 512))
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return nil
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	return errors.New("service did not become ready in time")
}

// RequestActivation asks a running supervisor to reload a generated config.
func RequestActivation(paths config.Paths, configPath string, timeout time.Duration) error {
	if !Running(paths) {
		state, err := config.LoadState(paths.State)
		if err != nil {
			state = config.ProcessState{}
		}
		state.LiteLLMConfig = configPath
		state.UpdatedAt = time.Now().UTC()
		return config.SaveState(paths.State, state)
	}
	id := strconv.FormatInt(time.Now().UnixNano(), 10)
	req := activationRequest{ID: id, Config: configPath}
	raw, err := json.Marshal(req)
	if err != nil {
		return err
	}
	requestPath := filepath.Join(paths.RunDir, "activate.request")
	responsePath := filepath.Join(paths.RunDir, "activate."+id+".response")
	if err := os.WriteFile(requestPath, raw, 0o600); err != nil {
		return err
	}
	defer os.Remove(responsePath)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(responsePath); err == nil {
			var resp activationResponse
			if err := json.Unmarshal(b, &resp); err != nil {
				return errors.New("invalid activation response")
			}
			if !resp.OK {
				return errors.New(resp.Error)
			}
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("activation timed out")
}

// RequestStop asks a running supervisor to shut down all children.
func RequestStop(paths config.Paths, timeout time.Duration) error {
	if !Running(paths) {
		_ = os.Remove(paths.StopFlag)
		return nil
	}
	if err := os.WriteFile(paths.StopFlag, []byte("stop"), 0o600); err != nil {
		return err
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !Running(paths) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("stop timed out")
}
