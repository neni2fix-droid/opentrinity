# OPENTRINITY MVP — handoff

Date: 2026-11-24  
Status: built, tested and launched locally. No public deployment performed.

## What is running now

A loopback-only instance is active with three real OpenRouter providers in generation 3:

| Component | Address                    | Purpose                                  |
| --------- | -------------------------- | ---------------------------------------- |
| Gateway   | `http://127.0.0.1:3888/v1` | OpenAI/Anthropic-compatible ingress      |
| LiteLLM   | `127.0.0.1:3889`           | private inference router                 |
| Control   | `127.0.0.1:8369`           | read-only status API                     |

Configured fallback slots:

| Slot | Model                                   |
| ---- | --------------------------------------- |
| A    | `nvidia/nemotron-3.5-super-vl:free`     |
| B    | `particle-ai/particle-1:free`           |
| C    | `baidu/ernie-5.5-flash-preview:free`    |

A live chat completion through slot A returned the exact requested text with HTTP 200. The earlier fake upstreams remain as test fixtures only and are no longer the active configuration.

Client model alias: `nenifix-coder`.

Retrieve the local gateway key with:

```bash
cd C:/Users/ai9/Documents/opentrinity
set OT_USER_DATA=C:\Users\ai9\Documents\opentrinity\.demo-userdata
dist\opentrinity.exe gateway key show
```

It requires typing `SHOW` and is never written to logs or files.

Example request:

```bash
curl http://127.0.0.1:3888/v1/chat/completions \
  -H "Authorization: Bearer <gateway-key>" \
  -H "Content-Type: application/json" \
  -d "{\"model\":\"nenifix-coder\",\"messages\":[{\"role\":\"user\",\"content\":\"Hello\"}]}"
```

## Coding agent CLI setup

- **Codex CLI 0.171.0**: `~/.codex/config.toml` now uses `model = "nenifix-coder"`, `model_provider = "opentrinity"`, `base_url = "http://127.0.0.1:3888/v1"`, `env_key = "OPENTRINITY_API_KEY"` and `wire_api = "responses"`. The native executable at `C:\Users\ai9\AppData\Local\Programs\OpenAI\Codex\bin\codex.exe` must be used because the `codex` shim on PATH points at a missing module.
- **Claude Code 2.1.331**: `~/.claude/settings.json` now points `ANTHROPIC_BASE_URL` at the local gateway, pins all built-in model aliases to `nenifix-coder`, and uses `scripts/claude_gateway_key.py` as `apiKeyHelper` so the gateway key is read from Credential Manager at run time. The previously stored plaintext OpenRouter key in that file was removed.

## Files delivered

- `dist/opentrinity.exe` — Windows/amd64 CLI and supervisor host.
- `README.md` — setup, client usage, security and limitations.
- `cmd/opentrinity/main.go` — CLI and process host.
- `internal/gateway` — ingress authentication, allowlist, limits, SSE pass-through and sanitization.
- `internal/supervisor` — singleton, child supervision, restart and Windows Job Object containment.
- `internal/database`, `internal/routingconfig`, `internal/apply`, `internal/secrets` — config store, YAML generation, safe apply and Credential Manager secrets.
- `internal/control` — local read-only status API.
- `internal/tui` — Bubble Tea terminal UI with masked secret entry.
- `scripts/acceptance/run.sh` — deterministic end-to-end acceptance suite.
- `scripts/launch-demo.sh` — local demo launcher.
- `scripts/claude_gateway_key.py` — Claude Code key helper; reads the gateway key from Credential Manager without writing it to disk.
- `docs/PHASE_REPORTS/phase-0.md` … `phase-5.md` — gate reports with real evidence.
- `docs/PHASE_REPORTS/evidence/` — acceptance summary, stream samples and tool response.

## Verification summary

- `go test -vet=off ./...` — PASS.
- `bash scripts/acceptance/run.sh` — GATE RESULT: PASS.
- Real A/B/C counters: A only `1/0/0`, A→B `1/1/0`, A→B→C `1/1/1`, all fail `1/1/1` with Go-sanitized HTTP 502.
- Streaming: pre-commit fallback `1/1/0`; post-commit no fallback `1/0/0`; streamed tool-call deltas preserved with counters `1/0/0`.
- Security: missing/invalid key, unknown alias, unsupported path, oversized body and invalid Host all produced zero upstream requests.
- Process: singleton enforced; gateway, control and LiteLLM crashes restarted independently; port conflict reported with owning PID.
- Concurrency: deterministic Go test proved the 16-request limit returns 429 under 30 concurrent requests.
- Secret canary: no gateway or provider key found in database, YAML, state, logs or evidence.
- Final live check: `/ready` returned `{"status":"ready"}`; chat completion returned HTTP 200 through `nenifix-coder`.

## Open items requiring owner action

1. Rotate the three OpenRouter keys at some point, because they were pasted into a chat message. The stored copies are in Windows Credential Manager; the chat transcript is the exposure risk.
2. Publish/deploy only after specifying a target and giving explicit approval. Nothing was pushed, published or exposed beyond loopback.
3. Full v1.6.6 features (web dashboard, installers, spool/telemetry, discovery, Linux, backup UX) remain deferred by the MVP scope.

## Stop the local demo

```bash
cd C:/Users/ai9/Documents/opentrinity
set OT_USER_DATA=C:\Users\ai9\Documents\opentrinity\.demo-userdata
dist\opentrinity.exe stop
```
