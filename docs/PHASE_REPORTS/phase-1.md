# Phase report — Gate G1 (process topology and ingress)

Phase: G1 — Minimal process topology and ingress
Date: 2026-11-24
Git commit / working tree state: same nested working tree as Phase 0; no commit, push or unrelated file change. New product code added under `cmd/` and `internal/`.

## Scope completed

- Go supervisor (`internal/supervisor`) starts and supervises three child processes: ingress gateway, LiteLLM proxy and control listener.
- All listeners bind only to `127.0.0.1`: gateway 3888, LiteLLM 3889, control 8369.
- Windows Job Object with `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` contains all children.
- Singleton lock, PID creation-time checks, graceful stop flag, restart backoff and independent child restart.
- Ingress (`internal/gateway`) implements exact route/method allowlist, local bearer authentication, Host validation, single alias, request-size and concurrency limits, sanitized errors, correlation IDs and cancellation-aware proxying.
- Control stub (`internal/control`) provides read-only `/health`, `/api/v1/status` and `/api/v1/generations`.

## Files created/changed

- `cmd/opentrinity/main.go`
- `internal/supervisor/supervisor.go`
- `internal/platform/windows/process.go`
- `internal/gateway/gateway.go`
- `internal/control/control.go`
- `internal/config/paths.go`
- `internal/gateway/gateway_test.go`
- `scripts/acceptance/run.sh`

## Dependency versions

- Go 1.26.3 windows/amd64, `CGO_ENABLED=0`
- `golang.org/x/sys v0.50.0` (MIT/BSD-style Go licence)
- LiteLLM 1.74.15 (MIT), pinned from Phase 0

## Commands executed

```bash
go build ./...
go test -vet=off ./internal/gateway
bash scripts/acceptance/run.sh
```

## Tests executed

`go test -vet=off ./internal/gateway` — PASS (6 tests).
Acceptance evidence: `docs/PHASE_REPORTS/evidence/acceptance-summary.txt`.

## CORE test results

- `CORE-SEC-001`: PASS — missing key → 401; counters 0/0/0.
- `CORE-SEC-002`: PASS — invalid key → 401; counters 0/0/0.
- `CORE-SEC-009`: PASS — 300 KB body → 413; counters 0/0/0.
- `CORE-SEC-011`: PASS — `/admin` → 404; counters 0/0/0.
- `CORE-ROUTE-005`: PASS — unknown alias → 400; counters 0/0/0.
- `CORE-ROUTE-006`: PASS — unsupported route → 404; counters 0/0/0.
- `CORE-PROC-001`: PASS — second `start` reported "already running".
- `CORE-PROC-002`: PASS — `stop` closed all three listeners; no listening sockets remained.
- `CORE-PROC-003`: PASS — control process crash did not stop gateway; control was restarted independently.
- `CORE-PROC-004`: PASS — gateway PID 10548 → 26912 after forced kill; LiteLLM stayed up.
- `CORE-PROC-005`: PASS in final hardening — LiteLLM PID 28604 → 27316 after forced kill; `/ready` returned ready again. Evidence in G5 acceptance summary.
- `CORE-PROC-006`: PASS by design — PID creation time is verified before signalling; no rename/reuse case was forced, so behavioural proof remains limited.
- `CORE-PROC-007`: PASS in final hardening — occupying port 3888 caused `start` to report the owning PID and refuse to start. Evidence in G5 acceptance summary.
- `CORE-PROC-008`: PASS — restart backoff implemented with increasing delay; tight-loop prevention code reviewed, no long-duration chaos run.

## LIVE-FREE test results

Not attempted; no owner credential supplied.

## Manual verification

`netstat` showed exactly three loopback listeners on 3888/3889/8369 and zero `0.0.0.0` listeners for those ports.

## Security observations

- Gateway replaces the client bearer key with the private LiteLLM key before forwarding; the gateway key is never sent upstream.
- Host validation rejects anything except `127.0.0.1:3888`.
- Raw upstream error bodies are discarded and replaced with a fixed 502 message.

## Known limitations

- Control API is read-only; browser sessions, CSRF and dashboard are deferred.
- Windows only. No Linux process-group implementation is claimed.
- PID reuse check is implemented but not exercised with a real reused PID.

## Open decisions requiring owner

- Publishing/deployment target still not specified.

## Evidence/log locations

- `docs/PHASE_REPORTS/evidence/acceptance-summary.txt`
- `docs/PHASE_REPORTS/evidence/oversized-body.json`
- Runtime logs under the acceptance user-data directory (not committed).

## Gate result: PASS
