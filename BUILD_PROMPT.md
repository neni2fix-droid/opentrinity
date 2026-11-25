# OPENTRINITY — NENIFIX :: Operating Contract

This file is the working agreement for the implementing agent. The normative
technical specification is `SPEC.md` (v1.6.6). If this file and `SPEC.md`
disagree, **`SPEC.md` wins** and the conflict must be reported.

---

## 1. What is being built

A local-first, OpenAI-compatible gateway with one public alias (`nenifix-coder`),
a Go control plane, a Templ/HTMX dashboard, and a first-class `opentrinity` CLI.

```
opentrinity supervisor (Go)
 ├── opentrinity-gateway   127.0.0.1:3888   thin ingress only
 ├── opentrinity-control   127.0.0.1:8369   dashboard + /api/v1
 └── litellm proxy         127.0.0.1:3889   sole inference router
```

## 2. Continuous execution

1. Start at Phase 0. Never write product feature code before the Phase 0 gate passes.
2. Proceed through phases sequentially. After a phase gate passes, write its report
   (`docs/PHASE_REPORTS/phase-N.md`, template = SPEC.md §25) and continue automatically.
3. Never continue past a failed gate.
4. Fix ordinary bugs and rerun the relevant tests — that is not a gate failure.

## 3. Hard stops — stop and ask the owner only when

- A Phase 0 spike proves a mandatory behaviour cannot be implemented exactly with the
  pinned dependencies.
- A CORE test exposes an architectural conflict that cannot be resolved without changing
  a normative decision.
- A required destructive or external action lacks authorization.
- The owner must supply a real key for an explicitly requested live integration test and
  no deterministic fake test can establish the property.

A missing live provider key is **not** a hard stop: mark the live test
`SKIP — key unavailable`, prove the property with the fake upstream, and continue.

## 4. Evidence rules

- Real command output only. Never fabricate results, timings, prices, availability,
  compatibility or version behaviour.
- Fallback is **not proven** without printed per-slot (A/B/C) fake-upstream counters.
- Every phase report lists every applicable CORE test ID as `PASS` / `FAIL` / `SKIP`
  with short evidence; every `SKIP` carries a reason.
- A phase is never `PASS` while a required test failed or is unexplained-skipped.
- A live provider test is evidence of availability **at that time only**.

## 5. Division of labour (non-negotiable)

**Go owns:** process supervision and containment, ingress authentication, endpoint
allowlisting, request limits, cancellation propagation, configuration generation,
generations and atomic apply/rollback, telemetry and spool, control API, dashboard, CLI,
diagnostics, backup.

**LiteLLM owns:** alias → deployment mapping, ordered A→B→C fallback, retries, cooldown
(only if Phase 0 proves exact semantics), load balancing, provider protocol translation.

Go must never select providers, reorder slots, retry, classify errors in order to
initiate fallback, rewrite model names, or implement cooldown/circuit breaking.

## 6. Never do

- No Rust. No Docker/Kubernetes/Redis/PostgreSQL/second database. No local model
  downloads, no GPU requirement.
- No second public alias. No silent model substitution.
- No secret in SQLite, YAML values, logs, events, HTML, JSON responses, URLs, source
  control, process arguments, diagnostics or test snapshots.
- No embedded, shared or shipped provider keys — including free-tier keys.
- No `0.0.0.0`, LAN, tunnel, UPnP or firewall exposure.
- No arbitrary provider URL fetches; no disabled SSRF/DNS-rebinding checks.
- No prompts, completions, tool schemas/arguments/results, source code or raw upstream
  bodies in telemetry.
- No fabricated evidence and no compatibility claim that was not tested.
- No push, publish, deploy or release without explicit owner approval.
- No deletion or overwrite of unrelated user files.

## 7. Platform reality on this machine (recorded in Phase 0)

- Go 1.26.3 windows/amd64, `CGO_ENABLED=0` → CGO-free SQLite driver required.
- Python is **not on PATH**; the `py` launcher resolves 3.11 / 3.12 / 3.14.
  The LiteLLM venv is pinned to CPython 3.11 by absolute interpreter path.
- No Linux host available for local execution → Linux spikes are reported
  `SKIP — no Linux host`, never `PASS`.

## 8. Reporting

Every phase report uses the SPEC.md §25 template verbatim, including:
`CORE test results (ID + PASS/FAIL/SKIP + evidence)`, `LIVE-FREE test results (if attempted)`,
`Security observations`, `Known limitations`, `Open decisions requiring owner`,
`Evidence/log locations`, `Gate result: PASS / FAIL / BLOCKED`.
