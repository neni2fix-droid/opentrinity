# OPENTRINITY / NENIFIX — local AI gateway (MVP)

A Windows-first, local-only OpenAI-compatible gateway for integrating AI features into company products. Go handles authentication, limits, process lifecycle, configuration and the CLI. **LiteLLM is the only inference router** — it alone maps the public alias `nenifix-coder` to up to three provider slots and performs ordered A→B→C fallback.

This is the narrowed MVP defined in `docs/MVP_BUILD_PROMPT.md`, not the complete v1.6.6 release.

## Product promise

```text
Base URL: http://127.0.0.1:3888/v1
API key:  local OPENTRINITY gateway key
Model:    nenifix-coder
```

Supported ingress (exact allowlist):

- `GET /health`
- `GET /ready`
- `GET /v1/models`
- `POST /v1/chat/completions`
- `POST /v1/messages` (Anthropic-compatible endpoint for Claude Code; forwarded to LiteLLM's beta `/v1/messages` router)

Everything else is rejected before reaching LiteLLM. All listeners bind only to `127.0.0.1`: gateway 3888, LiteLLM 3889, control 8369.

## Compatibility — carefully bounded

This is an OpenAI-compatible **subset**, tested for non-streaming chat completions, SSE streaming, basic tool-call pass-through and the `/v1/models` listing. It also exposes a small Anthropic-compatible `/v1/messages` route for Claude Code. Universal OpenAI or Anthropic API compatibility is **not** claimed. Fallback means a request may be sent to a second or third provider if the first fails; disclose this to users when configuring real providers.

No free-tier provider, model, price, quota or uptime is guaranteed or claimed. Live provider availability was not tested because no owner credential was supplied.

## Requirements

- Windows 11 (tested on build 26100), no administrator rights required.
- Go 1.26.3 was used to build the binary; the shipped `dist/opentrinity.exe` needs no Go toolchain at runtime.
- LiteLLM 1.74.15 in a CPython 3.11 virtual environment. The acceptance environment used `.phase0/venv2/Scripts/litellm.exe`.
- Windows Credential Manager for all secret values.

## Build

```bash
cd C:/Users/ai9/Documents/opentrinity
CGO_ENABLED=0 go build -o dist/opentrinity.exe ./cmd/opentrinity
```

Pinned dependencies are in `go.mod`/`go.sum`:

| Dependency | Version | Purpose |
|---|---:|---|
| `modernc.org/sqlite` | 1.62.0 | CGO-free configuration database |
| `github.com/charmbracelet/bubbletea` | 1.3.10 | small terminal UI |
| `github.com/danieljoos/wincred` | 1.2.3 | Windows Credential Manager |
| `golang.org/x/term` | 0.48.0 | masked terminal input |
| `golang.org/x/sys` | 0.50.0 | Windows Job Object containment |

## First-time setup with a real provider

All commands use the CLI executable. Secrets are never accepted as arguments.

```bash
set OT_LITELLM_EXE=C:\Users\ai9\Documents\opentrinity\.phase0\venv2\Scripts\litellm.exe

dist\opentrinity.exe init
dist\opentrinity.exe providers add openai https://api.openai.com/v1
dist\opentrinity.exe credentials add 1            # prompts with masked input
dist\opentrinity.exe models add 1 gpt-4o-mini
dist\opentrinity.exe deployments set A 1
dist\opentrinity.exe apply run
dist\opentrinity.exe start
dist\opentrinity.exe status
dist\opentrinity.exe gateway key show             # requires typing SHOW
```

To configure fallback slots B and C, add more providers/credentials/models and assign them:

```bash
dist\opentrinity.exe deployments set B 2
dist\opentrinity.exe deployments set C 3
dist\opentrinity.exe apply run
```

Slots must be contiguous starting at A. The generated LiteLLM config uses `os.environ/<reference>` placeholders; the supervisor reads actual values from Windows Credential Manager and supplies them only in the child process environment.

## Client example

```bash
curl http://127.0.0.1:3888/v1/chat/completions \
  -H "Authorization: Bearer <gateway-key>" \
  -H "Content-Type: application/json" \
  -d "{\"model\":\"nenifix-coder\",\"messages\":[{\"role\":\"user\",\"content\":\"Hello\"}]}"
```

Streaming: add `"stream": true`. Tool calls: pass standard `tools` and `tool_choice` fields; arguments are passed through without inspection or logging.

## Coding agent CLI setup

Both CLIs use the same local gateway key and the `nenifix-coder` alias. The gateway key is read from Windows Credential Manager by a helper, so it is not written into either CLI config as plain text.

### Codex CLI

`~/.codex/config.toml`:

```toml
model = "nenifix-coder"
model_provider = "opentrinity"

[model_providers.opentrinity]
name = "OPENTRINITY local gateway"
base_url = "http://127.0.0.1:3888/v1"
env_key = "OPENTRINITY_API_KEY"
wire_api = "responses"
```

Run with:

```bash
OPENTRINITY_API_KEY=$(printf 'SHOW\n' | dist/opentrinity.exe gateway key show | tail -1) codex exec "hello"
```

Current Codex releases require `wire_api = "responses"`; the gateway forwards `/v1/responses` to LiteLLM.

### Claude Code CLI

`~/.claude/settings.json` uses `apiKeyHelper` with `scripts/claude_gateway_key.py`, `ANTHROPIC_BASE_URL=http://127.0.0.1:3888` and `ANTHROPIC_MODEL=nenifix-coder`. The helper reads the gateway key from Credential Manager at run time. Claude Code's `/v1/messages` calls are forwarded to LiteLLM's beta Anthropic-format router, not to the raw Anthropic pass-through endpoint.

Run with:

```bash
claude -p "hello"
```

## Interactive TUI

```bash
dist\opentrinity.exe tui
```

The TUI offers status, provider/credential setup with masked secret entry, A/B/C deployment assignment, apply/rollback confirmation, help and quit. It calls the same services as the CLI. Non-interactive terminals fail with a helpful message instead of hanging.

## Other commands

```bash
dist\opentrinity.exe providers list
dist\opentrinity.exe models list
dist\opentrinity.exe credentials list
dist\opentrinity.exe credentials rotate <id>
dist\opentrinity.exe deployments list
dist\opentrinity.exe apply validate
dist\opentrinity.exe apply history
dist\opentrinity.exe apply rollback
dist\opentrinity.exe doctor
dist\opentrinity.exe restart
dist\opentrinity.exe stop
```

For automation only, `credentials stdin <provider-id> [label]` reads one secret line from stdin. It never accepts a secret as a command-line argument and must not be used with untrusted input.

## Local deterministic demo

`bash scripts/launch-demo.sh` starts the bundled fake upstream on loopback ports 11001–11003, configures three fake providers on first run, applies a generated config and starts OPENTRINITY. Demo state lives in `.demo-userdata` inside the project. The fake upstream returns fixed text — **it is a routing test fixture, not a real AI model**. Use real provider credentials for actual AI features.

## Testing

```bash
go test -vet=off ./...
bash scripts/acceptance/run.sh
```

The acceptance script proves A→B→C fallback with real per-slot counters, streaming commit behaviour, cancellation, tool pass-through, sanitized errors, loopback-only binding, singleton/child restart behaviour and a secret-canary scan. Results are in `docs/PHASE_REPORTS/evidence/acceptance-summary.txt`. Gate reports are in `docs/PHASE_REPORTS/`.

## Privacy and security notes

- Provider keys and the local gateway key are stored only in Windows Credential Manager. SQLite stores references, never values.
- Secrets never appear in YAML, argv, logs, events, diagnostics, URLs, source control or generated reports.
- Prompts, completions and tool arguments are not logged or persisted by this MVP.
- All settings are local-first. There is no remote administration, tunnel, LAN binding or UPnP.
- The supervisor uses a Windows Job Object so child processes cannot outlive it.
- `apply run` validates a candidate, activates it and rolls back to last-known-good if health checks fail.

## Known limitations

- Windows only; Linux support is deferred.
- No web dashboard, browser sessions, CSRF flow, event spool, analytics, backup UX, installer, updater or uninstaller.
- No live provider test was performed; no free-tier availability, price or quota claim is made.
- Cooldown, same-slot retries and provider classification by Go are disabled/absent by design.
- Role-only SSE commit point remains unobservable with the pinned LiteLLM/harness combination and is explicitly reported as skipped. Streamed tool-call deltas were verified with an extended fake upstream.
- LiteLLM crash recovery and port-conflict reporting were verified in the final hardening run.

## Release status

No publish, push, deployment or public exposure was performed. External release requires explicit owner approval and a target environment.
