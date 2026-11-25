//go:build windows

// Command opentrinity is the local control CLI and supervisor host.
package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/nenifix/opentrinity/internal/apply"
	"github.com/nenifix/opentrinity/internal/config"
	"github.com/nenifix/opentrinity/internal/control"
	"github.com/nenifix/opentrinity/internal/database"
	"github.com/nenifix/opentrinity/internal/gateway"
	"github.com/nenifix/opentrinity/internal/routingconfig"
	"github.com/nenifix/opentrinity/internal/secrets"
	"github.com/nenifix/opentrinity/internal/supervisor"
	"github.com/nenifix/opentrinity/internal/tui"
	"github.com/nenifix/opentrinity/internal/platform/windows"
	"golang.org/x/term"
)

const version = "mvp-0.1.0"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return nil
	}
	paths, err := config.DefaultPaths()
	if err != nil {
		return err
	}
	switch args[0] {
	case "-h", "--help", "help":
		usage()
		return nil
	case "init":
		return cmdInit(paths)
	case "start":
		return cmdStart(paths)
	case "stop":
		return supervisor.RequestStop(paths, 20*time.Second)
	case "restart":
		if err := supervisor.RequestStop(paths, 20*time.Second); err != nil {
			return err
		}
		return cmdStart(paths)
	case "status":
		return cmdStatus(paths)
	case "doctor":
		return cmdDoctor(paths)
	case "serve-supervisor":
		return serveSupervisor(paths)
	case "serve-gateway":
		return serveGateway(paths)
	case "serve-control":
		return serveControl(paths)
	case "providers":
		return cmdProviders(paths, args[1:])
	case "models":
		return cmdModels(paths, args[1:])
	case "credentials":
		return cmdCredentials(paths, args[1:])
	case "deployments":
		return cmdDeployments(paths, args[1:])
	case "apply":
		return cmdApply(paths, args[1:])
	case "gateway":
		return cmdGateway(paths, args[1:])
	case "tui":
		return cmdTUI(paths)
	default:
		usage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func usage() {
	fmt.Print(`OPENTRINITY — local OpenAI-compatible gateway (MVP)

Usage:
  opentrinity init
  opentrinity start | stop | restart | status | doctor
  opentrinity providers list | add <name> <api-base> [kind]
  opentrinity models list | add <provider-id> <model-id> [label]
  opentrinity credentials list | add <provider-id> [label] | stdin <provider-id> [label] | rotate <id> | disable <id>
  opentrinity deployments list | set <slot> <model-id> | enable <slot> | disable <slot>
  opentrinity apply validate | run [generation-id] | history | rollback
  opentrinity gateway key rotate | show
  opentrinity tui

Client base URL: http://127.0.0.1:3888/v1
Client model:    nenifix-coder
`)
}

func openStore(paths config.Paths) (*database.Store, error) {
	if err := paths.Ensure(); err != nil {
		return nil, err
	}
	return database.Open(paths.DB)
}

func logger(path string) (*log.Logger, io.Closer, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, nil, err
	}
	return log.New(f, "", log.LstdFlags|log.LUTC), f, nil
}

func cmdInit(paths config.Paths) error {
	if err := paths.Ensure(); err != nil {
		return err
	}
	store, err := openStore(paths)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.Integrity(); err != nil {
		return err
	}
	if _, err := secrets.Get(secrets.GatewayTarget()); errors.Is(err, secrets.ErrNotFound) {
		key, err := secrets.NewGatewayKey()
		if err != nil {
			return err
		}
		if err := secrets.Set(secrets.GatewayTarget(), key); err != nil {
			return err
		}
		fmt.Println("gateway key created in Windows Credential Manager (not displayed)")
	}
	if _, err := secrets.Get(secrets.LiteLLMKeyTarget()); errors.Is(err, secrets.ErrNotFound) {
		key, err := secrets.NewGatewayKey()
		if err != nil {
			return err
		}
		if err := secrets.Set(secrets.LiteLLMKeyTarget(), key); err != nil {
			return err
		}
		fmt.Println("private LiteLLM key created in Windows Credential Manager (not displayed)")
	}
	fmt.Printf("initialized: %s\n", paths.Root)
	return nil
}

func resolveLitellmExe() (string, error) {
	if v := os.Getenv("OT_LITELLM_EXE"); v != "" {
		if _, err := os.Stat(v); err == nil {
			return v, nil
		}
	}
	candidates := []string{}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, ".phase0", "venv2", "Scripts", "litellm.exe"))
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "..", ".phase0", "venv2", "Scripts", "litellm.exe"))
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", errors.New("LiteLLM executable not found; set OT_LITELLM_EXE")
}

func cmdStart(paths config.Paths) error {
	if supervisor.Running(paths) {
		return errors.New("supervisor is already running")
	}
	if conflict := portConflictMessage(); conflict != "" {
		return errors.New(conflict)
	}
	store, err := openStore(paths)
	if err != nil {
		return err
	}
	defer store.Close()
	if _, err := os.Stat(paths.Generated); errors.Is(err, os.ErrNotExist) {
		mgr := &apply.Manager{Paths: paths, Store: store, Activate: func(string) error { return nil }}
		if _, err := mgr.Run(0); err != nil {
			return fmt.Errorf("no active configuration and automatic apply failed: %w", err)
		}
	}
	gatewayKey, err := secrets.Get(secrets.GatewayTarget())
	if err != nil {
		return err
	}
	litellmKey, err := secrets.Get(secrets.LiteLLMKeyTarget())
	if err != nil {
		return err
	}
	litellmExe, err := resolveLitellmExe()
	if err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	logf, err := os.OpenFile(paths.Supervisor, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer logf.Close()
	cmd := exec.Command(self, "serve-supervisor")
	cmd.Env = mergeEnv(os.Environ(), map[string]string{
		"OT_USER_DATA":    paths.Root,
		"OT_GATEWAY_KEY":  gatewayKey,
		"OT_LITELLM_KEY":  litellmKey,
		"OT_LITELLM_EXE":  litellmExe,
	})
	cmd.Dir = paths.Root
	cmd.Stdout = logf
	cmd.Stderr = logf
	// Detach the supervisor from the launching console so it survives the
	// shell that started it. Without this the whole stack is torn down when
	// the parent process group exits.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS,
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	// Release the supervisor's handle so it is not killed with this process.
	_ = cmd.Process.Release()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if supervisor.Running(paths) && tcpOpen("127.0.0.1:3888") {
			fmt.Println("OPENTRINITY started on 127.0.0.1:3888 (gateway), 3889 (LiteLLM), 8369 (control)")
			return nil
		}
		time.Sleep(150 * time.Millisecond)
	}
	return errors.New("supervisor did not become ready; check logs/supervisor.log")
}

func serveSupervisor(paths config.Paths) error {
	if os.Getenv("OT_GATEWAY_KEY") == "" {
		key, err := secrets.Get(secrets.GatewayTarget())
		if err != nil {
			return fmt.Errorf("gateway key unavailable: %w", err)
		}
		if err := os.Setenv("OT_GATEWAY_KEY", key); err != nil {
			return err
		}
	}
	if os.Getenv("OT_LITELLM_KEY") == "" {
		key, err := secrets.Get(secrets.LiteLLMKeyTarget())
		if err != nil {
			return fmt.Errorf("LiteLLM key unavailable: %w", err)
		}
		if err := os.Setenv("OT_LITELLM_KEY", key); err != nil {
			return err
		}
	}
	store, err := openStore(paths)
	if err != nil {
		return err
	}
	defer store.Close()
	l, closer, err := logger(paths.Supervisor)
	if err != nil {
		return err
	}
	defer closer.Close()
	configPath := paths.Generated
	if state, err := config.LoadState(paths.State); err == nil && state.LiteLLMConfig != "" {
		configPath = state.LiteLLMConfig
	}
	configPath, err = filepath.Abs(configPath)
	if err != nil {
		return err
	}
	return supervisor.New(paths, store, l).Run(configPath)
}

func serveGateway(paths config.Paths) error {
	l, closer, err := logger(paths.Gateway)
	if err != nil {
		return err
	}
	defer closer.Close()
	return gateway.Run(paths, l)
}

func serveControl(paths config.Paths) error {
	l, closer, err := logger(paths.Control)
	if err != nil {
		return err
	}
	defer closer.Close()
	return control.Run(paths, l)
}

func cmdStatus(paths config.Paths) error {
	state, err := config.LoadState(paths.State)
	if err != nil {
		fmt.Println("status: not initialized")
		return nil
	}
	status := "running"
	if !supervisor.Running(paths) {
		status = "stopped"
	}
	fmt.Printf("status: %s\nversion: %s\ngeneration: %d\nstarted: %s\n",
		status, state.Version, state.GenerationID, state.StartedAt.Format(time.RFC3339))
	for name, port := range map[string]int{"gateway": config.GatewayPort, "litellm": config.LiteLLMPort, "control": config.ControlPort} {
		addr := "127.0.0.1:" + strconv.Itoa(port)
		fmt.Printf("%s: %s (%s)\n", name, addr, listenerState(addr))
	}
	return nil
}

// portConflictMessage reports a non-OPENTRINITY listener on a required port.
// It never terminates or modifies the owning process.
func portConflictMessage() string {
	out, err := exec.Command("netstat", "-ano", "-p", "TCP").Output()
	if err != nil {
		return ""
	}
	ports := map[string]string{
		":3888": "gateway",
		":3889": "LiteLLM",
		":8369": "control",
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || !strings.EqualFold(fields[3], "LISTENING") {
			continue
		}
		local := fields[1]
		for suffix, name := range ports {
			if !strings.HasSuffix(local, suffix) {
				continue
			}
			if !strings.Contains(local, "127.0.0.1") && !strings.Contains(local, "0.0.0.0") && !strings.Contains(local, "[::]") {
				continue
			}
			return fmt.Sprintf("port %s required by %s is already in use by PID %s; stop that process before starting OPENTRINITY", suffix[1:], name, fields[4])
		}
	}
	return ""
}

func listenerState(addr string) string {
	if tcpOpen(addr) {
		return "listening"
	}
	return "closed"
}

func tcpOpen(addr string) bool {
	c, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

func cmdDoctor(paths config.Paths) error {
	checks := []struct {
		name string
		ok   bool
	}{}
	_, statErr := os.Stat(paths.DB)
	checks = append(checks, struct {
		name string
		ok   bool
	}{"database exists", statErr == nil})
	if statErr == nil {
		store, err := database.Open(paths.DB)
		if err == nil {
			checks = append(checks, struct {
				name string
				ok   bool
			}{"database integrity", store.Integrity() == nil})
			store.Close()
		}
	}
	_, gwErr := secrets.Get(secrets.GatewayTarget())
	_, llmErr := secrets.Get(secrets.LiteLLMKeyTarget())
	checks = append(checks,
		struct {
			name string
			ok   bool
		}{"gateway key in Credential Manager", gwErr == nil},
		struct {
			name string
			ok   bool
		}{"LiteLLM key in Credential Manager", llmErr == nil},
	)
	exe, err := resolveLitellmExe()
	checks = append(checks, struct {
		name string
		ok   bool
	}{"LiteLLM executable: " + exe, err == nil})
	_, cfgErr := os.Stat(paths.Generated)
	checks = append(checks, struct {
		name string
		ok   bool
	}{"active generated config", cfgErr == nil})
	for _, c := range checks {
		result := "FAIL"
		if c.ok {
			result = "PASS"
		}
		fmt.Printf("%s %s\n", result, c.name)
	}
	return nil
}

func cmdProviders(paths config.Paths, args []string) error {
	store, err := openStore(paths)
	if err != nil {
		return err
	}
	defer store.Close()
	if len(args) == 0 || args[0] == "list" {
		items, err := store.ListProviders()
		if err != nil {
			return err
		}
		for _, p := range items {
			fmt.Printf("%d\t%s\t%s\t%t\n", p.ID, p.Name, p.APIBase, p.Enabled)
		}
		return nil
	}
	if args[0] == "add" && len(args) >= 3 {
		if err := routingconfig.ValidateAPIBase(args[2]); err != nil {
			return fmt.Errorf("invalid API base: %w", err)
		}
		kind := "openai-compatible"
		if len(args) > 3 {
			kind = args[3]
		}
		id, err := store.AddProvider(args[1], args[2], kind)
		if err != nil {
			return err
		}
		fmt.Printf("provider %d added\n", id)
		return nil
	}
	return errors.New("usage: providers list | add <name> <api-base> [kind]")
}

func cmdModels(paths config.Paths, args []string) error {
	store, err := openStore(paths)
	if err != nil {
		return err
	}
	defer store.Close()
	if len(args) == 0 || args[0] == "list" {
		items, err := store.ListModels()
		if err != nil {
			return err
		}
		for _, m := range items {
			fmt.Printf("%d\t%d\t%s\t%t\n", m.ID, m.ProviderID, m.ModelID, m.Enabled)
		}
		return nil
	}
	if args[0] == "add" && len(args) >= 3 {
		label := ""
		if len(args) > 3 {
			label = args[3]
		}
		id, err := store.AddModel(parseID(args[1]), args[2], label)
		if err != nil {
			return err
		}
		fmt.Printf("model %d added\n", id)
		return nil
	}
	return errors.New("usage: models list | add <provider-id> <model-id> [label]")
}

func cmdCredentials(paths config.Paths, args []string) error {
	store, err := openStore(paths)
	if err != nil {
		return err
	}
	defer store.Close()
	if len(args) == 0 || args[0] == "list" {
		items, err := store.ListCredentials()
		if err != nil {
			return err
		}
		for _, c := range items {
			fmt.Printf("%d\t%d\t%s\t%s\t%t\n", c.ID, c.ProviderID, c.Label, c.SecretRef, c.Enabled)
		}
		return nil
	}
	if args[0] == "add" && len(args) >= 2 {
		providerID := parseID(args[1])
		label := "primary"
		if len(args) > 2 {
			label = args[2]
		}
		value, err := secrets.ReadMasked("Provider key")
		if err != nil {
			return err
		}
		return saveCredential(store, providerID, label, value)
	}
	if args[0] == "stdin" && len(args) >= 2 {
		providerID := parseID(args[1])
		label := "primary"
		if len(args) > 2 {
			label = args[2]
		}
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		return saveCredential(store, providerID, label, strings.TrimSpace(line))
	}
	if args[0] == "rotate" && len(args) >= 2 {
		c, err := store.GetCredential(parseID(args[1]))
		if err != nil {
			return err
		}
		value, err := secrets.ReadMasked("New provider key")
		if err != nil {
			return err
		}
		if value == "" {
			return errors.New("secret cannot be empty")
		}
		if err := secrets.Set(credentialTarget(c.SecretRef), value); err != nil {
			return err
		}
		fmt.Printf("credential %d rotated; value not displayed\n", c.ID)
		return nil
	}
	if args[0] == "disable" && len(args) >= 2 {
		return store.SetCredentialEnabled(parseID(args[1]), false)
	}
	return errors.New("usage: credentials list | add <provider-id> [label] | rotate <id> | disable <id>")
}

func saveCredential(store *database.Store, providerID int64, label, value string) error {
	if providerID <= 0 {
		return errors.New("invalid provider id")
	}
	if label == "" {
		label = "primary"
	}
	if value == "" {
		return errors.New("secret cannot be empty")
	}
	ref, err := newSecretRef(providerID)
	if err != nil {
		return err
	}
	target := credentialTarget(ref)
	if err := secrets.Set(target, value); err != nil {
		return err
	}
	id, err := store.AddCredential(providerID, label, ref)
	if err != nil {
		_ = secrets.Delete(target)
		return err
	}
	fmt.Printf("credential %d stored in Windows Credential Manager (reference: %s)\n", id, ref)
	return nil
}

func newSecretRef(providerID int64) (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("OT_P%d_%s_KEY", providerID, strings.ToUpper(hex.EncodeToString(b))), nil
}

func credentialTarget(ref string) string { return secrets.CredentialTarget(ref) }

func cmdDeployments(paths config.Paths, args []string) error {
	store, err := openStore(paths)
	if err != nil {
		return err
	}
	defer store.Close()
	if len(args) == 0 || args[0] == "list" {
		items, err := store.ListDeployments()
		if err != nil {
			return err
		}
		for _, d := range items {
			fmt.Printf("%s\tmodel=%d\tname=%s\tenabled=%t\n", d.Slot, d.ModelID, d.ModelName, d.Enabled)
		}
		return nil
	}
	if len(args) >= 3 && (args[0] == "set") {
		return store.SetDeployment(strings.ToUpper(args[1]), parseID(args[2]), true)
	}
	if len(args) >= 2 && (args[0] == "enable" || args[0] == "disable") {
		return store.SetDeployment(strings.ToUpper(args[1]), 0, args[0] == "enable")
	}
	return errors.New("usage: deployments list | set <slot> <model-id> | enable <slot> | disable <slot>")
}

func activation(paths config.Paths) func(string) error {
	return func(configPath string) error {
		return supervisor.RequestActivation(paths, configPath, 40*time.Second)
	}
}

func cmdApply(paths config.Paths, args []string) error {
	store, err := openStore(paths)
	if err != nil {
		return err
	}
	defer store.Close()
	mgr := &apply.Manager{Paths: paths, Store: store, Activate: activation(paths)}
	if len(args) == 0 || args[0] == "validate" {
		g, err := mgr.Validate()
		if err != nil {
			return err
		}
		fmt.Printf("candidate generation %d validated (sha256=%s)\n", g.ID, g.SHA256)
		return nil
	}
	if args[0] == "run" {
		var id int64
		if len(args) > 1 {
			id = parseID(args[1])
		}
		g, err := mgr.Run(id)
		if err != nil {
			return err
		}
		fmt.Printf("generation %d applied\n", g.ID)
		return nil
	}
	if args[0] == "history" {
		generations, err := store.ListGenerations(20)
		if err != nil {
			return err
		}
		for _, g := range generations {
			fmt.Printf("generation\t%d\t%s\t%s\t%s\n", g.ID, g.CreatedAt, g.Status, g.SHA256)
		}
		runs, err := store.ListApplyRuns(20)
		if err != nil {
			return err
		}
		for _, r := range runs {
			fmt.Printf("apply\t%d\t%d\t%s\t%s\n", r.ID, r.GenerationID, r.Outcome, r.CreatedAt)
		}
		return nil
	}
	if args[0] == "rollback" {
		g, err := mgr.Rollback()
		if err != nil {
			return err
		}
		fmt.Printf("rolled back to generation %d\n", g.ID)
		return nil
	}
	return errors.New("usage: apply validate | run [generation-id] | history | rollback")
}

func cmdGateway(paths config.Paths, args []string) error {
	if len(args) >= 2 && args[0] == "key" && args[1] == "rotate" {
		key, err := secrets.NewGatewayKey()
		if err != nil {
			return err
		}
		if err := secrets.Set(secrets.GatewayTarget(), key); err != nil {
			return err
		}
		fmt.Println("gateway key rotated; update your client. Value not displayed.")
		return nil
	}
	if len(args) >= 2 && args[0] == "key" && args[1] == "show" {
		fmt.Fprint(os.Stderr, "Type SHOW to reveal the gateway key locally: ")
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		if strings.TrimSpace(line) != "SHOW" {
			return errors.New("confirmation not given")
		}
		key, err := secrets.Get(secrets.GatewayTarget())
		if err != nil {
			return err
		}
		fmt.Println(key)
		return nil
	}
	return errors.New("usage: gateway key rotate | show")
}

func cmdTUI(paths config.Paths) error {
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return errors.New("TUI requires an interactive terminal; use the CLI commands instead")
	}
	store, err := openStore(paths)
	if err != nil {
		return err
	}
	defer store.Close()
	mgr := &apply.Manager{Paths: paths, Store: store, Activate: activation(paths)}
	services := tui.Services{
		Status: func() string {
			state, err := config.LoadState(paths.State)
			if err != nil {
				return "not initialized"
			}
			litellm := "down"
			resp, err := (&http.Client{Timeout: time.Second}).Get("http://127.0.0.1:3889/health/liveliness")
			if err == nil {
				resp.Body.Close()
				litellm = "up"
			}
			return fmt.Sprintf("generation=%d litellm=%s gateway=%s control=%s",
				state.GenerationID, litellm, listenerState("127.0.0.1:3888"), listenerState("127.0.0.1:8369"))
		},
		Providers: func() ([]tui.Provider, error) {
			items, err := store.ListProviders()
			out := make([]tui.Provider, 0, len(items))
			for _, p := range items {
				out = append(out, tui.Provider{ID: p.ID, Name: p.Name, APIBase: p.APIBase})
			}
			return out, err
		},
		AddProvider: func(name, base string) error {
			if err := routingconfig.ValidateAPIBase(base); err != nil {
				return err
			}
			_, err := store.AddProvider(name, base, "openai-compatible")
			return err
		},
		AddCredential: func(providerID int64, label, secretValue string) error {
			ref, err := newSecretRef(providerID)
			if err != nil {
				return err
			}
			target := credentialTarget(ref)
			if err := secrets.Set(target, secretValue); err != nil {
				return err
			}
			if _, err := store.AddCredential(providerID, label, ref); err != nil {
				_ = secrets.Delete(target)
				return err
			}
			return nil
		},
		Models: func() ([]tui.Model, error) {
			models, err := store.ListModels()
			deployments, err2 := store.ListDeployments()
			if err != nil {
				return nil, err
			}
			if err2 != nil {
				return nil, err2
			}
			slotByModel := map[int64]string{}
			for _, d := range deployments {
				if d.Enabled {
					slotByModel[d.ModelID] = d.Slot
				}
			}
			out := make([]tui.Model, 0, len(models))
			for _, m := range models {
				out = append(out, tui.Model{ID: m.ID, Label: m.ModelID, Slot: slotByModel[m.ID], Enabled: m.Enabled})
			}
			return out, nil
		},
		SetDeployment: func(slot string, modelID int64) error {
			return store.SetDeployment(strings.ToUpper(slot), modelID, true)
		},
		Apply: func() error {
			_, err := mgr.Run(0)
			return err
		},
		Rollback: func() error {
			_, err := mgr.Rollback()
			return err
		},
	}
	return tui.Run(services)
}

func parseID(s string) int64 {
	v, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return v
}

func mergeEnv(base []string, overrides map[string]string) []string {
	out := make([]string, 0, len(base)+len(overrides))
	for _, e := range base {
		key := strings.SplitN(e, "=", 2)[0]
		if _, ok := overrides[key]; !ok {
			out = append(out, e)
		}
	}
	for k, v := range overrides {
		out = append(out, k+"="+v)
	}
	return out
}

var _ = context.Background
