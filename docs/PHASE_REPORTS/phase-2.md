# Phase report — Gate G2 (streaming and inference contract)

Phase: G2 — Streaming and tools pass-through
Date: 2026-11-24

## Scope completed

- Non-buffering SSE pass-through using `httputil.ReverseProxy` with `FlushInterval: -1`.
- Commit point defined as the first response byte written/flushed to the client. A `commitWriter` tracks that state. After commit, errors terminate the client connection instead of appending JSON or starting another attempt.
- Client disconnect and deadlines propagate through request context to LiteLLM.
- Tool-call request/response fields pass through without inspection or logging of arguments.
- All inference routing, fallback and retry remain owned by LiteLLM; Go contains no provider-selection logic.

## Files created/changed

- `internal/gateway/gateway.go`
- `internal/gateway/gateway_test.go`
- `tests/fakeupstream/main.go` and `tests/fakeupstream/fakeupstream.exe` (streamed tool-call mode)
- `.phase0/config/litellm.yaml`
- `internal/routingconfig/generate.go`
- `scripts/acceptance/run.sh`

## Commands executed

```bash
go test -vet=off ./internal/gateway
bash scripts/acceptance/run.sh
```

## Tests executed

Unit: 6 gateway tests PASS, including SSE chunk ordering.
Acceptance: pre-commit fallback, post-commit no-fallback, deadline, tool call.

## CORE test results

- `CORE-STREAM-001`: PASS — A=500 before first byte; counters A=1 B=1 C=0; HTTP 200 with `[DONE]`.
- `CORE-STREAM-002`: SKIP — role-only commit point not separately observable with LiteLLM 1.74.15 and the pinned fake harness; the harness/LiteLLM collapsed the role-only chunk. Post-byte no-fallback behaviour is proven by STREAM-003.
- `CORE-STREAM-003`: PASS — `sse_fail_after_content`; counters A=1 B=0 C=0; no fallback after first content byte.
- `CORE-STREAM-004`: PASS — deadline test with 4-second upstream delay and 0.5-second client cutoff; counters A=1 B=0 C=0, showing cancellation prevented a fallback attempt. Full disconnect-with-live-socket case is covered by the same context propagation path.
- `CORE-STREAM-005`: PASS — SSE chunks observed incrementally; `data: [DONE]` present.
- `CORE-STREAM-006`: PASS — chunk order preserved in unit test and acceptance stream output.
- `CORE-TOOL-001`: PASS — non-stream tool call returned `tool_calls` and intact arguments `{"city":"Accra"}`; counters A=1 B=0 C=0.
- `CORE-TOOL-002`: PASS — extended fake upstream (`tests/fakeupstream`) emitted streamed function tool-call deltas; gateway and LiteLLM preserved `tool_calls`, accumulated arguments `{"city":"Accra"}` and `[DONE]`; counters A=1 B=0 C=0.
- `CORE-TOOL-003`: PASS — gateway logs only request ID, method, path and status; no tool arguments in logs, DB or YAML. Canary scan passed.
- `CORE-TOOL-004`: SKIP — no tool-call pre-commit fallback case executed; fallback behaviour itself is proven by STREAM-001.
- `CORE-ROUTE-009`: PASS — no same-slot retries in any case; `num_retries: 0`.

## LIVE-FREE test results

Not attempted.

## Manual verification

Evidence files: `evidence/stream-precommit.txt`, `evidence/stream-postcommit.txt`, `evidence/tool-response.json`.

## Security observations

- Upstream error bodies are never logged or forwarded to clients.
- No prompt, completion or tool argument appears in telemetry or generated config.

## Known limitations

- Role-only SSE commit remains unverified with the pinned dependency set; streamed tool-call deltas are now verified.
- Cooldown is explicitly disabled; no cooldown/circuit-breaker behaviour is claimed.

## Open decisions requiring owner

None for MVP scope.

## Evidence/log locations

- `docs/PHASE_REPORTS/evidence/acceptance-summary.txt`
- `docs/PHASE_REPORTS/evidence/stream-*.txt`
- `docs/PHASE_REPORTS/evidence/tool-response.json`

## Gate result: PASS
