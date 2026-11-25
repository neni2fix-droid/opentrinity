# Phase report — Phase 0 / Gate G0 (MVP feasibility)

Phase: Phase 0 — Feasibility spikes, no product feature code
Date: 2026-11-24
Git commit / working tree state: branch `master` tracking `origin/main`; HEAD `88029e2`. The repository is nested under a parent Git root. Pre-existing unrelated changes under `Desktop/neni9/sir9tui` were left untouched. No commit, push or Git reconfiguration was performed.

## Scope completed

- Recorded Windows, Go, Python and LiteLLM versions.
- Reused the existing deterministic fake-upstream harness with three independent loopback slots (10001/10002/10003) and real per-slot counters.
- Reused and executed the CGO-free SQLite spike.
- Proved ordered A→B→C fallback with a pinned LiteLLM configuration and real counters.
- Proved zero same-slot retries, no fallback on client errors, invalid alias rejection, and sanitization requirement for aggregate upstream errors.
- Proved pre-commit fallback, post-commit no-fallback, SSE pass-through and non-stream tool-call pass-through.
- Established that LiteLLM returns raw upstream error messages in its aggregate error body; the Go ingress must sanitize all upstream errors before returning them to a client.

## Files created/changed

- `.phase0/config/litellm.yaml` — working placeholder-only proof configuration.
- `docs/PHASE_REPORTS/phase-0.md` — this report.
- No product feature code was written before this gate.

## Dependency versions

| Component | Exact version | Evidence / licence |
|---|---|---|
| Go | `go1.26.3 windows/amd64`, `CGO_ENABLED=0` | `go version`, `go env` |
| Windows | Microsoft Windows 11 Pro build 26100 | WMI `Win32_OperatingSystem` |
| SQLite driver | `modernc.org/sqlite v1.62.0` | `.phase0/spikes/sqlite/go.mod`; MIT licence (per module metadata) |
| LiteLLM | `1.74.15` | `importlib.metadata.distribution('litellm').version` |
| LiteLLM licence | MIT | package metadata `License: MIT` |
| Python for LiteLLM | CPython 3.11.9 at `C:\Users\ai9\AppData\Local\Programs\Python\Python311\python.exe` | `.phase0/venv2/Scripts/python.exe -c "import sys;print(sys.version)"` |

## Working LiteLLM configuration (placeholder secrets only)

```yaml
model_list:
  - model_name: nenifix-coder
    litellm_params:
      model: openai/fake-model
      api_base: http://127.0.0.1:10001/v1
      api_key: os.environ/OT_FAKE_A_KEY
  - model_name: nenifix-coder-b
    litellm_params:
      model: openai/fake-model
      api_base: http://127.0.0.1:10002/v1
      api_key: os.environ/OT_FAKE_B_KEY
  - model_name: nenifix-coder-c
    litellm_params:
      model: openai/fake-model
      api_base: http://127.0.0.1:10003/v1
      api_key: os.environ/OT_FAKE_C_KEY

router_settings:
  num_retries: 0
  allowed_fails: 0
  disable_cooldowns: true
  cooldown_time: 0
  timeout: 8
  fallbacks:
    - nenifix-coder: [nenifix-coder-b]
    - nenifix-coder-b: [nenifix-coder-c]

litellm_settings:
  drop_params: true
  telemetry: false
  success_callback: []
  failure_callback: []
  set_verbose: false

general_settings:
  master_key: os.environ/OT_LITELLM_KEY
```

## Commands executed

```bash
# toolchain and driver
go version && go env CGO_ENABLED GOOS GOARCH
cd .phase0/spikes/sqlite && CGO_ENABLED=0 go run main.go

# harness and proxy
./.phase0/harness/fakeupstream/fakeupstream.exe -ports 10001,10002,10003
OT_FAKE_A_KEY=... OT_FAKE_B_KEY=... OT_FAKE_C_KEY=... OT_LITELLM_KEY=... \
  ./.phase0/venv2/Scripts/litellm.exe --config .phase0/config/litellm.yaml --port 3889 --host 127.0.0.1

# each routing case reset all counters, set slot behaviours via POST /_config,
# sent POST /v1/chat/completions with model nenifix-coder, then read GET /_stats
```

## Tests executed and actual evidence

SQLite spike output included:

```text
goos/goarch: windows/amd64
cgo enabled at build: false
PASS open / PASS ping
journal_mode = wal
foreign_keys = 1
PASS foreign key enforcement rejects orphan: constraint failed: FOREIGN KEY constraint failed (787)
PASS VACUUM INTO backup
RESULT: PASS CGO-free SQLite works on windows
```

Routing counters (port → count) from the fake upstream:

| Case | A / 10001 | B / 10002 | C / 10003 | Result |
|---|---:|---:|---:|---|
| A success | 1 | 0 | 0 | HTTP 200 |
| A=500, B success | 1 | 1 | 0 | HTTP 200 |
| A=500, B=500, C success | 1 | 1 | 1 | HTTP 200 |
| A=B=C=500 | 1 | 1 | 1 | HTTP 500, aggregate error |
| Invalid alias | 0 | 0 | 0 | HTTP 400 |
| Malformed client JSON | 0 | 0 | 0 | HTTP 400 |

Streaming evidence:

| Case | Counters | Observed result |
|---|---|---|
| `stream=true`, A success | A=1 B=0 C=0 | SSE chunks then `data: [DONE]` |
| A=500 before body, B success | A=1 B=1 C=0 | fallback happened before first client byte |
| A `sse_fail_after_content`, B healthy | A=1 B=0 C=0 | no fallback after first content byte; stream ended cleanly |
| A `sse_fail_after_role`, B healthy | A=1 B=0 C=0 | LiteLLM emitted a single synthetic terminal chunk; role-only commit point not separately observable with this pinned version/harness |

Tool evidence: non-stream request with a `get_weather` tool returned `finish_reason=tool_calls` with `arguments={"city":"Accra"}` intact; counters A=1 B=0 C=0.

## CORE test results (ID + PASS/FAIL/SKIP + evidence)

- `CORE-ROUTE-001`: PASS — A=1 B=0 C=0.
- `CORE-ROUTE-002`: PASS — A=1 B=1 C=0.
- `CORE-ROUTE-003`: PASS — A=1 B=1 C=1.
- `CORE-ROUTE-004`: PASS — A=1 B=1 C=1, HTTP 500 aggregate error. Raw upstream text was present in LiteLLM's body; Go ingress sanitization is therefore mandatory.
- `CORE-ROUTE-005`: PASS — invalid alias produced zero upstream requests.
- `CORE-ROUTE-006`: PASS — non-existent route produced HTTP 404 with zero upstream requests.
- `CORE-ROUTE-007`: PASS — malformed client JSON yielded HTTP 400 with zero upstream requests.
- `CORE-ROUTE-008`: SKIP in G0 — end-to-end deadline enforcement is implemented and tested at G1/G5. LiteLLM `timeout: 8` is configured.
- `CORE-ROUTE-009`: PASS — same-slot retry count was zero in every case (`num_retries: 0`, observed A=1 for a failing A).
- `CORE-STREAM-001`: PASS — A=1 B=1 C=0 for pre-first-byte 500.
- `CORE-STREAM-002`: SKIP — role-only commit is not separately observable: LiteLLM 1.74.15 collapsed the role-only upstream chunk into a single synthetic chunk with this harness. No post-byte fallback was observed.
- `CORE-STREAM-003`: PASS — content chunk then abort: A=1 B=0 C=0, no fallback.
- `CORE-STREAM-004`: SKIP in G0 — client disconnect cancellation is tested at G2/G5.
- `CORE-STREAM-005`: PASS — response arrived as incremental SSE chunks; no whole-response buffering observed.
- `CORE-STREAM-006`: PASS — `data:` chunks followed by `data: [DONE]` in order.
- `CORE-TOOL-001`: PASS — non-stream tool call arguments passed through unchanged.
- `CORE-TOOL-002`: SKIP — pinned harness did not emit streamed tool-call deltas; acceptance harness will add this case.
- `CORE-TOOL-003`: PASS in principle at G0 — config has no tool data; enforced by telemetry design at G2/G5.
- `CORE-TOOL-004`: SKIP — no tool-call pre-commit fallback case executed in G0.
- `CORE-SEC-003`: PASS — proof config contains only `os.environ/...` references and placeholder environment values; no real secret used.
- `CORE-SEC-004`: PASS — same as above; no provider key was embedded in the config or report.
- `CORE-DATA-001`: PASS — CGO-free SQLite opened in WAL mode with foreign keys enforced and backup succeeded.

## LIVE-FREE test results (if attempted)

Not attempted. No owner-supplied provider credential was available. Deterministic fake upstreams were used instead.

## Manual verification

Fake-upstream counters were read after every request through `GET /_stats`; the table above is copied from actual command output. The SQLite spike was executed directly, not simulated.

## Security observations

- LiteLLM's aggregate error body contained the raw fake upstream message. The Go gateway must replace all upstream error bodies with a fixed sanitized message and must never log upstream bodies.
- LiteLLM admin routes must not be reachable through the ingress allowlist.
- All secrets are supplied as environment variables to the child process; no secret appears in the YAML, source, argv or this report.

## Known limitations

- No Linux host was available; Linux-specific spikes are not claimed.
- Role-only streaming commit point and streamed tool-call deltas could not be observed with the current harness/pinned LiteLLM combination; they are explicitly skipped, not claimed.
- LiteLLM 1.74.15 has no observable same-slot retry with `num_retries: 0` in the tested cases. A later G5 run showed that `allowed_fails: 0` alone still triggers cooldown between test cases; `disable_cooldowns: true` and `cooldown_time: 0` were added and verified in G5. No cooldown behaviour is claimed as enabled.
- Live provider availability was not tested.

## Open decisions requiring owner

- Windows Credential Manager is the selected secret store; no fallback is silently assumed.
- Publishing/deployment target was not specified. Local launch is in scope; publishing remains approval-gated.

## Evidence/log locations

- `.phase0/config/litellm.yaml`
- Fake upstream source: `.phase0/harness/fakeupstream/main.go`
- SQLite spike source/result: `.phase0/spikes/sqlite/main.go`
- This report and subsequent gate reports under `docs/PHASE_REPORTS/`.

## Gate result: PASS

G0 is passed for the MVP scope: exact LiteLLM version and configuration are recorded, and ordered A→B→C fallback is proven with real counters. Explicit skips are carried forward to G1/G2/G5 rather than claimed as proven.
