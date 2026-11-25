# OPENTRINITY / NENIFIX — MVP Build Prompt

**Purpose:** Build a small, usable Windows-first MVP from the existing OPENTRINITY repository and v1.6.6 specification. This prompt intentionally narrows the first release to a working local OpenAI-compatible chat gateway with deterministic LiteLLM fallback, a CLI, and a small interactive terminal UI (TUI). It is not authorization to publish, deploy, or expose services beyond loopback.

**Repository:** `C:\Users\ai9\Documents\opentrinity` (or the working directory explicitly supplied by the owner)  
**Primary platform:** Windows 11, per-user, no administrator requirement  
**Language/runtime:** Go; `CGO_ENABLED=0`  
**Ingress:** `127.0.0.1:3888/v1`  
**Private LiteLLM:** `127.0.0.1:3889`  
**Control port:** `127.0.0.1:8369` (MVP may provide a minimal health/status stub; no dashboard)  
**Client model alias:** `nenifix-coder`

---

## 1. Agent execution contract

1. Start by inspecting the repository, its Git root, branch, working-tree state, tests, current code and existing documentation. The repository may be nested in another Git repository. Do not run Git initialization, change the Git root, commit, or modify unrelated tracked files without owner approval.
2. Preserve existing work. Reuse the existing `.phase0/harness/fakeupstream` and `.phase0/spikes/sqlite` assets if they are sound; inspect them before relying on them.
3. Work sequentially through gates G0–G5. A gate must pass with recorded evidence before the next gate starts. Debug ordinary implementation failures; do not paper over failures or claim unrun tests passed.
4. Do not implement product features before G0 proves the pinned LiteLLM routing behavior and records the exact version/configuration. Do not assume duplicate alias entries provide ordered fallback.
5. Use deterministic fake upstreams and actual per-slot request counters for routing proof. Live provider checks are optional and only use credentials supplied and authorized by the owner. If unavailable, mark live checks `SKIP — owner credential unavailable`.
6. Do not expose secrets in SQLite, generated YAML values, logs, events, diagnostics, HTML/JSON responses, URLs, command arguments, source control or test snapshots. Never invent or embed provider keys.
7. Keep all listeners loopback-only. Do not add Docker, Kubernetes, Redis, PostgreSQL, a second database, local model downloads, GPU requirements, Rust, public tunnels or LAN binding.
8. No publication, release, deployment or push without explicit owner approval.
9. At each gate, write a concise report under `docs/PHASE_REPORTS/` including scope, exact commands, dependency versions, tests and evidence, applicable test IDs, limitations, and gate result. Report every applicable test `PASS`, `FAIL` or `SKIP` with evidence/reason.
10. Stop only for a genuine architectural blocker, a required owner decision, or an unauthorized destructive/external action. Present the evidence and the smallest decision needed.

## 2. MVP product promise

A user can initialize the app, configure one or more of their own provider credentials and up to three deployment slots, start OPENTRINITY, and send an OpenAI Chat Completions request to:

```text
Base URL: http://127.0.0.1:3888/v1
API key:  local OPENTRINITY gateway key
Model:    nenifix-coder
```

LiteLLM Proxy is the **only** inference router. Go may authenticate, validate the exact route and alias, enforce limits, forward requests, stream bytes and propagate cancellation. Go must never pick a provider, reorder slots, retry, implement fallback, cooldown, or interpret an upstream failure to start a new attempt.

The MVP must support slots A, B and C in configured order, non-streaming completions, SSE streaming, basic tool-call pass-through when supported by the pinned LiteLLM/provider combination, safe local lifecycle management, a CLI for core configuration/operations, and a small SQLite-backed configuration store with generated LiteLLM configuration and rollback to the last known good generation.

Do not claim universal OpenAI compatibility or guaranteed free availability. A real provider test demonstrates availability only at the recorded time.

## 3. MVP scope and deferrals

### In scope

- Go supervisor, ingress gateway and minimal control/status stub, bound only to loopback.
- LiteLLM Proxy as a supervised child process, pinned to an exact tested version.
- Exact ingress method/path allowlist: `GET /health`, `GET /ready`, `GET /v1/models`, `POST /v1/chat/completions` only.
- Local bearer-key authentication; one public alias, `nenifix-coder`; unknown aliases/routes rejected before upstream forwarding.
- Request-size and concurrency limits, streaming pass-through, cancellation and sanitized errors.
- Up to three persisted deployment slots A/B/C, generated LiteLLM config, candidate validation, atomic apply and last-known-good rollback.
- Minimal SQLite schema for settings, providers, credential references, models, deployments and config generations; WAL, migrations, foreign keys and a single logical writer.
- CLI: `init`, `start`, `stop`, `restart`, `status`, `doctor`, provider/credential/deployment configuration, `apply validate|run|history|rollback`, and gateway-key rotation/reveal with explicit confirmation.
- A small interactive TUI built with the open-source Go framework **Bubble Tea** (Charmbracelet; MIT license), with exact framework and transitive dependency versions/licenses pinned and recorded. Use Bubbles and Lip Gloss only where useful; verify the chosen versions and licenses before adoption.
- TUI workflows limited to overview/status, guided provider and credential entry, deployment slot assignment/order, apply/rollback confirmation, and help/exit. It should call the same application services as the CLI (no duplicate business logic), support keyboard navigation and visible errors, and remain usable in a Windows terminal. Secret entry must be masked and must use the same approved secure storage path as the CLI; never display/reveal a stored key in the TUI.
- Keep all CLI commands available for scripts and non-interactive use; provide an explicit command such as `opentrinity tui` to launch the interactive interface. Detect non-interactive terminals and fail with a helpful message rather than hanging.
- Fake-upstream tests with observable A/B/C counters.
- Windows child-process containment (Job Object) or a documented, tested MVP containment strategy that prevents child processes outliving the supervisor.
- README for setup, client connection, free-tier caveats, privacy/fallback disclosure and known limitations.

### Explicitly deferred

- Templ/HTMX web dashboard and browser sessions/CSRF flows (the small Bubble Tea terminal UI is in scope).
- Provider/model discovery and pricing verification UI.
- Rich provider credential state machine and credential testing workflows.
- Event database, JSONL spool/ingestion, analytics, retention and diagnostics export.
- Linux support, installers, updater, uninstaller, client config writers, MCP and autonomous agent features.
- Backup/restore UX, dynamic free-model discovery, cooldown and same-slot retries.

Do not accidentally expand scope. Keep architecture compatible with the larger v1.6.6 specification where that does not materially increase MVP work or contradict this prompt.

## 4. Decisions that require owner input

Do not silently decide these. At the first point they block implementation, present the options and ask the owner:

1. **Provider secret storage on Windows:** preferred design is Windows Credential Manager. If that cannot be completed within the MVP, ask whether the owner accepts a clearly warned, per-user ACL-protected secret-file fallback. Never silently downgrade to plaintext or claim an ACL is encryption.
2. **Live provider acceptance:** ask whether the owner wants to supply a provider key for optional live testing. Never request or place a real key in chat, source, command arguments or logs; provide a safe local credential entry path.
3. **Number of configured slots:** default plan is to implement A/B/C and prove all three. Ask only if the owner explicitly wants a smaller MVP.

If the owner has not supplied a live key, complete all fake-upstream acceptance work and report live tests as skipped; do not block deterministic MVP work.

## 5. Gate plan

### G0 — Feasibility and LiteLLM proof (no product feature code)

Inspect the repository/toolchain and finish the Phase 0 work needed for the MVP:

- Record Windows version, Go version, `CGO_ENABLED`, Python/runtime availability, and exact selected LiteLLM version and license.
- Resolve why the existing `.phase0/venv` lacks LiteLLM before attempting installation. Use the available supported Python/uv environment; pin the resulting dependency. Do not install an unpinned latest dependency as the final result.
- Inspect and run the existing fake upstream. Ensure it provides independent A/B/C counters and deterministic failure/success behaviors.
- Construct a minimal LiteLLM configuration and prove A→B→C sequential fallback with fake upstream counters. Verify that client errors do not fallback and same-slot retries are zero.
- Prove streaming behavior around the commit point: eligible failure before the first response byte may fallback if LiteLLM supports it; after the first byte there must be no fallback. If exact semantics cannot be achieved with the pinned LiteLLM configuration, stop and report the smallest design decision.
- Verify minimal tool-call pass-through, telemetry/database behavior, and the exact provider model syntax needed for the chosen test configuration. Disable optional/unproven retries, cooldown, telemetry and LiteLLM database features.
- Record exact working configuration using placeholder/environment-secret references only, actual command output, fake-upstream counters, limitations and go/no-go.

**G0 gate:** no product feature code until pinned LiteLLM A/B/C behavior is evidenced. No guessed fallback configuration.

### G1 — Minimal process topology and ingress

- Implement or complete the Go supervisor and gateway using the existing repository structure where practical.
- Start the private LiteLLM process on `127.0.0.1:3889`; expose ingress only at `127.0.0.1:3888`; provide a minimal control/status listener on `127.0.0.1:8369` only if needed by the MVP.
- Implement singleton protection, startup/readiness checks, graceful shutdown and child containment appropriate to Windows.
- Implement ingress authentication, exact path/method allowlist, alias allowlist, request limits, sanitized errors, correlation IDs, pass-through and cancellation.
- Never proxy to arbitrary paths or expose LiteLLM admin endpoints.

**Gate tests:** missing/invalid local key, unknown alias, unsupported path/method and oversized body all cause zero fake-upstream requests; listeners bind only to loopback; health and readiness semantics are correct.

### G2 — Streaming and inference contract

- Implement non-buffering SSE pass-through with prompt flushing and client-disconnect cancellation.
- Define commit point as first response-body byte successfully written/flushed to the client. After commit, never retry/fallback or append a JSON error; terminate and record sanitized status.
- Pass supported tool-call request/response fields without inspection or logging of tool arguments.

**Gate tests:** non-stream completion; stream headers/chunk ordering; stream not fully buffered; pre-commit fallback; post-commit no fallback; disconnect cancels upstream; non-stream and streamed tool calls where pinned dependencies support them. Unsupported dependency behavior must be recorded, not fabricated.

### G3 — Minimal config store and safe apply

- Select and pin a CGO-free Go SQLite driver only after verifying Windows behavior. Record exact version and license.
- Implement migrations, WAL, foreign keys, busy timeout, bounded transactions and a single logical writer.
- Persist the minimal provider/model/credential-reference/deployment/routing settings and immutable config generations. Do not persist secret values.
- Generate LiteLLM config from validated state. Use environment-variable references; keep values out of YAML.
- Validate candidate configuration, write to a private temporary location, atomically activate only after health/smoke checks, preserve last-known-good, and rollback automatically on failure.
- Implement secure gateway-key generation/storage/rotation according to the owner-approved storage decision. Show/reveal only once with explicit local confirmation.

**Gate tests:** fresh migration and integrity; invalid configuration cannot replace active; failed health check restores last-known-good; crash/restart uses last-known-good; secret-canary scan finds no secret on forbidden surfaces.

### G4 — Core CLI, small TUI and first-run path

Implement a small, documented CLI for:

```text
opentrinity init
opentrinity start|stop|restart|status|doctor
opentrinity providers list|add
opentrinity credentials add|rotate|disable
opentrinity deployments list|set|enable|disable
opentrinity apply validate|run|history|rollback
opentrinity gateway key rotate|show
```

Implement `opentrinity tui` using Bubble Tea as a thin interactive shell over the same application services used by the CLI. Keep the TUI deliberately small: status/overview, guided provider and credential setup, A/B/C deployment assignment, apply/rollback confirmation, and help/quit. Use a masked input component for secrets and never echo a key, even after submission. The TUI must honor the approved credential-storage decision, validate input, show actionable errors, handle cancellation safely, and warn before actions that may contact providers or change active configuration. Pin and record Bubble Tea and any other adopted TUI dependencies with their licenses. Test keyboard navigation and workflows on a real supported Windows terminal, as well as non-interactive detection.

Adjust command names to existing CLI conventions where appropriate and document omissions. Never accept secrets as command-line arguments. Use protected/local input and never echo a secret after entry. Destructive actions require confirmation. `status` and basic diagnostics must remain useful if the control process is unavailable. The CLI remains complete for automation; the TUI is an optional guided interface, not a replacement for CLI commands.

**Gate test:** clean user-data directory → initialize → configure fake provider credentials/references through CLI and TUI → apply → start → successful chat completion through `nenifix-coder`; stop/restart works safely. Verify TUI secret masking, no secret in terminal output/logs, keyboard operation, cancellation, and non-interactive failure behavior.

### G5 — End-to-end acceptance and MVP handoff

- Run the full deterministic fake-upstream suite, including A-only, A→B, A→B→C, all fail, invalid alias, client error, timeout/deadline, streaming commit boundary, cancellation and tools.
- Verify actual A/B/C request counters for every routing test and confirm no Go inference routing/retry logic exists (source review/test).
- Run a secret-canary scan across logs, database, generated YAML, diagnostics, process arguments and test artifacts.
- Run `go test ./...`, build for Windows/amd64 with CGO disabled, and document any skipped platform test accurately.
- If and only if the owner supplies a credential through the local secure path and requests it, run an optional live provider test. Record date, provider/model, sanitized outcome and latency. Otherwise mark `LIVE-FREE-*` skipped with reason.
- Write MVP README and a final handoff report listing exact commands, versions, evidence, test statuses, limitations, deferred work and any owner decisions still needed.

**MVP acceptance:** G0–G5 pass; fake routing counters prove the fallback chain; API, CLI and the small Bubble Tea TUI work on Windows; TUI secret entry is masked; security scans pass; no unresolved critical secret-handling or process-lifecycle issue. Do not claim the broader v1.6.6 release definition of done, Linux readiness, web-dashboard completeness, live free-tier availability or production hardening unless separately verified.

## 6. Test identifiers and report rules

Use existing test IDs where applicable. At minimum report:

- `CORE-ROUTE-001…009`, with actual A/B/C counters and evidence.
- `CORE-STREAM-001…006` and `CORE-TOOL-001…004`, marking dependency-limited behavior explicitly.
- `CORE-SEC-001…004`, plus applicable body/route/loopback checks.
- `CORE-DATA-001…004` and `CORE-APPLY-001…004` to the extent implemented in MVP.
- Relevant `CORE-PROC-*` tests for singleton, shutdown containment, independent process failure behavior and restart safety.
- `LIVE-FREE-*` only if explicitly attempted; otherwise `SKIP` with reason.

A phase/gate passes only if every required test for that gate passes. Skips require a reason and cannot be counted as proof.

## 7. Non-negotiable constraints

- LiteLLM is the sole inference router. No Go routing, provider choice, fallback, retry, cooldown, load balancing or circuit breaker.
- One public alias: `nenifix-coder`.
- No real secrets in source, database, YAML values, logs, events, diagnostics, HTML/JSON, URLs, argv, test snapshots or reports.
- No arbitrary reverse proxy behavior; exact ingress allowlist only.
- No public/LAN binding or remote access feature.
- No silent model/provider substitution or automatic changes from discovery.
- No guaranteed free quota, permanent free model list, provider uptime or zero-invoice claim.
- Never log or persist prompts, completions, tool arguments/results, auth headers or raw upstream error bodies.
- Never claim a test, provider capability, compatibility or security property without evidence.
- No push, release or publication without explicit owner approval.

---

**End of MVP Build Prompt**
