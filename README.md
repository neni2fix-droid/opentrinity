# OpenTrinity

OpenTrinity is a local, Windows-first AI gateway that gives your tools one stable OpenAI-compatible endpoint while routing inference through **LiteLLM** behind the scenes.

It exposes a single local model alias, `nenifix-coder`, and lets you map that alias to up to three provider deployments (A/B/C) with ordered fallback. Go owns the local control plane — configuration, secrets, process lifecycle, validation, and the CLI/TUI. LiteLLM owns inference routing. That boundary is deliberate: one stable client contract, one routing engine.

> **Status:** MVP. This repository reflects the narrowed MVP build, not the complete v1.6.6 release vision.

## Why it exists

Product teams often want to integrate AI features without hard-coding a vendor SDK, spreading API keys across apps, or rewriting clients every time a model/provider changes. OpenTrinity gives you a local gateway contract:

```text
Base URL: http://127.0.0.1:3888/v1
API key:  <local OpenTrinity gateway key>
Model:    nenifix-coder
```

Your app talks to localhost. OpenTrinity handles the local gateway key, provider credentials, deployment slots, and safe apply/rollback of routing config.

## What it does

- **OpenAI-compatible local ingress** — A small, explicit allowlist of endpoints on `127.0.0.1:3888`.
- **Stable model alias** — Clients use `nenifix-coder`; you decide which provider/model sits behind slots A, B, and C.
- **Ordered fallback** — LiteLLM performs A→B→C fallback when configured and eligible.
- **Local secret handling** — Provider keys and the local gateway key are stored in Windows Credential Manager, not in SQLite, YAML, logs, or argv.
- **Atomic config apply** — Candidate config is validated, activated, and rolled back to last-known-good on failure.
- **Supervisor-managed processes** — Gateway, control plane, and LiteLLM run as local loopback services with lifecycle supervision.
- **CLI + TUI** — Manage providers, credentials, models, deployments, apply history, and diagnostics from the terminal.

## Supported API surface

OpenTrinity intentionally exposes a narrow allowlist:

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/health` | Gateway process liveness |
| `GET` | `/ready` | Gateway-to-LiteLLM readiness |
| `GET` | `/v1/models` | OpenAI-compatible model listing |
| `POST` | `/v1/chat/completions` | Chat completions, streaming and non-streaming |
| `POST` | `/v1/messages` | Anthropic-compatible route for Claude Code, forwarded through LiteLLM |

Everything else is rejected before reaching LiteLLM.

**Compatibility note:** This is a carefully bounded OpenAI-compatible subset, tested for non-streaming chat completions, SSE streaming, basic tool-call pass-through, and `/v1/models`. It also exposes a small Anthropic-compatible `/v1/messages` route for Claude Code. It does **not** claim universal OpenAI or Anthropic API compatibility.

## Architecture

```text
Client (Codex / Claude Code / your app)
            │
            ▼
OpenTrinity Gateway        127.0.0.1:3888
  - local bearer auth
  - exact route allowlist
  - request limits / cancellation
  - sanitized operational events
            │
            ▼
LiteLLM Proxy              127.0.0.1:3889  (private)
  - alias -> deployment routing
  - A / B / C fallback
            │
            ├── Provider slot A
            ├── Provider slot B
            └── Provider slot C

OpenTrinity Control        127.0.0.1:8369  (local control plane)
OpenTrinity Supervisor     owns lifecycle, apply, readiness, restart policy
SQLite (WAL)               config/state metadata only, no secret values
Windows Credential Manager provider keys + local gateway key
```

## Requirements

- **Windows 11** (tested on build 26100). No administrator rights required for normal operation.
- **Go 1.26+** to build from source. The shipped binary can run without a Go toolchain.
- **LiteLLM 1.74.15** in a CPython 3.11 virtual environment.
- **Windows Credential Manager** for secret storage.

## Build from source

```powershell
cd D:\opentrinity
$env:CGO_ENABLED = "0"
go build -o dist/opentrinity.exe ./cmd/opentrinity
```

## Quick start

All commands use the CLI. Secrets are never passed as command-line arguments.

```powershell
# Point OpenTrinity at your LiteLLM executable
$env:OT_LITELLM_EXE = "C:\path\to\litellm.exe"

# Initialize local state and keys
.\dist\opentrinity.exe init

# Add a provider, credential, and model
.\dist\opentrinity.exe providers add openai https://api.openai.com/v1
.\dist\opentrinity.exe credentials add 1
.\dist\opentrinity.exe models add 1 gpt-4o-mini

# Assign slot A and apply
.\dist\opentrinity.exe deployments set A 1
.\dist\opentrinity.exe apply run

# Start the local stack
.\dist\opentrinity.exe start
.\dist\opentrinity.exe status

# Reveal the local gateway key when you need it (requires typing SHOW)
.\dist\opentrinity.exe gateway key show
```

### Configure fallback

```powershell
.\dist\opentrinity.exe deployments set B 2
.\dist\opentrinity.exe deployments set C 3
.\dist\opentrinity.exe apply run
```

Slots must be contiguous starting at A. The generated LiteLLM config uses `os.environ/<reference>` placeholders; the supervisor resolves actual secret values from Windows Credential Manager and supplies them only to the child process environment.

## Use it from a client

### cURL

```bash
curl http://127.0.0.1:3888/v1/chat/completions \
  -H "Authorization: Bearer <gateway-key>" \
  -H "Content-Type: application/json" \
  -d "{\"model\":\"nenifix-coder\",\"messages\":[{\"role\":\"user\",\"content\":\"Hello\"}]}"
```

Add `"stream": true` for SSE streaming. Standard `tools` and `tool_choice` fields are passed through.

### Codex CLI

`~/.codex/config.toml`:

```toml
model = "nenifix-coder"
model_provider = "opentrinity"

[model_providers.opentrinity]
name = "OpenTrinity local gateway"
base_url = "http://127.0.0.1:3888/v1"
env_key = "OPENTRINITY_API_KEY"
wire_api = "responses"
```

### Claude Code

Configure `~/.claude/settings.json` to use `ANTHROPIC_BASE_URL=http://127.0.0.1:3888`, `ANTHROPIC_MODEL=nenifix-coder`, and an `apiKeyHelper` that reads the gateway key from Credential Manager at runtime (see `scripts/claude_gateway_key.py`). Claude Code `/v1/messages` calls are forwarded through LiteLLM's Anthropic-format router.

## CLI commands

```text
opentrinity init
opentrinity start | stop | restart | status | doctor
opentrinity providers list | add <name> <api-base> [kind]
opentrinity models list | add <provider-id> <model-id> [label]
opentrinity credentials list | add <provider-id> [label] | stdin <provider-id> [label] | rotate <id> | disable <id>
opentrinity deployments list | set <slot> <model-id> | enable <slot> | disable <slot>
opentrinity apply validate | run [generation-id] | history | rollback
opentrinity gateway key rotate | show
opentrinity tui
```

The TUI provides an interactive flow for status, provider/credential setup, deployment assignment, apply/rollback, and help. In non-interactive terminals it exits with a helpful message instead of hanging.

## Security and privacy

- **Secrets stay in Windows Credential Manager.** SQLite stores secret references, never secret values.
- **No secrets in generated config values.** LiteLLM YAML uses environment-reference placeholders.
- **No prompt/completion logging by default.** The MVP does not log or persist prompts, completions, or tool arguments.
- **Loopback only.** Gateway, LiteLLM, and control plane bind to `127.0.0.1`. No LAN binding, tunnel, or remote admin surface.
- **Process containment.** The supervisor uses a Windows Job Object so child processes do not outlive it.
- **Safe apply.** `apply run` validates a candidate, activates it, and rolls back to last-known-good if health checks fail.

## Local demo and tests

```bash
# Deterministic local routing demo (fake upstreams, not real AI models)
bash scripts/launch-demo.sh

# Unit tests
go test -vet=off ./...

# Acceptance suite
bash scripts/acceptance/run.sh
```

The demo uses fake upstreams on loopback ports and fixed responses. It is a routing/behavior fixture, not a real AI model. Use real provider credentials for actual AI features.

## Known limitations

- **Windows-first MVP.** Linux support is planned/design intent, not shipped in this MVP.
- **Narrow compatibility.** Not a universal OpenAI/Anthropic proxy.
- **No web dashboard.** Control plane and TUI/CLI exist; a full browser dashboard is not part of this MVP.
- **No live provider proof in this repo.** No free-tier availability, price, quota, or uptime is claimed.
- **LiteLLM is the router.** Go does not implement provider selection, retries, cooldown, or fallback policy.
- **Some streaming edge cases are documented.** See `docs/PHASE_REPORTS/` for gate evidence and known skips.

## Repository layout

```text
cmd/opentrinity/     CLI entrypoint and process modes
internal/apply/      config validation, apply, rollback
internal/config/     paths and runtime config
internal/control/    local control plane
internal/database/   SQLite state
internal/gateway/    OpenAI-compatible ingress
internal/platform/   OS-specific process containment
internal/routingconfig/  LiteLLM config generation
internal/secrets/    Windows Credential Manager integration
internal/supervisor/ child process lifecycle
internal/tui/        terminal UI
docs/                specs, phase reports, evidence
scripts/             demo, acceptance, helpers
tests/fakeupstream/  deterministic upstream fixture
```

## Release status

This repository is an MVP build with local acceptance evidence under `docs/PHASE_REPORTS/`. It has not been positioned here as a public, production-hardened release. Review provider terms, data handling, and fallback behavior before routing real user data through third-party providers.