# Phase report — Gate G3 (config store, secrets and safe apply)

Phase: G3 — Minimal SQLite store and safe apply
Date: 2026-11-24

## Scope completed

- CGO-free SQLite store with WAL, foreign keys, busy timeout, integrity check and a single logical writer (`SetMaxOpenConns(1)` plus write mutex).
- Migrations for settings, providers, credentials (references only), models, deployments, config generations and apply runs.
- LiteLLM YAML generated from validated state with `os.environ/...` secret references; only loopback HTTP or public HTTPS API bases accepted, with no network fetch during validation.
- Atomic candidate write, activation through the supervisor, last-known-good copy and automatic restore when health checks fail.
- Gateway and LiteLLM keys generated with crypto/rand and stored only in Windows Credential Manager.

## Files created/changed

- `internal/database/store.go`, `internal/database/store_test.go`
- `internal/routingconfig/generate.go`, `internal/routingconfig/generate_test.go`
- `internal/apply/apply.go`, `internal/apply/apply_test.go`
- `internal/secrets/store.go`
- `internal/supervisor/supervisor.go`

## Dependency versions

- `modernc.org/sqlite v1.62.0` — SQLite public-domain style licence wrapper; previously proven with the Phase 0 spike on Windows/amd64 with CGO disabled.
- `github.com/danieljoos/wincred v1.2.3` — MIT.

## Commands executed

```bash
go test -vet=off ./internal/database ./internal/apply ./internal/routingconfig
bash scripts/acceptance/run.sh
```

## Tests executed

- database: fresh migration, integrity, WAL, credential CRUD, no secret value in DB file — PASS.
- apply: validate does not replace active config; failed activation restores active file; successful apply stores last-good and rollback works — PASS.
- routingconfig: environment-only references, contiguous slots, URL validation — PASS.
- acceptance canary scan across user-data files and evidence — PASS.

## CORE test results

- `CORE-DATA-001`: PASS — fresh migrations and integrity check passed.
- `CORE-DATA-002`: SKIP — migration checksum mismatch is not implemented in MVP; no false claim.
- `CORE-DATA-003`: PASS (limited) — WAL enabled and `VACUUM INTO` backup proven in Phase 0; explicit crash-kill recovery test not repeated in MVP.
- `CORE-DATA-004`: PASS — single connection and write mutex serialize writes.
- `CORE-APPLY-001`: PASS — valid candidate applied atomically; generation recorded.
- `CORE-APPLY-002`: PASS — `apply validate` left active config byte-for-byte unchanged.
- `CORE-APPLY-003`: PASS — failing activation restored active config automatically.
- `CORE-APPLY-004`: PASS (limited) — last-good copy persisted and `apply rollback` restored it; crash-during-apply was not injected.
- `CORE-SEC-003`: PASS — canary scan found no gateway key in DB, YAML, state, logs or evidence.
- `CORE-SEC-004`: PASS — canary scan found no provider key in DB, YAML, state, logs or evidence.

## LIVE-FREE test results

Not attempted.

## Manual verification

Generated YAML inspected during acceptance: all `api_key` entries were `os.environ/...`; no secret values.

## Security observations

- Credential Manager target names and generated secret references are safe to persist; values are not.
- Secrets are passed to LiteLLM only via child-process environment variables, never argv or YAML.

## Known limitations

- No migration checksum, no spool/event database, no backup UX; these are deferred per MVP prompt.
- Crash-during-apply and PID-reuse destructive scenarios were not injected.
- Credential Manager writes from repeated acceptance runs leave test credentials under unique `opentrinity/secret/...` targets; they contain only fake placeholder keys and can be removed manually if desired.

## Open decisions requiring owner

- Windows Credential Manager was used as preferred; no ACL-file fallback was introduced.

## Evidence/log locations

- `docs/PHASE_REPORTS/evidence/acceptance-summary.txt`
- Unit test output recorded in this report.

## Gate result: PASS
