# OPENTRINITY — Tech Stack & What Has Been Built

Generated from project state (phases 0–5 complete). Source of truth: `README.md`, `BUILD_PROMPT.md`, `SPEC.md`, `docs/PHASE_REPORTS/`.

## Product

OPENTRINITY (NENIFIX) — a Windows-first, local-only, OpenAI-compatible gateway for integrating AI features into company products. Go handles authentication, limits, process lifecycle, configuration and the CLI. **LiteLLM is the only inference router** — it alone maps the public alias `nenifix-coder` to up to three provider slots (A/B/C) and performs ordered A→B→C fallback.

- Base URL: `http://127.0.0.1:3888/v1`
- API key: local OPENTRINITY gateway key (Windows Credential Manager)
- Model alias: `nenifix-coder`
- Listeners: gateway 3888, LiteLLM 3889, control 8369 (all `127.0.0.1` only)

## Tech stack

### Language / runtime
- **Go 1.26.3** (windows/amd64), `CGO_ENABLED=0` — single binary, no runtime toolchain needed.
- **Python 3.11.9** (CPython) for LiteLLM only, pinned to `C:\Users\ai9\AppData\Local\Programs\Python\Python311\python.exe`.
- **LiteLLM 1.74.15** (MIT) — sole inference router.

### Storage / secrets
- **SQLite** via `modernc.org/sqlite v1.62.0` (CGO-free) — CGO-free driver, WAL, foreign keys, busy timeout, integrity check, single logical writer (`SetMaxOpenConns(1)` + write mutex).
- **Windows Credential Manager** via `github.com/danieljoos/wincred v1.2.3` (MIT) — all secret values (provider keys, gateway key, LiteLLM key).
- Migrations: settings, providers, credentials (references only), models, deployments, config generations, apply runs.

### Process / OS
- **Windows Job Object** (`JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`) via `golang.org/x/sys v0.50.0` (MIT/BSD-style) — contains all child processes; singleton lock, PID creation-time checks, graceful stop, restart backoff, independent child restart.
- Three supervised children: ingress gateway, LiteLLM proxy, control listener.

### Terminal UI
- **Bubble Tea v1.3.10** (MIT) — small interactive TUI.
- **Lip Gloss v1.1.0**, Charmbracelet x/ansi, x/cellbuf, colorprofile, termenv (transitive).
- **golang.org/x/term v0.48.0** — masked terminal input.

### Networking / proxy
- `net/http`, `httputil.ReverseProxy` with `FlushInterval: -1` for non-buffering SSE pass-through.
- `commitWriter` tracks first-byte commit point; post-commit errors terminate the client connection (no retry/fallback after commit).
- Context propagation for client disconnect/deadlines.

### Other Go modules
- `github.com/google/uuid v1.6.0`
- `github.com/dustin/go-humanize`, `golang.org/x/text`, etc. (see `go.mod`).

## What has been built (phases 0–5, all gates PASS)

### Phase 0 — Feasibility (PASS)
- Recorded versions (Go, Windows, LiteLLM, SQLite driver, Python).
- CGO-free SQLite spike.
- Proved ordered A→B→C fallback with pinned LiteLLM config and real per-slot counters.
- Proved zero same-slot retries, no fallback on client errors, invalid alias rejection, upstream error sanitization requirement.
- Proved pre-commit fallback, post-commit no-fallback, SSE pass-through, non-stream tool-call pass-through.

### Phase 1 — Process topology & ingress (PASS)
- Supervisor starts/supervises gateway (3888), LiteLLM (3889), control (8369), all loopback-only.
- Windows Job Object containment, singleton lock, restart backoff.
- Ingress: exact route/method allowlist, local bearer auth, Host validation, single alias, request-size & concurrency limits, sanitized errors, correlation IDs, cancellation-aware proxying.
- Control stub: read-only `/health`, `/api/v1/status`, `/api/v1/generations`.

### Phase 2 — Streaming & tools pass-through (PASS)
- Non-buffering SSE via ReverseProxy (FlushInterval -1).
- Commit point = first response byte; commitWriter tracks it; no fallback after commit.
- Client disconnect/deadlines propagate through context.
- Tool-call request/response fields pass through uninspected/unlogged.
- All routing/fallback/retry owned by LiteLLM; Go has no provider-selection logic.

### Phase 3 — SQLite store & safe apply (PASS)
- CGO-free SQLite store (WAL, FK, busy timeout, integrity, single writer).
- Migrations for settings/providers/credentials/models/deployments/generations/apply-runs.
- LiteLLM YAML generated from validated state using `os.environ/...` secret references; only loopback HTTP or public HTTPS API bases accepted; no network fetch during validation.
- Atomic candidate write, activation through supervisor, last-known-good copy, auto-restore on health-check failure.
- Gateway & LiteLLM keys generated with crypto/rand, stored only in Credential Manager.

### Phase 4 — CLI, TUI & first-run (PASS)
- CLI commands: `init`, `start`, `stop`, `restart`, `status`, `doctor`, `providers`, `models`, `credentials`, `deployments`, `apply`, `gateway key`, `tui`.
- Masked secret entry via terminal; `credentials stdin` for automation (never argv).
- Gateway key reveal requires typing `SHOW`; rotate never displays value.
- Small Bubble Tea TUI: status, provider add, masked credential entry, deployment slot assignment, apply/rollback confirmation, help, quit. Calls same services as CLI.
- Non-interactive terminal detection returns helpful error.

### Phase 5 — End-to-end acceptance (PASS)
- Deterministic fake-upstream matrix through real Go gateway + supervisor + LiteLLM 1.74.15.
- Verified A→B→C fallback with real per-slot counters, client-error rejection, sanitized aggregate failures, streaming commit boundary, deadline cancellation, tool pass-through, process isolation/restart, loopback-only binding.
- Secret canary scans across DB/YAML/state/logs/evidence — PASS.
- Built Windows/amd64 binary `dist/opentrinity.exe` with `CGO_ENABLED=0`.
- `go test -vet=off ./...` PASS (apply, database, gateway, routingconfig, tui).
- `bash scripts/acceptance/run.sh` → GATE RESULT: PASS.

## Ingress allowlist
- `GET /health`
- `GET /ready`
- `GET /v1/models`
- `POST /v1/chat/completions`
- `POST /v1/messages` (Anthropic-compatible, for Claude Code)

Everything else rejected before LiteLLM. `/v1/responses` forwarded for Codex CLI.

## Packages (internal/)
- `apply` — candidate validation, atomic apply, rollback, last-known-good.
- `auth` — bearer auth.
- `backup` — backup helpers.
- `config` — paths, state.
- `control` — read-only control API.
- `database` — SQLite store + migrations.
- `deployments`, `providers`, `routingconfig`, `secrets` — domain logic + secret refs.
- `diagnostics`, `events`, `gateway`, `platform/windows`, `supervisor`, `telemetry`, `spool`, `tui`.

## Key test/acceptance evidence
- `docs/PHASE_REPORTS/evidence/acceptance-summary.txt`
- `docs/PHASE_REPORTS/evidence/stream-precommit.txt`, `stream-postcommit.txt`
- `docs/PHASE_REPORTS/evidence/tool-response.json`
- CORE test IDs: CORE-ROUTE-001..013, CORE-STREAM-001..006, CORE-TOOL-001..004, CORE-SEC-001..010, CORE-DATA, CORE-APPLY, CORE-PROC — mostly PASS, some SKIP (documented).

## Known limitations (MVP)
- Windows only; Linux deferred.
- No web dashboard, browser sessions, CSRF, event spool, analytics, backup UX, installer, updater, uninstaller.
- No live provider test performed; no free-tier availability/price/quota claims.
- Cooldown, same-slot retries, Go-side provider classification — disabled/absent by design.
- Role-only SSE commit point unobservable with pinned LiteLLM/harness (reported SKIP).
- No publish/push/deploy performed.

## Release status
No publish, push, deployment or public exposure. External release requires explicit owner approval.

## Build
```bash
CGO_ENABLED=0 go build -o dist/opentrinity.exe ./cmd/opentrinity
```
Binary: `dist/opentrinity.exe` (Windows/amd64).

## Saved prompt reference
Full Bubble Tea TUI dashboard spec saved at `docs/OPENTRINITY_BUBBLE_TEA_TUI_PROMPT.md` (from Notion). The current TUI is the MVP "small TUI"; the full 20-phase dashboard (F1–F10 screens, SSE, credential manager, models/deployments/routing/activity) is a future effort.
