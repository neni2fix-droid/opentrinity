# OPENTRINITY — Bubble Tea Real-Time Terminal Dashboard Build Prompt

Source: Notion page "OPENTRINITY — Bubble Tea Real-Time Terminal Dashboard Build Prompt" (saved 2026-11-24).

## Implementation Prompt — Go / Bubble Tea

### 1. Project objective

Build a professional, real-time terminal dashboard for OPENTRINITY using:

- Go
- Bubble Tea (Charmbracelet)
- Lip Gloss for styling
- Bubbles for reusable terminal components
- Go standard library HTTP client
- Server-Sent Events (SSE) for live updates

Application name: `opentrinity-tui`.

The TUI is a companion operational interface. It does NOT replace the existing browser dashboard, FastAPI control plane, CLI or inference gateway.

Targets:

- Windows 11 first; Linux compatible.
- Lightweight on 4–8 GB RAM computers.
- Responsive, keyboard-first interface.
- No Docker requirement.

### 2. Read the existing specification first

Before coding, inspect the latest approved `OPENTRINITY_SPEC_v1.3` and the existing repository. Treat v1.3 as authoritative.

Inspect existing FastAPI routes, authentication, shared business services, credential/keyring implementation, provider testing, configuration generation and apply flow, SSE events, audit logging, CLI commands and tests.

Do not invent duplicate business logic or a second credential system. If an endpoint is missing, implement it in the FastAPI control plane using existing shared services and document the addition.

### 3. Architecture

```
                  OPENTRINITY TUI
                 Go + Bubble Tea
                        |
                 HTTP / SSE client
                        |
                        v
             FASTAPI CONTROL PLANE
                  127.0.0.1:4001
                        |
           +------------+------------+
           |            |            |
        Services      SQLite       Keyring
           |          (WAL)       / DPAPI
           |
      Config generator
           |
      Apply / validation
           |
           v
       LITELLM GATEWAY
        127.0.0.1:3888
```

Rules:

- TUI must never connect directly to SQLite.
- TUI must never access the OS keyring.
- TUI must never communicate directly with upstream AI providers.
- All operations go through authenticated control API endpoints.
- LiteLLM remains the only inference router.
- Existing browser dashboard must continue working.
- TUI must not be required for gateway operation.

### 4. Dashboard navigation

Create a polished terminal dashboard with a dark, restrained visual style, clear headers, compact panels, status indicators, progress bars, tables, responsive layouts, keyboard shortcuts and confirmation dialogs.

Navigation:

- F1 Overview
- F2 Providers
- F3 Credentials
- F4 Models
- F5 Deployments
- F6 Routing
- F7 Activity
- F8 Agents
- F9 Diagnostics
- F10 Settings
- Q Quit; ? Help; R Refresh

Support number-key navigation where function keys are unavailable.

### 5. Overview screen

Display control-plane status, gateway status and uptime, active model alias, deployment chain, provider health, total/successful/failed requests, average latency, recent fallback events, event-ingestion lag and last successful provider test.

Label data LIVE, CACHED or LAST KNOWN. Never invent metrics or present placeholders as real measurements.

### 6. Providers screen

Support OpenRouter, DeepSeek and Qwen / DashScope.

Show provider name, enabled/disabled status, configured credentials, available models, health, last test result, last checked time, region and base URL where applicable.

Actions: test provider, view credentials, add credential, enable/disable credential and view available models.

### 7. Credentials screen — critical feature

Build a complete interactive credential manager.

Display provider, label, status, last-tested time and masked key suffix only. All displayed values must come from the API, not hardcoded examples.

Actions:

- Add API key.
- Replace API key.
- Test API key.
- Enable credential.
- Disable credential.
- Delete credential.
- View credential metadata.

#### Add credential workflow

1. Select provider.
2. Enter credential label.
3. Enter API key using masked terminal input.
4. Confirm provider selection.
5. Submit securely to the authenticated FastAPI endpoint.
6. Control plane stores it in the OS keyring.
7. Test the credential.
8. Display result without exposing the key.

Support OpenRouter, DeepSeek and Qwen. Never automatically test an ambiguous key against multiple providers.

#### Replace credential workflow

Implement this sequence:

```
SELECT EXISTING CREDENTIAL
          |
          v
    ENTER NEW KEY
          |
          v
     TEST NEW KEY
          |
      +---+---+
      |       |
    FAIL    SUCCESS
      |       |
      v       v
 KEEP OLD   APPLY CHANGE
 KEY ACTIVE     |
                v
        CHECK DEPENDENT SLOTS
                |
                v
         POST-APPLY TEST
                |
           +----+----+
           |         |
         FAIL      SUCCESS
           |         |
           v         v
       ROLLBACK   DELETE OLD KEY
```

Requirements:

- Test new key before changing anything.
- Keep old credential active during validation.
- Use standard configuration apply process.
- Test every deployment slot using the credential.
- Delete old secret only after successful post-apply validation.
- Roll back on failure.
- Explain failure clearly.
- Audit every mutation.
- Require confirmation for deletion.

#### Secret handling

- Mask all API-key input; disable terminal echo while typing.
- Never place secrets in command-line arguments, TUI config files, logs, debug output or persistent application state.
- Never display full upstream keys.
- Clear sensitive input buffers as soon as practical.
- Local control API must remain loopback-only and authenticated.
- Never bypass API authentication.

### 8. Distinguish the three key types

**A. Upstream provider API keys**

- OpenRouter, DeepSeek and Qwen credentials.
- Add and replace through Credentials screen.
- Stored in OS keyring.

**B. Local gateway API key**

- Used by applications connecting to `127.0.0.1:3888/v1`.
- Provide a separate Gateway Security screen.
- Support reveal after explicit confirmation, temporary reveal, copy and rotate.
- Never confuse this with upstream provider credentials.

**C. Control-plane admin token**

- Protects FastAPI control API.
- Separate security-management workflow.
- Never display current token.
- Support secure rotation with confirmation.
- Never use admin token as inference gateway key.

### 9. Models screen

Display provider, model ID, display name, known context size, input/output pricing evidence, verification status, discovery timestamp and pinned status.

Actions: discover, search, add manually, view details, pin/unpin and test.

Never silently replace a pinned model. Unknown pricing must display UNKNOWN, never FREE.

### 10. Deployments screen

Show slots A (primary), B (secondary) and C (tertiary), with provider, model, credential label, enabled state, health, last test and circuit-breaker state.

Allow users to propose configuration changes. Show a configuration diff before applying. Use existing control-plane validation and apply services.

### 11. Routing screen

Show current routing policy, A → B → C fallback sequence, retry eligibility, circuit-breaker status, cooling-down slots and last fallback event.

Never implement routing logic inside the Go TUI.

### 12. Activity screen

Create a real-time activity stream showing request timestamp, model alias, selected deployment, provider, latency, success/failure, fallback events, sanitised error category and request identifier.

Do not display prompts, completions, source code, tool arguments, authorization headers or secrets.

Provide filters for provider, status and time window.

### 13. Real-time implementation

Use the existing FastAPI SSE implementation. Inspect its actual route and event schema before coding.

Requirements:

- Connect to existing SSE endpoint.
- Reconnect automatically after interruption.
- Use exponential backoff.
- Implement polling fallback.
- Show connection state and last event timestamp.
- Prevent duplicate event rendering.
- Avoid blocking Bubble Tea event loop.
- Keep background requests cancellable.
- Avoid excessive polling.

Target event reflection within two seconds under normal operating conditions, consistent with v1.3.

### 14. API client

Create dedicated Go package `internal/api/` for control-plane HTTP client, admin authentication, request timeouts, SSE, JSON decoding, error handling, API version compatibility and redaction.

Use the actual existing API contract. Do not invent route names or request payloads.

### 15. Suggested project structure

```
opentrinity-tui/
├── cmd/
│   └── opentrinity-tui/
│       └── main.go
├── internal/
│   ├── api/
│   │   ├── client.go
│   │   ├── auth.go
│   │   ├── events.go
│   │   └── errors.go
│   ├── app/
│   │   ├── model.go
│   │   ├── update.go
│   │   └── views.go
│   ├── screens/
│   │   ├── overview.go
│   │   ├── providers.go
│   │   ├── credentials.go
│   │   ├── models.go
│   │   ├── deployments.go
│   │   ├── routing.go
│   │   ├── activity.go
│   │   ├── agents.go
│   │   └── diagnostics.go
│   ├── components/
│   │   ├── header.go
│   │   ├── navigation.go
│   │   ├── status.go
│   │   ├── table.go
│   │   ├── modal.go
│   │   └── secret_input.go
│   └── styles/
│       └── theme.go
├── tests/
├── go.mod
├── go.sum
└── README.md
```

Adapt this structure to the existing repository without disturbing unrelated files.

### 16. Integration

Add command:

```
opentrinity tui
```

It must check whether control plane is running, show exact startup hint if unavailable, authenticate securely, start Bubble Tea, load initial state and subscribe to live events.

Do not create a second daemon or database.

### 17. Testing and acceptance criteria

Implement automated tests for:

- API authentication.
- SSE connection and reconnection.
- Polling fallback.
- Terminal resizing and keyboard navigation.
- Provider and credential listing.
- Masked secret input.
- Add and replace credential workflows.
- Failed key test retains old key.
- Successful replacement deletes old key only after post-apply validation.
- Rollback on post-apply failure.
- Credential deletion confirmation.
- Gateway key rotation.
- Admin-token rotation.
- Audit-log creation.
- Secret redaction.
- No direct SQLite access.
- No direct provider calls.
- No secrets in logs, configuration or terminal output.

Use mock API responses for UI tests and the existing OPENTRINITY test environment for integration tests. Never use real API keys in automated tests.

### 18. Build targets

Produce Windows AMD64 and Linux AMD64 executables; Linux ARM64 where practical. Use standard Go cross-compilation and keep dependencies minimal.

### 19. Documentation

Create:

- `README.md`
- `docs/INSTALL.md`
- `docs/KEY_MANAGEMENT.md`
- `docs/SECURITY.md`
- `docs/SHORTCUTS.md`

Document the distinction between upstream provider keys, gateway keys and admin tokens.

### 20. Execution instructions

Work in small, testable phases:

1. Repository inspection and API contract discovery.
2. Bubble Tea application foundation.
3. API client and authentication.
4. Overview and navigation.
5. Providers and credentials.
6. Secure key replacement and rollback.
7. Models and deployments.
8. Routing and activity.
9. SSE and polling fallback.
10. Diagnostics, tests and packaging.

At the end of each phase, run tests, inspect changes, record results, update CHANGELOG.md and document deviations from v1.3.

Do not rewrite the existing OPENTRINITY application. Do not replace the browser dashboard. Do not bypass existing control-plane security.

**Definition of done:** A working `opentrinity tui` application that provides real-time monitoring and safe operational control of OPENTRINITY through its existing FastAPI control plane, with secure credential management and no duplicated business logic.
