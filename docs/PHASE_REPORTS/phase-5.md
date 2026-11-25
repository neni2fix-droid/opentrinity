# Phase report — Gate G5 (end-to-end acceptance and MVP handoff)

Phase: G5 — End-to-end acceptance
Date: 2026-11-24

## Scope completed

- Executed the deterministic fake-upstream matrix through the real Go gateway, real supervisor and real LiteLLM 1.74.15.
- Verified ordered A→B→C fallback with actual per-slot counters, client-error rejection, aggregate failure sanitization, streaming commit boundary, deadline cancellation, tool-call pass-through, process isolation/restart and loopback-only binding.
- Ran secret canary scans across database, generated YAML, state, logs and evidence.
- Built Windows/amd64 binary with `CGO_ENABLED=0`.
- Ran the complete Go test suite.

## Files created/changed

- `scripts/acceptance/run.sh`
- `docs/PHASE_REPORTS/evidence/acceptance-summary.txt`
- `docs/PHASE_REPORTS/evidence/stream-precommit.txt`
- `docs/PHASE_REPORTS/evidence/stream-postcommit.txt`
- `docs/PHASE_REPORTS/evidence/tool-response.json`
- `docs/PHASE_REPORTS/phase-1.md` … `phase-5.md`
- `README.md`

## Dependency versions

| Component | Version | Licence |
|---|---|---|
| Go | 1.26.3 windows/amd64, CGO disabled | Go BSD-style |
| LiteLLM | 1.74.15 | MIT |
| SQLite driver | modernc.org/sqlite v1.62.0 | SQLite public-domain style |
| TUI | bubbletea v1.3.10 | MIT |
| Windows credentials | wincred v1.2.3 | MIT |
| Terminal input | x/term v0.48.0 | Go BSD-style |
| Python for LiteLLM | CPython 3.11.9 | PSF |

## Commands executed

```bash
CGO_ENABLED=0 go build -o dist/opentrinity.exe ./cmd/opentrinity
go test -vet=off ./...
bash scripts/acceptance/run.sh
printf 'SHOW\n' | dist/opentrinity.exe gateway key show
```

`go test` uses `-vet=off` because vetting the large CGO-free SQLite dependency exceeded the available time on this 8 GB machine; compilation and tests themselves completed successfully.

## Tests executed

- `go test -vet=off ./...` — PASS (apply, database, gateway, routingconfig, tui).
- `bash scripts/acceptance/run.sh` — GATE RESULT: PASS.

## CORE test results (ID + PASS/FAIL/SKIP + evidence)

- `CORE-ROUTE-001`: PASS — A=1 B=0 C=0.
- `CORE-ROUTE-002`: PASS — A=1 B=1 C=0.
- `CORE-ROUTE-003`: PASS — A=1 B=1 C=1.
- `CORE-ROUTE-004`: PASS — A=1 B=1 C=1, HTTP 502 sanitized by Go gateway.
- `CORE-ROUTE-005`: PASS — unknown alias, 0/0/0.
- `CORE-ROUTE-006`: PASS — unsupported route, 0/0/0.
- `CORE-ROUTE-007`: PASS — malformed client JSON, 0/0/0.
- `CORE-ROUTE-008`: PASS — client deadline/cancel, 1/0/0.
- `CORE-ROUTE-009`: PASS — zero same-slot retries.
- `CORE-ROUTE-010`…`013`: SKIP — credential state machine, quota classification and cooldown are deferred; no cooldown is enabled or claimed.
- `CORE-STREAM-001`: PASS — A=1 B=1 C=0.
- `CORE-STREAM-002`: SKIP — role-only commit not separately observable with pinned harness.
- `CORE-STREAM-003`: PASS — A=1 B=0 C=0.
- `CORE-STREAM-004`: PASS — cancellation path evidenced by deadline test, 1/0/0.
- `CORE-STREAM-005`: PASS — incremental SSE chunks.
- `CORE-STREAM-006`: PASS — chunk order and `[DONE]` preserved.
- `CORE-TOOL-001`: PASS — tool call and arguments passed through.
- `CORE-TOOL-002`: PASS — extended fake upstream emitted streamed tool-call deltas; gateway preserved `tool_calls`, arguments and `[DONE]`; counters A=1 B=0 C=0.
- `CORE-TOOL-003`: PASS — no tool data in logs/DB/YAML; canary scan passed.
- `CORE-TOOL-004`: SKIP — not executed.
- `CORE-SEC-001`: PASS — missing key, 0/0/0.
- `CORE-SEC-002`: PASS — invalid key, 0/0/0.
- `CORE-SEC-003`: PASS — gateway key absent from forbidden surfaces.
- `CORE-SEC-004`: PASS — provider keys absent from forbidden surfaces.
- `CORE-SEC-005`…`008`: SKIP — browser sessions/CSRF/discovery endpoints deferred; no browser surface exists in MVP.
- `CORE-SEC-009`: PASS — oversized body, 0/0/0.
- `CORE-SEC-010`: PASS — deterministic Go test sent 30 concurrent valid requests; some returned 429 while others succeeded, proving the 16-request semaphore.
- `CORE-SEC-011`: PASS — admin path rejected.
- `CORE-SEC-012`: SKIP — no redirect feature exists.
- `CORE-SEC-013`: SKIP — diagnostics export deferred.
- `CORE-DATA-001`: PASS.
- `CORE-DATA-002`: SKIP — checksum mismatch not implemented.
- `CORE-DATA-003`: PASS (limited) — WAL and backup spike passed; no forced crash recovery.
- `CORE-DATA-004`: PASS.
- `CORE-DATA-005`…`010`: SKIP — spool/retention/backup UX deferred.
- `CORE-APPLY-001`: PASS.
- `CORE-APPLY-002`: PASS.
- `CORE-APPLY-003`: PASS.
- `CORE-APPLY-004`: PASS (limited) — rollback verified; crash injection not performed.
- `CORE-PROC-001`: PASS.
- `CORE-PROC-002`: PASS.
- `CORE-PROC-003`: PASS.
- `CORE-PROC-004`: PASS — PID 10548 → 26912.
- `CORE-PROC-005`: PASS — forced LiteLLM kill; supervisor restarted it, PID 28604 → 27316, and `/ready` recovered.
- `CORE-PROC-006`: SKIP (behavioural) — PID reuse guard implemented but not exercised with a real reused PID.
- `CORE-PROC-007`: PASS — occupying port 3888 caused `start` to report the owning PID and refuse to start.
- `CORE-PROC-008`: PASS by implementation — backoff present.
- `CORE-OPS-*`: SKIP — installers/updater/uninstall/Linux are explicitly deferred.
- `CORE-FREE-001`…`008`: PARTIAL — fake provider configuration and fake free-style model assignment are proven; UI status/discovery/free-tier labelling is deferred, so no free-tier availability claim is made.
- `LIVE-FREE-*`: SKIP — no owner credential supplied; no live provider availability claim.

## LIVE-FREE test results

Not attempted. Reason: no owner-supplied credential and no live test authorization. This is not a failure of deterministic MVP acceptance.

## Manual verification

- Client completion through `nenifix-coder` returned HTTP 200 with fake upstream content.
- `gateway key show` required explicit `SHOW` confirmation.
- Three loopback listeners and zero public listeners observed.

## Security observations

- No secret value was found in SQLite, generated YAML, state JSON, runtime logs or evidence files.
- Raw upstream error text was replaced by a fixed sanitized 502 before reaching the client.
- LiteLLM admin routes are not reachable through the ingress allowlist.

## Known limitations

- Windows-only MVP. No Linux, Docker, dashboard, installer, updater, spool/telemetry or browser session.
- No live provider test; free-tier availability and pricing are not claimed.
- Role-only commit point and PID-reuse with a genuinely recycled PID remain unverified and are explicitly skipped above. Streamed tool-call deltas, forced LiteLLM crash recovery and port-conflict reporting are now verified.
- LiteLLM is the sole inference router; cooldown and same-slot retries are disabled.

## Open decisions requiring owner

- Supply a real provider credential through the local secure input path if a live integration test is desired.
- Specify a publication/deployment target if external release is desired. No publish, push or public exposure was performed.

## Evidence/log locations

- `docs/PHASE_REPORTS/evidence/acceptance-summary.txt`
- `docs/PHASE_REPORTS/evidence/stream-precommit.txt`
- `docs/PHASE_REPORTS/evidence/stream-postcommit.txt`
- `docs/PHASE_REPORTS/evidence/tool-response.json`
- `docs/PHASE_REPORTS/evidence/stream-tool-response.txt`
- `dist/opentrinity.exe`

## Gate result: PASS

MVP acceptance is met for the narrowed G0–G5 scope. This is not a claim of the full v1.6.6 release definition of done.
