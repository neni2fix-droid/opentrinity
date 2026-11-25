# Changelog

All notable changes to OPENTRINITY are recorded here.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/);
versioning is [SemVer](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Phase 0 — Feasibility spikes (in progress)

- Repository scaffolded: `cmd/`, `internal/`, `web/`, `tests/`, `scripts/`, `docs/`.
- Adopted `SPEC.md` v1.6.6 as the single normative specification.
- Recorded toolchain baseline:
  - Go 1.26.3 `windows/amd64`, `CGO_ENABLED=0`
  - Windows 11 Pro 10.0.26100
  - git 2.53.0.windows.2
  - CPython 3.11.9 at `%LOCALAPPDATA%\Programs\Python\Python311\python.exe`
    (not on `PATH`; reached via the `py` launcher or absolute path)
  - PowerShell 5.1.26100.9711
