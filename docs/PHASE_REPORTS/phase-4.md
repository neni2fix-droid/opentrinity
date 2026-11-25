# Phase report — Gate G4 (CLI, TUI and first-run path)

Phase: G4 — Core CLI, small Bubble Tea TUI and first-run path
Date: 2026-11-24

## Scope completed

- CLI commands: `init`, `start`, `stop`, `restart`, `status`, `doctor`, `providers`, `models`, `credentials`, `deployments`, `apply`, `gateway key`, `tui`.
- Secret entry via masked terminal read for interactive CLI; `credentials stdin` added strictly as a non-interactive local input path for automation. No secret is accepted as an argv value.
- Gateway key reveal requires typing `SHOW`; rotate never displays the value.
- Small Bubble Tea TUI with status, provider add, masked credential entry, deployment slot assignment, apply/rollback confirmation, help and quit.
- TUI calls the same database/config/apply services as CLI; no duplicate business logic.
- Non-interactive terminal detection returns a helpful error rather than hanging.

## Files created/changed

- `cmd/opentrinity/main.go`
- `internal/tui/tui.go`, `internal/tui/tui_test.go`
- `dist/opentrinity.exe` (build artifact)

## Dependency versions

- `github.com/charmbracelet/bubbletea v1.3.10` — MIT.
- Transitive Charmbracelet dependencies pinned in `go.sum` (lipgloss v1.1.0, x/ansi v0.10.1, x/cellbuf, colorprofile, termenv etc.).
- `golang.org/x/term v0.48.0` — Go BSD-style licence.
- `github.com/danieljoos/wincred v1.2.3` — MIT.

## Commands executed

```bash
go build -o dist/opentrinity.exe ./cmd/opentrinity
go test -vet=off ./internal/tui
bash scripts/acceptance/run.sh
printf 'SHOW\n' | dist/opentrinity.exe gateway key show
```

## Tests executed

- `internal/tui`: masked input never renders secret; keyboard navigation and ctrl+c quit — PASS.
- Acceptance first-run path: clean user-data dir → init → providers → credentials → models → deployments → apply → start → successful chat completion → stop/restart of children — PASS.
- Gateway key reveal with explicit `SHOW` confirmation succeeded during acceptance; value was not written to evidence.

## CORE test results

- `CORE-SEC-003`: PASS — key reveal required explicit confirmation; value absent from terminal logs/evidence.
- `CORE-SEC-004`: PASS — TUI and CLI secret paths store values only in Credential Manager.
- `CORE-PROC-001`: PASS — singleton enforced by CLI start and supervisor lock.
- `CORE-DATA-001`: PASS — clean user-data initialization worked in acceptance.
- `CORE-APPLY-001`: PASS — `apply run` activated generated config.
- TUI keyboard workflow: PASS for navigation/masking in unit tests; full Windows terminal interactive run was not automated.

## LIVE-FREE test results

Not attempted.

## Manual verification

`gateway key show` prompt was observed in acceptance output; no key value was echoed to the transcript or evidence files.

## Security observations

- TUI renders masked input as asterisks and clears the in-memory input immediately after submission.
- Non-interactive TUI mode fails with a CLI guidance message.

## Known limitations

- TUI full interactive flow on a physical Windows terminal was not automated; only unit-level keyboard/masking tests and non-interactive detection are evidenced.
- The TUI does not include model add or credential rotate; those remain CLI-only in MVP.
- `credentials stdin` is for automation and reads a single line from stdin; it must not be used with untrusted input streams.

## Open decisions requiring owner

None.

## Evidence/log locations

- `docs/PHASE_REPORTS/evidence/acceptance-summary.txt`
- `internal/tui/tui_test.go`

## Gate result: PASS
