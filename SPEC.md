# OPENTRINITY — NENIFIX
## FINAL BUILD PROMPT — Master Specification v1.6.6
### Go-only · Templ/HTMX · LiteLLM-only inference routing · Local-first · Free-tier capable

**Status:** Corrected normative build specification  
**Supersedes:** v1.6.5 and all earlier OPENTRINITY specifications  
**Primary platform:** Windows 11; Linux support designed in from the start  
**Repository:** `C:\Users\ai9\Documents\opentrinity` (or the working directory supplied)  
**Public local API:** `http://127.0.0.1:3888/v1`  
**Private LiteLLM upstream:** `http://127.0.0.1:3889`  
**Control API / Dashboard:** `http://127.0.0.1:8369`  
**CLI:** `opentrinity`  
**Public model alias:** `nenifix-coder`

This document is the single normative source of truth. It incorporates the useful additions in v1.6.5 and resolves identified contradictions and implementation risks. Do not combine it with older versions unless the owner explicitly requests that.

---

# AGENT EXECUTION CONTRACT

You are Command Code (or an equivalent coding agent). Implement OPENTRINITY completely, phase by phase, in the supplied repository.

## Continuous execution

1. Begin with repository inspection and Phase 0.
2. Proceed sequentially through phases only after the current phase gate passes.
3. After a phase passes, write its report and continue automatically. Do not ask for permission between successful phases.
4. Never continue past a failed gate. Debug ordinary implementation defects. If a required behaviour is architecturally impossible with the pinned dependency, or a CORE test remains blocked after reasonable debugging, stop and present evidence and the smallest decision needed from the owner.
5. If a real external API key is unavailable, use fake upstreams for automated routing proof. Mark the corresponding live-provider test `SKIP — key unavailable`; do not invent a success.
6. Do not stop merely because a live provider key is absent if the phase can be completed and proven with deterministic fake upstreams. Live tests are an additional integration check, not a substitute for fake-upstream proof.
7. Every phase report must list every CORE test ID relevant to that phase as `PASS`, `FAIL`, or `SKIP`, with concise evidence and a reason for every skip.
8. Never mark a phase `PASS` while a required test is failed or an unexplained test is skipped.
9. Preserve unrelated user work. Never delete or overwrite unreviewed files.
10. Never push, publish, deploy or distribute a release without explicit owner approval.
11. Never fabricate output, test evidence, provider availability, pricing, model capabilities, compatibility or version behaviour.

## Hard-stop conditions

Stop and ask the owner only when:
- A Phase 0 spike proves a mandatory behaviour cannot be implemented exactly using the selected pinned dependency.
- A CORE test reveals an architectural conflict that cannot be resolved without changing a normative decision.
- A required destructive operation or external action lacks authorization.
- The owner must provide a real key for an explicitly requested live integration test and no fake test can establish the requested property.

For an ordinary bug, fix it and rerun the relevant tests.

## Non-negotiable engineering rules

- LiteLLM is the sole inference router.
- Go must not implement provider selection, model fallback, retries, cooldown, load balancing or routing policy.
- Go may implement ingress authentication, endpoint allowlisting, limits, request cancellation, process supervision, config generation and telemetry.
- If pinned LiteLLM cannot provide a required behaviour exactly, disable it or stop for owner decision. Never approximate it in Go.
- Pin and record exact dependency versions and licences.
- No Docker, Kubernetes, Redis, PostgreSQL, second database, local model downloads, GPU requirement or Rust.
- Bind all services to loopback only.
- Never store raw secrets in SQLite, YAML, logs, events, HTML, JSON responses, URLs, source control, process arguments or test snapshots.
- Do not hard-code or share provider API keys, including free-tier keys.
- Never claim fallback works without actual per-slot fake-upstream counters.
- Keep all provider/deployment changes explicit, validated, auditable and reversible.

---

# 0. Definitions

- **Supervisor:** Go parent process that owns the lifecycle of child processes.
- **Ingress gateway:** Thin Go HTTP boundary exposed to local clients on port 3888.
- **LiteLLM Proxy:** Sole inference router, private on port 3889.
- **Control plane:** Go HTTP API and Templ/HTMX dashboard on port 8369.
- **Deployment slot:** One configured provider/model/credential target in ordered slot A, B or C.
- **Public alias:** Stable client-facing model name `nenifix-coder`.
- **Generation:** Immutable, numbered snapshot of validated runtime configuration.
- **Commit point:** First response-body byte successfully flushed/written to the client. For SSE, a role-only event counts.
- **Free-tier capable:** The user can configure a provider/model that the provider currently offers under a no-token-charge/free allocation. This does not guarantee permanent availability, a fixed quota, zero ancillary charges or uninterrupted service.
- **CORE test:** A required deterministic acceptance test identified by a stable ID in this document.

---

# 1. Product goal and client contract

OPENTRINITY is a local-first OpenAI-compatible API gateway and control plane. It exposes one stable local endpoint and one public model alias while LiteLLM performs sequential deployment routing and fallback.

Default client settings:

```text
Base URL: http://127.0.0.1:3888/v1
API key:  <local OPENTRINITY gateway key>
Model:    nenifix-coder
```

## 1.1 Public ingress endpoints

| Method | Path | Purpose |
|---|---|---|
| GET | `/health` | Gateway process liveness |
| GET | `/ready` | Gateway-to-LiteLLM readiness |
| GET | `/v1/models` | OpenAI-compatible model list |
| POST | `/v1/chat/completions` | Chat completions, streaming and non-streaming |

All other paths and methods are rejected at ingress. Do not expose LiteLLM admin routes, metrics, spend endpoints, model management or arbitrary reverse-proxy paths.

## 1.2 Compatibility

Support the OpenAI Chat Completions request/response contract to the extent supported by the pinned LiteLLM release and configured upstream provider.

Required:
- Bearer authentication with the local OPENTRINITY gateway key.
- Model alias `nenifix-coder`.
- Non-streaming chat completions.
- SSE streaming.
- Tool/function calls and streamed tool-call deltas when supported by the upstream.
- OpenAI-style sanitized error envelope.
- Cancellation propagated to LiteLLM.
- Unknown aliases rejected before an upstream request.
- Unknown paths/methods rejected without forwarding.

Do not claim universal compatibility. A client must support a configurable OpenAI-compatible base URL, bearer key and model name. This release does not claim compatibility with Anthropic Messages, OpenAI Responses, embeddings, image generation or clients that hard-code vendor endpoints.

## 1.3 Alias validation boundary

The ingress may validate that the requested model equals the one configured public alias, `nenifix-coder`. This is an allowlist check only. It must not select a provider, map to a provider model, or choose a deployment. LiteLLM performs alias-to-deployment routing.

---

# 2. Technology decisions and process topology

| Layer | Required choice |
|---|---|
| Main application | Go |
| Supervisor | Go |
| Ingress gateway | Go, thin pass-through only |
| Control API | Go |
| CLI | Go (`opentrinity`) |
| Dashboard | Templ + HTMX + HTML + CSS |
| Inference routing | LiteLLM Proxy only |
| State | SQLite, WAL, one logical writer |
| Secrets | OS credential store; secure documented fallback only |
| Runtime config | Generated from validated SQLite state |
| Event buffer | Append-only JSONL spool |
| Windows process containment | Windows Job Object |
| Linux process containment | Process group / parent-child lifecycle |
| Containers / Rust | Not used |

## 2.1 Normative process topology

```text
opentrinity supervisor
 ├── opentrinity-gateway   127.0.0.1:3888
 ├── opentrinity-control   127.0.0.1:8369
 └── litellm proxy         127.0.0.1:3889
```

The supervisor owns lifecycle only. The ingress gateway does not route inference. LiteLLM is the only inference router.

## 2.2 Ingress gateway permitted responsibilities

The Go ingress may:
- Bind to `127.0.0.1:3888`.
- Validate the local gateway bearer key.
- Validate the one configured public alias.
- Enforce exact method/path allowlist.
- Enforce body, header, concurrency and connection limits.
- Add/propagate a correlation ID.
- Forward the request to private LiteLLM.
- Stream the response without full buffering.
- Propagate client cancellation.
- Emit sanitized operational events.

It must not:
- Select providers or deployment slots.
- Reorder A/B/C.
- Retry requests.
- Interpret upstream errors to initiate fallback.
- Rewrite model names to provider/model names.
- Implement cooldown, circuit breaking, load balancing or retry policy.
- Modify prompts, messages, tools, tool arguments or completions.

## 2.3 Ports

- Ingress: `127.0.0.1:3888`
- LiteLLM private listener: `127.0.0.1:3889`
- Control API/dashboard: `127.0.0.1:8369`

These are the only default service ports. No `0.0.0.0`, LAN, public binding, tunnel, UPnP or firewall exposure. Port changes require validated settings and must be reflected consistently in generated client configuration and diagnostics.

## 2.4 Control-plane outage isolation

Gateway and LiteLLM continue serving with the last successfully applied immutable generation if the control plane crashes or SQLite becomes temporarily unavailable.

- The gateway must not query SQLite on every request.
- The supervisor loads a validated runtime snapshot and passes the minimum required configuration to the gateway through a protected local mechanism.
- On successful apply, supervisor distributes a new immutable snapshot and atomically switches the gateway to it.
- If snapshot distribution or gateway reload fails, retain the previous snapshot and report apply failure.
- Do not expose raw secrets in the snapshot. The gateway receives only the gateway-key verifier and non-secret ingress settings.

---

# 3. Supervisor, lifecycle and recovery

## 3.1 Responsibilities

The supervisor:
- Acquires a single-instance lock.
- Creates secure data, config, runtime and log directories.
- Loads active and last-known-good configuration generations.
- Resolves required provider secrets from the OS credential store.
- Generates LiteLLM configuration atomically.
- Starts LiteLLM, waits for readiness, then starts ingress and control plane.
- Monitors child exit and readiness independently.
- Restarts only failed children with bounded exponential backoff and jitter.
- Prevents children outliving the supervisor.
- Coordinates graceful shutdown.
- Writes sanitized lifecycle events to spool.
- Never logs secrets or complete child environments.

## 3.2 Startup

1. Acquire singleton lock.
2. Validate directories, permissions and SQLite integrity.
3. Load active and last-known-good generations.
4. Resolve required secret references; fail closed if a required secret is missing.
5. Generate private LiteLLM configuration.
6. Validate configuration using the pinned LiteLLM version.
7. Start LiteLLM on 3889.
8. Wait for LiteLLM readiness with bounded timeout.
9. Start gateway on 3888 with active snapshot.
10. Start control plane on 8369.
11. Report system readiness only when gateway and LiteLLM are ready and the active generation is valid.

If LiteLLM is unavailable, `/health` may remain successful but `/ready` must fail. The control plane may run in diagnostic mode.

## 3.3 Restart policy

Defaults: initial delay 1 second, multiplier 2, maximum 60 seconds, jitter ±20%, reset after sustained healthy operation. These values must be configurable and tested.

- Control-plane crash must not stop gateway or LiteLLM.
- Gateway crash must not unnecessarily restart LiteLLM.
- LiteLLM crash marks gateway readiness false; restart LiteLLM and revalidate.
- Repeated failures mark the component degraded and expose diagnostics.
- Never tight-loop on invalid configuration.
- Preserve last-known-good generation.

## 3.4 Windows

- Use Windows Job Object so children terminate with supervisor.
- Per-user installation; no admin required for normal operation.
- Verify PID, executable path and process creation identity before signalling.
- Never kill a process only because a port is occupied.
- Report unrelated port owner and fail safely.
- Graceful shutdown before force termination.
- Use explicit executable + argument arrays; no shell command string.
- Never pass secrets in argv.

## 3.5 Linux

- Use process group or equivalent containment.
- Per-user restrictive directories.
- No root requirement for normal use.
- Optional `systemd --user`.
- Same PID identity and graceful shutdown requirements.

## 3.6 Health

- `/health`: ingress process is alive and can answer HTTP.
- `/ready`: active generation is valid and gateway can reach private LiteLLM.
- Provider health is reported separately and does not change process liveness.
- Control status reports supervisor, gateway, LiteLLM, database, spool and active generation independently.

---

# 4. Routing policy — LiteLLM only

## 4.1 Deployments

- One public alias: `nenifix-coder`.
- Up to three ordered deployment slots: A, B, C.
- A is primary; B and C are sequential fallbacks.
- Slots are independent provider/model/credential deployments, not load-balanced replicas.
- Slot order is explicit and persisted.
- No silent model substitution.
- A slot can use a paid or free-tier provider, provided the user supplies the required credential and the selected LiteLLM provider integration is verified.

## 4.2 Routing ownership

LiteLLM configuration expresses alias, deployment targets, ordered fallback and verified timeout/retry/cooldown settings. Go generates and validates this configuration only. Go must not make runtime routing decisions.

## 4.3 Retry and fallback

- Same-slot retry count defaults to zero.
- No Go retry or fallback layer.
- A→B→C order must be proven by fake-upstream request counters.
- No retry after response bytes are committed.
- LiteLLM retry settings may be enabled only if the exact semantics of the pinned version are proven and owner-approved.
- If LiteLLM cannot provide exact behaviour, disable the feature or stop for owner decision.

## 4.4 Cooldown

Disabled by default. Enable only after Phase 0 proves the pinned LiteLLM setting, trigger scope, duration, reset behaviour and interaction with 429/quota/auth errors. Never implement cooldown in Go.

## 4.5 Error taxonomy

Use only these normalized categories:

`connection`, `dns`, `tls`, `timeout_connect`, `timeout_first_token`, `timeout_total`, `deadline_exceeded`, `rate_limit`, `quota`, `upstream_5xx`, `upstream_auth`, `upstream_not_found`, `empty_response`, `malformed_upstream`, `client_error`, `unsupported_param`, `context_length`, `content_policy`, `stream_interrupted`, `client_disconnect`, `skipped_cooldown`, `internal`.

Rules:
- 401/403 marks the credential that produced the response `invalid`.
- Do not retry the same credential through a duplicate slot.
- A different slot may be attempted only if it has a different eligible credential and pinned LiteLLM supports the configured fallback semantics.
- Quota exhaustion marks that credential `quota_exhausted`; that credential becomes ineligible until successful explicit retest. Fallback may proceed only to another eligible deployment using a different eligible credential.
- Do not classify every 404 as a missing model. Verify provider error code/body; route/path 404 is a configuration error.
- Client errors, unsupported parameters, context length and content policy do not fallback by default.
- Overall request deadline is a hard stop.
- `timeout_total` is per-attempt timeout while overall budget remains.
- `deadline_exceeded` means the overall request budget is exhausted.

## 4.6 Time budgets

Initial defaults, subject to Phase 0 validation:
- Overall request deadline: 90 seconds.
- Connect timeout: 5 seconds.
- First-token timeout: 20 seconds.
- Per-attempt total timeout: 45 seconds.
- Same-slot retries: 0.

LiteLLM enforces routing/attempt timeouts. Ingress may enforce an outer client deadline and cancellation, but may not start a new attempt. Use monotonic elapsed-time measurement where available.

---

# 5. Streaming and tool-call contract

## 5.1 Commit point

Commit point is the first response-body byte successfully flushed/written to the client. For SSE, a role-only event or SSE comment counts if it is the first body byte.

Before commit:
- LiteLLM may perform only configured and proven fallback.
- Ingress may return a normal OpenAI-compatible JSON error.

After commit:
- No fallback.
- No response replacement.
- Terminate stream on upstream failure.
- Record `stream_interrupted` and `partial_content_sent=true`.
- Never append a JSON error body to an already-started SSE response.

## 5.2 Streaming

- Do not buffer the full stream.
- Preserve event order and data payloads.
- Flush promptly.
- Preserve required SSE headers.
- Do not inspect or rewrite prompt/completion content.
- Bound idle behaviour without truncating healthy streams.

## 5.3 Cancellation

Client disconnect cancels upstream immediately, releases resources, records `client_disconnect`, and never triggers fallback.

## 5.4 Tool calls

- Pass tool-call request fields and arguments through unchanged.
- Support non-stream and streamed tool-call deltas when upstream and pinned LiteLLM support them.
- OPENTRINITY does not execute tools.
- Never parse or log tool arguments for telemetry.
- Test multiple calls, partial streamed arguments, malformed chunks and fallback before stream commit.

---

# 6. Ingress security

## 6.1 Gateway key

- Generate high-entropy local key during initialization.
- Store raw key only in OS credential store.
- Store only a cryptographic verifier/hash in protected application state where feasible.
- Show raw key once at creation; reveal/rotate requires explicit local confirmation.
- Constant-time verifier comparison.
- Missing/invalid key rejected before forwarding; fake-upstream counter must remain zero.
- Never place key in URL, logs, events, process arguments or crash report.

## 6.2 Admin/browser authentication

- Control API/dashboard binds to loopback.
- State-changing actions require authenticated local admin session.
- One-time login code expires after 60 seconds; store only hash.
- Exchange code by POST, never query string.
- Session ID rotates after login.
- Cookies HttpOnly, SameSite and Secure when HTTPS is used.
- CSRF protection on state-changing requests.
- Validate Host and Origin against expected loopback origins.
- Reject cross-origin state changes.
- Session expiry, logout and revocation required.

## 6.3 Request limits

Defaults:
- Maximum request body: 10 MiB.
- Maximum concurrent chat requests: 20.
- Maximum active SSE streams: 20.
- Maximum header size: 32 KiB.
- Header read timeout: 10 seconds.
- Keep-alive timeout: 60 seconds.

Reject oversized bodies with 413 before forwarding. Return stable 429/503 errors for local admission limits. No unbounded in-memory queue.

## 6.4 Route allowlist

Exact method/path allowlist. Reject path traversal, encoded path tricks, unexpected methods, conflicting content-length headers and unsupported query parameters. Never act as a generic reverse proxy.

## 6.5 SSRF and outbound destinations

Provider endpoints are untrusted configuration:
- HTTPS by default.
- HTTP allowed only for explicitly enabled local fake-upstream test mode.
- Reject URL userinfo, fragments and unsupported schemes.
- Reject loopback, private, link-local, multicast and reserved destinations unless explicitly using a local fake upstream in test mode.
- Validate DNS resolution at configuration and connection time.
- Prevent DNS rebinding by validating the actual dialed IP, not only the hostname.
- Disable redirects by default; if any redirect is required, validate every target before following.
- Block endpoints resolving to OPENTRINITY's own ports.
- Do not fetch arbitrary URLs from provider/model metadata.

## 6.6 Secret storage

Preferred:
- Windows Credential Manager.
- Linux Secret Service/libsecret.

Fallback:
- Windows DPAPI-protected per-user secret file only if Credential Manager unavailable and fallback explicitly accepted.
- Linux owner-only `0600` file only as a clearly warned fallback; do not describe permissions alone as encryption.

Rules:
- Fail closed if required secret cannot be read.
- No silent downgrade to plaintext.
- Provider keys injected into LiteLLM child environment or an equally protected supported mechanism.
- Never persist resolved environment values in YAML, DB or diagnostics.
- Redact Authorization, API-key, cookies and common secret patterns.
- No prompts, completions, tool args, source code or raw upstream bodies in logs/events.

---

# 7. Free-tier provider support

## 7.1 Product promise — carefully bounded

OPENTRINITY must support user-configured free-tier models as normal deployment options in A, B or C. It does not provide keys, credits, quotas or guaranteed uptime.

“Free” is a time-sensitive provider/model/plan property, not an intrinsic permanent model property. The UI must distinguish:
- **Verified free:** current source confirms applicable input and output token prices are zero under stated conditions.
- **Free-plan access:** model access is included in a provider's free plan but may have rate, daily, regional, account or eligibility limits.
- **Unknown:** price/eligibility could not be verified.
- **Stale:** previously verified information is past its freshness window.

Never display Unknown or Stale as Free.

A free-tier provider may still require account registration, phone verification, billing setup, regional eligibility or acceptance of data-use terms. The UI must not imply that no account setup or billing details are ever required.

## 7.2 Initial provider integrations

Provide first-class configuration guidance and LiteLLM integration support for:
- OpenRouter, including explicit model IDs ending in `:free`.
- Groq, for models currently available under its free plan.
- Google AI Studio / Gemini, for models currently eligible under its free tier.
- Cloudflare Workers AI as optional/recommended where the user's account and current plan permit it.

The exact LiteLLM provider prefix, required account fields, environment variable names and model identifier must be verified against the pinned LiteLLM release during Phase 0. Do not assume a provider model prefix or auth shape based on this document alone.

Cloudflare may require account-specific identifiers in addition to an API token. Model it as provider-specific configuration, not as a generic single-key provider if the integration requires more fields.

## 7.3 Provider registration and credentials

- “Add Free Provider” is a guided UI path, not a separate credential security system.
- User supplies their own provider key/token.
- Store key/token only in OS credential store.
- Provider-specific non-secret fields (e.g. account ID) may be stored in SQLite after validation.
- Never ship, embed or share API keys.
- Explain provider rate limits and that provider terms/availability may change.
- Provider tests are explicit user actions and may be billable or consume free quota.

## 7.4 Model discovery and listing

- Discovery is optional and rate-limited.
- Prefer official provider model-list APIs or verified provider documentation.
- OpenRouter: when a user key exists, attempt to list currently available free models if the API supports reliable identification. Otherwise show a small curated list with last-verified timestamp and mark it stale when overdue.
- Do not infer that a model is free merely because its name contains `free`.
- Do not hard-code a permanent model list.
- Do not automatically change a pinned deployment when discovery changes.
- Show provider, exact model ID, pricing/eligibility status, source, last-checked timestamp, rate-limit warning and known restrictions.
- Allow any validated free-tier model to be assigned to A, B or C.

## 7.5 Dynamic free routers versus pinned free models

Treat these as different deployment types:

1. **Pinned free model:** an explicit provider/model identifier such as an OpenRouter model ending in `:free`. This is preferred when reproducibility and provider identity matter.
2. **Dynamic free router:** an upstream router such as a provider's “free” model selector that may choose among several underlying models. Clearly label it as dynamic, disclose that the actual underlying model may change, and do not describe it as a pinned model.

The OPENTRINITY alias remains `nenifix-coder`; LiteLLM remains the only router. A dynamic upstream router is an additional provider-side selection behaviour and must be disclosed in the deployment details and privacy information.

Do not include a hard-coded example model ID as a guaranteed live model. Use placeholders in documentation or Phase 0 verified model IDs with a verification date.

## 7.6 LiteLLM configuration and secrets

Generated LiteLLM YAML may contain secret *references* such as environment variable names, but never secret values.

Example pattern only; exact provider/model names and environment variable names must be verified:

```yaml
model_list:
  - model_name: nenifix-coder
    litellm_params:
      model: openrouter/VERIFIED_VENDOR/VERIFIED_MODEL:free
      api_key: os.environ/OPENROUTER_API_KEY
```

If multiple deployment slots share one alias, generate the exact LiteLLM structure proven in Phase 0. Do not assume the illustrative snippet alone implements A/B/C fallback.

## 7.7 Free-tier warnings

README and UI must explain:
- Free tiers have changing limits and policies.
- No daily quota or uptime is guaranteed.
- Providers may require account eligibility or billing details.
- Some providers may use submitted content for product improvement or model training under their current terms.
- Fallback may send request content to a different provider.
- The user must review each provider's current data handling and terms.
- A provider may impose ancillary charges or change its plan; OPENTRINITY cannot guarantee a $0 invoice.

## 7.8 Free-tier acceptance tests

Automated deterministic tests:
- `CORE-FREE-001`: Free-tier provider can be represented and validated in provider configuration using fake upstream.
- `CORE-FREE-002`: Free-tier model can be assigned to Slot A.
- `CORE-FREE-003`: Free-tier model can be assigned to Slot B or C and used by LiteLLM fallback.
- `CORE-FREE-004`: Free-tier credentials never appear in logs, events, DB, generated YAML values, errors or diagnostics.
- `CORE-FREE-005`: UI distinguishes verified free, free-plan access, unknown and stale.
- `CORE-FREE-006`: UI shows rate-limit, provider-policy and fallback data-sharing warnings.
- `CORE-FREE-007`: Dynamic free router is visibly distinguished from a pinned model.
- `CORE-FREE-008`: Model discovery cannot silently alter a pinned deployment.

Optional live tests, only with the owner's own credentials:
- `LIVE-FREE-001`: OpenRouter currently lists or accepts a user-selected free model and returns a completion.
- `LIVE-FREE-002`: Groq currently accepts a user-selected model under the user's current plan and returns a completion.
- `LIVE-FREE-003`: Google AI Studio/Gemini current free-tier model test, if owner configures it.
- `LIVE-FREE-004`: Cloudflare Workers AI test, if owner configures account ID, token and model.

Live tests must record date, region if known, provider/model ID, sanitized outcome and latency. Never claim they passed when skipped. A live test is not a CI requirement when credentials are absent. A successful live test is evidence of availability at that time only, not a future guarantee.

---

# 8. SQLite schema and persistence

SQLite is the sole source of truth for application configuration and operational metadata. WAL mode is required. Use one logical writer queue; readers may use separate read-only connections.

## 8.1 Database requirements

- Select a CGO-free Go SQLite driver only after Phase 0 proves Windows and Linux behaviour, backup, migrations and concurrency.
- Record exact driver/version/licence.
- Foreign keys ON, busy timeout explicit, WAL enabled.
- Bounded transactions; never hold a write transaction during network calls.
- Immutable migration IDs and checksums.
- Back up before schema migration.
- SQLite is not a secrets vault.

## 8.2 Required logical tables

### `schema_migrations`
`migration_id` PK, `checksum`, `applied_at`.

### `settings`
`key` PK, `value_json`, `updated_at`. No secret values.

### `providers`
`id` PK, `provider_type`, `display_name`, `base_url`, `enabled`, `created_at`, `updated_at`, validated non-secret `metadata_json`.

### `credentials`
`id` PK, `provider_id` FK, `label`, `secret_ref`, `state`, `last_test_at`, `last_test_result`, `last_error_category`, timestamps. States: `untested`, `valid`, `invalid`, `quota_exhausted`, `disabled`. Never store raw/reversibly encrypted provider keys.

### `models`
`id` PK, `provider_id` FK, `provider_model_id`, `display_name`, `context_window` nullable, `input_price` nullable, `output_price` nullable, `currency` nullable, `pricing_status` (`verified`, `unknown`, `stale`), `access_status` (`verified_free`, `free_plan`, `paid`, `unknown`, `stale`), `source_url`, `source_checked_at`, `enabled`, timestamps.

### `deployments`
`id` PK, `slot` unique A/B/C, `provider_id` FK, `credential_id` FK, `model_id` FK, `enabled`, timestamps. Validate provider/credential/model relationships agree.

### `routing_policies`
`id` PK, `public_alias` unique, `ordered_slots_json`, `request_deadline_ms`, `connect_timeout_ms`, `first_token_timeout_ms`, `attempt_timeout_ms`, `same_slot_retries`, `cooldown_enabled`, `cooldown_config_json`, `updated_at`. Exactly one active public alias in this release.

### `config_generations`
`generation` integer PK, `status` (`candidate`, `active`, `last_known_good`, `failed`, `rolled_back`), `config_hash`, `created_at`, `activated_at`, `failure_reason`, `source_revision`. No secrets.

### `apply_runs`
`id` PK, `candidate_generation`, `previous_generation`, `status`, `started_at`, `finished_at`, `health_result_json`, `failure_category`, `initiated_by`.

### `request_events`
`event_id` unique PK, `request_id`, timestamps, `public_alias`, `streaming`, `final_status`, `final_error_category`, `partial_content_sent`, `duration_ms`, `generation`. No content.

### `attempt_events`
`event_id` unique PK, `request_id`, `attempt_number`, `slot`, `provider_id`, `model_id`, `credential_id` internal ID only, timestamps, `http_status`, `error_category`, `duration_ms`, `selected_or_skipped_reason`. No response body or secret.

### `spool_checkpoints`
`spool_file`, `byte_offset`, `last_event_id`, `updated_at`, composite PK.

### `spool_quarantine`
`id` PK, `spool_file`, `line_number`, `reason`, `quarantined_at`. Store bounded sanitized metadata only.

### `sessions`
`session_id_hash` PK, `created_at`, `expires_at`, `last_seen_at`, `revoked_at`.

### `login_codes`
`code_hash` PK, `created_at`, `expires_at`, `consumed_at`.

### `privacy_acknowledgements`
`id` PK, `policy_version`, `acknowledged_at`, `scope`.

### `system_counters`
`counter_key` PK, `counter_value`, `updated_at`. Includes spool backlog, event loss, dropped diagnostics and restart counters.

## 8.3 Constraints and indexes

- Foreign keys enabled and indexed.
- Unique request/attempt event IDs.
- Index event timestamps and request IDs.
- Index credential state and generation status.
- Unique deployment slot.
- Enum and nonnegative duration checks.
- Validate all foreign-key relationships before apply.

## 8.4 Single writer and event ingestion

- All DB writes pass through one logical writer.
- Gateway and supervisor append sanitized events to JSONL spool.
- A single ingestor imports events transactionally.
- Stable event IDs; UUIDv7 or documented collision-resistant alternative.
- Ingestion idempotent.
- Checkpoint advances only after DB transaction commits.
- No DB write from request handler may block inference on telemetry failure.

## 8.5 Retention

Defaults:
- Events: 30 days OR 100,000 request events, whichever occurs first.
- Spool: 10 MiB × 5 segments.
- Logs: 10 MiB × 5 rotated files per component.

Do not delete un-ingested events unless loss is explicitly counted and visible. On disk full, keep inference alive, increment an in-memory loss counter, attempt bounded recovery, and persist the counter when storage becomes available. Never claim zero event loss if events were dropped.

## 8.6 Spool recovery

- One JSON object per line.
- Stable event IDs.
- Startup scans from last committed checkpoint.
- Recover incomplete trailing line safely.
- Quarantine malformed interior lines and continue where safe.
- Duplicate IDs do not duplicate DB rows.
- Rotation preserves checkpoint correctness.
- Locked files/disk full use bounded retries and visible counters.
- No prompts, completions, tool args, credentials or auth headers.

## 8.7 Backup and restore

- Use SQLite online backup API; never naïvely copy a live DB while WAL is active.
- Manifest includes app version, timestamp, schema version and checksum.
- Secrets are not in backup; credential references must be re-established if unavailable on target OS account.
- Restore requires confirmation and creates pre-restore backup.
- Validate integrity/schema/generation before replacing working state.
- Missing credential references must produce explicit degraded status, not silent success.

---

# 9. Config generation and atomic apply

SQLite is authoritative. LiteLLM YAML is generated output; never hand-edit it.

Apply sequence:
1. Validate inputs and authorization.
2. Validate provider URLs, credential refs, model IDs and deployment relationships.
3. Create candidate generation.
4. Resolve secret refs without persisting secret values.
5. Generate config in private temporary directory. YAML may contain env-variable references, never key values.
6. Validate YAML and run pinned LiteLLM config validation/startup check.
7. Start/reload candidate using a pinned-version-supported mechanism.
8. Run local readiness checks and fake-upstream smoke tests where appropriate.
9. Atomically activate only after health checks pass.
10. Mark previous generation last-known-good.
11. On failure, restore previous generation and verify it is live.
12. Record sanitized apply history.

Requirements:
- Write temp + fsync + atomic rename where supported.
- Never mutate active config in place.
- Crash during apply must boot last-known-good.
- Never remove last-known-good until a later generation is confirmed.
- Never silently change pinned model IDs.
- Discovery recommendations never apply automatically.

---

# 10. Providers, credentials and model discovery

## 10.1 Provider configuration

Provider-specific required fields must be defined from the pinned LiteLLM integration documentation and verified by Phase 0. Support OpenRouter, DeepSeek, Qwen/DashScope, Groq, Google AI Studio and optional Cloudflare where the pinned LiteLLM release supports them. Do not assume one generic key field fits every provider.

## 10.2 Credential state machine

States: `untested`, `valid`, `invalid`, `quota_exhausted`, `disabled`.

- Only valid credentials may back enabled production deployments.
- New credentials start untested.
- Successful explicit test may set valid.
- Verified 401/403 sets invalid.
- Verified quota/billing exhaustion sets quota_exhausted.
- Owner disable sets disabled.
- Invalid/quota-exhausted credentials are ineligible until successful explicit retest.
- Network/TLS/DNS failure alone must not invalidate a previously valid credential.
- Production traffic must never automatically re-test credentials.
- State transitions serialized and audited.

## 10.3 Credential test

- Explicit user action only.
- Explain that tests may incur charges or consume free quota.
- Minimal non-sensitive request.
- Never send real user prompts or conversation history.
- Record timestamp, latency and normalized outcome only.
- Do not store raw response body.
- Distinguish auth/quota response from local network failure.

## 10.4 Discovery and pricing

- Optional and rate-limited.
- Prefer official provider APIs/documentation.
- Record source URL and checked timestamp.
- Never infer price, context window, tool support, region or streaming support from model name.
- “Free” only when applicable input and output token prices are verified zero and eligibility conditions are displayed.
- Unknown/stale is not free.
- No silent pinned model changes.
- Discovery failure must not affect inference.

## 10.5 Provider data handling

For each provider show configured endpoint/region where known, current verified data-handling source, last checked timestamp, unknown/stale status and fallback order. Never claim data residency, retention or training policy without current evidence. Clearly warn that fallback can send content to another provider.

---

# 11. Telemetry, logs and privacy

Capture only operational metadata:
- Request ID and attempt IDs.
- Timestamps/durations.
- Alias and config generation.
- Slot/provider/model internal IDs.
- HTTP status and normalized error category.
- Streaming and commit state.
- Fallback outcome.
- Sanitized process/provider latency.

Never capture prompts, completions, tool schemas/arguments/results, source code, auth headers, secrets or raw upstream bodies.

Metrics:
- Request count and success/failure.
- Per-slot attempts and fallback frequency.
- Provider latency/timeouts.
- Credential states.
- Process health and active generation.
- Spool backlog/event loss.
- Last ingestion timestamp and freshness.

Logs:
- Structured, bounded and rotated.
- Redact before writing.
- No request/response bodies.
- Diagnostics export requires preview and redaction.
- Never include secrets, sessions, prompts or completions.

---

# 12. Control API contract

Binds to `127.0.0.1:8369`. Versioned routes under `/api/v1/`. State-changing actions require admin authentication and CSRF protection.

Mandatory capability groups:
- System status, readiness, diagnostics.
- Providers CRUD and enable/disable.
- Credentials add/test/rotate/disable/remove.
- Models list/add/pin/disable/discovery.
- Deployments A/B/C assign/reorder/enable/disable.
- Routing read/configure.
- Apply validate/run/history/rollback.
- Events list/filter/export.
- Privacy/provider data handling.
- Backup create/validate/restore.
- Sessions login/logout/revoke.
- Client config preview/apply/rollback.

Use strict schemas, stable machine-readable errors, request limits and sanitized details. Never return raw secrets. Destructive actions require confirmation.

Suggested error:

```json
{
  "error": {
    "code": "validation_failed",
    "message": "The deployment configuration is invalid.",
    "request_id": "correlation-id",
    "details": []
  }
}
```

---

# 13. CLI contract

Binary: `opentrinity`

Required commands/capabilities:

```text
opentrinity init
opentrinity start
opentrinity stop
opentrinity restart
opentrinity status
opentrinity doctor
opentrinity providers list|add|edit|enable|disable|remove
opentrinity credentials add|test|rotate|disable|remove
opentrinity models list|add|pin|disable|discover
opentrinity deployments list|set|enable|disable
opentrinity routing show|configure
opentrinity gateway key show|rotate
opentrinity apply validate|run|history|rollback
opentrinity events list|export
opentrinity logs
opentrinity diagnostics
opentrinity backup create|validate|restore
opentrinity connect <client>
opentrinity update
opentrinity uninstall
```

Rules:
- Never accept secrets as command-line arguments.
- Use secure prompt or protected input.
- One-time reveal only with explicit confirmation.
- `--json` where useful.
- CLI remains useful if dashboard/control plane is down for safe status/start/stop/log/diagnostic operations.
- Destructive actions require confirmation.

Exit codes:
`0` success; `1` general; `2` usage; `3` validation; `4` not found; `5` auth; `6` conflict; `7` confirmation required; `10` gateway unhealthy; `11` provider test failed; `12` partial success; `20` control plane unavailable; `30` unsupported.

---

# 14. Dashboard

Use Templ + HTMX + semantic HTML + CSS. No large frontend framework.

Required pages:
1. Overview.
2. Providers — includes Add Free Provider guidance.
3. Credentials.
4. Models — free/paid/unknown/stale filters and pricing provenance.
5. Deployments — A/B/C order and provider/model/credential.
6. Routing — alias, timeout, retries, verified cooldown.
7. Activity — sanitized events.
8. Privacy — provider data handling and fallback order.
9. Diagnostics — processes, ports, DB, spool, generation.
10. Settings — gateway key, sessions, limits, retention.
11. Apply history — generations and rollback.
12. Client connection — base URL, alias, secure key handling.

UX:
- Free models show exact status: Verified free, Free-plan access, Unknown or Stale. Do not use a simple “Free” badge for unknown eligibility.
- Show rate-limit and policy warnings.
- Distinguish dynamic upstream router from pinned model.
- Show provider order before applying changes.
- State-changing actions require authenticated POST + CSRF.
- Raw provider keys are never shown after initial entry.
- Accessible labels, keyboard navigation and visible error states.

---

# 15. Client integration helpers

A client is supported only after testing its exact version and config format.

For each client:
- Detect or ask config path.
- Read and back up existing config.
- Parse safely; do not regex-rewrite arbitrary JSON/YAML.
- Preview exact diff.
- Preserve unrelated settings.
- Require confirmation.
- Apply atomically and validate.
- Provide rollback.

Gateway key handling:
- Prefer the client's supported OS credential store or secret reference.
- If the client requires a plaintext key in its config file, warn before writing, show exact file path and permissions, require explicit confirmation, and never claim the key remains protected by OPENTRINITY's credential store.
- Do not silently write a secret to a shared/world-readable file.
- Never place key in shell history or URL.
- Clients on another machine cannot use this loopback endpoint without a separately designed secure remote-access feature; do not add one in this release.

---

# 16. Optional MCP and agent automation

MCP is deferred until core stable:
- Thin stdio server calling authenticated control API.
- No direct SQLite or secret access.
- Destructive changes require confirmation.
- MCP shutdown must not affect gateway.

Future AI agent automation:
- Opt-in and disabled by default.
- Agents propose typed, schema-validated changes only.
- Owner approval required before apply.
- Sensitive changes always require explicit approval.
- Kill switch and audit trail mandatory.
- No autonomous key rotation, model substitution or provider routing change.

---

# 17. Phase plan and gates

## Phase 0 — Feasibility spikes (no product feature code)

Prove and record:
1. Go toolchain and CGO-free SQLite driver on Windows 11.
2. Same driver on supported Linux environment.
3. SQLite WAL, backup, crash recovery and migration behaviour.
4. Templ + HTMX rendering and SSE pass-through.
5. Exact pinned LiteLLM version and licence.
6. Exact LiteLLM config proving A→B→C with fake upstream counters.
7. Streaming fallback before commit and no fallback after commit.
8. Tool-call and streamed tool-call support.
9. Retry, cooldown and timeout semantics; disable unproven settings.
10. LiteLLM database and telemetry disabled; prove no hidden dependency.
11. Windows Job Object and Linux process group containment.
12. Windows Credential Manager and Linux Secret Service/fallback.
13. Exact ingress allowlist and limits.
14. Outbound network observation with fake upstreams.
15. Port separation 3888/3889/8369.
16. Provider-specific LiteLLM model syntax and credential variable mapping for each initial provider integration.
17. At least one verified free-tier config pattern using a placeholder/fake upstream; optional live check only with owner key.

Phase 0 report must include exact versions, commands, outputs, full working LiteLLM config with placeholder secret references, actual fake upstream counters, limitations and go/no-go. Never include real secrets in the report.

**Gate:** All mandatory spikes pass or a required design decision is escalated. No feature code before Phase 0 gate.

## Phase 1 — Foundation, config and SQLite

Go module, repository structure, logging/redaction, migrations, WAL, writer queue, schema, credential abstraction, config generation and generations.

Gate: fresh DB, migrations, integrity, crash recovery, secret storage and no-leak tests.

## Phase 2 — Supervisor, process lifecycle and spool

Singleton, child lifecycle, readiness, backoff, containment, JSONL spool, ingestion, rotation, quarantine and diagnostics.

Gate: process kill/restart, outage isolation, spool replay, duplicate handling, corrupt line and disk-full tests.

## Phase 3 — Ingress gateway and private LiteLLM

Ingress on 3888, LiteLLM on 3889, authentication, allowlist, limits, cancellation, readiness and snapshot reload.

Gate: unauthorized requests cause zero upstream calls; unsupported routes blocked; no public listener; stream pass-through works.

## Phase 4 — Providers, credentials, free-tier support and deployments

Provider CRUD, credential state machine, secure storage, model records, A/B/C config, SSRF validation, provider tests and free-tier metadata.

Gate: credential matrix, URL validation, deployment integrity and fake-provider config tests.

## Phase 5 — Non-streaming routing and fallback

Alias config, sequential fallback, error mapping and sanitized telemetry.

Gate: full fake-upstream matrix proves order and attempt count; source audit finds no Go routing/retry.

## Phase 6 — Streaming and tools

SSE, commit-point behaviour, cancellation, tool-call pass-through and streamed deltas.

Gate: pre-commit fallback, post-commit no-fallback, disconnect and tool-call matrices.

## Phase 7 — Control API and CLI (Milestone M1)

Versioned API, sessions, CSRF, core CLI, apply/rollback, status/diagnostics and event query.

M1 only when:
- All three processes on correct loopback ports.
- Health and readiness correct.
- Completion through `nenifix-coder` works with fake upstream and, if key available, optional real provider.
- A→B→C proven with visible counters.
- Streaming commit rule proven.
- Tool calls proven.
- Core CLI works.
- Control-plane crash does not stop gateway/LiteLLM.
- Secret canary scan passes.
- Atomic apply/rollback passes.

## Phase 8 — Dashboard

All pages and authenticated state-changing flows.

Gate: browser tests for navigation, validation, CSRF, sessions, provider/deployment changes, apply and rollback.

## Phase 9 — Discovery, privacy and client integration

Discovery, provenance, provider privacy, free model listing and client helpers.

Gate: no silent model changes; pricing provenance correct; client diff/backup/rollback passes.

## Phase 10 — Installers and operations

Windows per-user installer, Linux per-user installer, optional autostart, update/rollback, backup/restore and uninstall.

Gate: fresh install, upgrade, rollback, restore and uninstall tests; Linux smoke test.

## Phase 11 — Security, hardening and release

Full CORE suite, security review, dependency/licence audit, diagnostics review and release checklist.

Gate: all release criteria pass. Owner approval required before publication.

---

# 18. Milestone M1 proof

M1 is valid only with evidence:
1. Supervisor starts gateway, control and LiteLLM on correct loopback ports.
2. `/health` and `/ready` semantics verified.
3. `POST /v1/chat/completions` with alias succeeds through LiteLLM.
4. A→B→C proven with actual fake upstream counters.
5. Streaming commit rule proven.
6. Non-stream and streamed tool calls pass.
7. Core CLI works.
8. Killing control process does not stop gateway or LiteLLM.
9. Secret canary scan finds no secret on forbidden surfaces.
10. Atomic apply and automatic rollback proven.
11. Every relevant CORE test has PASS/FAIL/SKIP and evidence.

A report that says “M1 complete” without evidence is invalid.

---

# 19. Fake upstream test harness

Build deterministic fake upstreams with:
- Independent A/B/C request counters.
- Configurable status/body.
- Connection refused/reset.
- Delayed connect, headers, first byte and chunks.
- Empty/malformed success payload.
- 401/403, 404, 429, quota, 5xx and 529.
- SSE role-only first chunk.
- Stream failure before/after first body byte.
- Client disconnect.
- Tool-call and streamed tool-call chunks.
- Test-only request capture to assert body unchanged.

Every routing test prints/asserts actual A/B/C counter values and verifies event outcomes. Do not use real provider services to prove deterministic routing.

---

# 20. CORE acceptance tests

Every phase report must include each applicable ID with `PASS`, `FAIL` or `SKIP`, evidence and skip reason.

## Routing
- `CORE-ROUTE-001`: A succeeds; counters A=1 B=0 C=0.
- `CORE-ROUTE-002`: A eligible failure, B succeeds; C=0.
- `CORE-ROUTE-003`: A and B fail, C succeeds.
- `CORE-ROUTE-004`: all fail; sanitized aggregate error.
- `CORE-ROUTE-005`: invalid alias; zero upstream requests.
- `CORE-ROUTE-006`: unsupported path/method; zero upstream requests.
- `CORE-ROUTE-007`: client 400 does not fallback.
- `CORE-ROUTE-008`: overall deadline prevents next attempt.
- `CORE-ROUTE-009`: same-slot retry exactly zero.
- `CORE-ROUTE-010`: duplicate credential is not retried after invalidation.
- `CORE-ROUTE-011`: quota-exhausted credential ineligible until retest.
- `CORE-ROUTE-012`: unverified 404 not misclassified as model missing.
- `CORE-ROUTE-013`: cooldown matches pinned evidence or remains disabled.

## Streaming and tools
- `CORE-STREAM-001`: pre-commit eligible failure permits configured LiteLLM fallback.
- `CORE-STREAM-002`: role-only SSE event commits; no later fallback.
- `CORE-STREAM-003`: first content chunk commits; no later fallback.
- `CORE-STREAM-004`: disconnect cancels upstream; no fallback.
- `CORE-STREAM-005`: stream not fully buffered.
- `CORE-STREAM-006`: headers/chunk order preserved.
- `CORE-TOOL-001`: non-stream tool call passes through.
- `CORE-TOOL-002`: streamed tool-call deltas pass through.
- `CORE-TOOL-003`: tool arguments absent from telemetry.
- `CORE-TOOL-004`: fallback before commit works with tool-call request.

## Security
- `CORE-SEC-001`: missing gateway key rejected before upstream.
- `CORE-SEC-002`: invalid gateway key rejected before upstream.
- `CORE-SEC-003`: gateway key absent from logs/events/DB/YAML/argv.
- `CORE-SEC-004`: provider key absent from logs/events/DB/YAML/argv.
- `CORE-SEC-005`: admin session required for state changes.
- `CORE-SEC-006`: CSRF/invalid Origin rejected.
- `CORE-SEC-007`: Host validation and DNS rebinding protection work.
- `CORE-SEC-008`: SSRF destinations rejected.
- `CORE-SEC-009`: oversized body rejected before upstream.
- `CORE-SEC-010`: concurrency limits enforced.
- `CORE-SEC-011`: ingress cannot reach LiteLLM admin routes.
- `CORE-SEC-012`: unvalidated redirect rejected.
- `CORE-SEC-013`: diagnostics export redacted.

## Data, spool and apply
- `CORE-DATA-001`: fresh migrations pass.
- `CORE-DATA-002`: migration checksum mismatch fails safely.
- `CORE-DATA-003`: WAL crash recovery preserves committed data.
- `CORE-DATA-004`: single writer serializes writes.
- `CORE-DATA-005`: duplicate spool IDs ingested once.
- `CORE-DATA-006`: incomplete trailing line recovered.
- `CORE-DATA-007`: corrupt interior line quarantined; later events processed.
- `CORE-DATA-008`: disk full increments visible loss/backpressure counters while inference remains available.
- `CORE-DATA-009`: backup/restore preserves valid state.
- `CORE-DATA-010`: missing credential refs on restore fail/degrade explicitly.
- `CORE-APPLY-001`: valid candidate applies atomically.
- `CORE-APPLY-002`: invalid candidate never replaces active generation.
- `CORE-APPLY-003`: failed health check restores last-known-good.
- `CORE-APPLY-004`: crash during apply boots last-known-good.

## Process and operations
- `CORE-PROC-001`: one supervisor instance.
- `CORE-PROC-002`: supervisor exit terminates children.
- `CORE-PROC-003`: control crash does not stop gateway/LiteLLM.
- `CORE-PROC-004`: gateway crash restarts gateway independently.
- `CORE-PROC-005`: LiteLLM crash marks readiness false and recovers.
- `CORE-PROC-006`: PID reuse does not kill unrelated process.
- `CORE-PROC-007`: port conflict reports owner and fails safely.
- `CORE-PROC-008`: restart backoff prevents tight loop.
- `CORE-OPS-001`: fresh Windows per-user install without admin.
- `CORE-OPS-002`: upgrade preserves DB/secrets.
- `CORE-OPS-003`: rollback restores previous executable/config.
- `CORE-OPS-004`: uninstall offers preserve-data and purge choices.
- `CORE-OPS-005`: Linux user-level smoke test.

## Free-tier capability
- `CORE-FREE-001`: provider can be configured and validated using fake upstream.
- `CORE-FREE-002`: free-tier model can be assigned Slot A.
- `CORE-FREE-003`: free-tier model in Slot B/C participates in LiteLLM fallback.
- `CORE-FREE-004`: free-tier key absent from forbidden surfaces.
- `CORE-FREE-005`: UI status verified-free/free-plan/unknown/stale is correct.
- `CORE-FREE-006`: rate-limit, policy and fallback data warnings visible.
- `CORE-FREE-007`: dynamic router distinguished from pinned model.
- `CORE-FREE-008`: discovery never silently changes pinned deployment.

Live provider tests are reported separately as `LIVE-FREE-*`. Skip if no owner key is available; never fake live evidence.

---

# 21. Repository structure (recommended)

Adapt to the existing repository after inspection; do not force destructive restructuring.

```text
opentrinity/
  cmd/
    opentrinity/
    supervisor/
    gateway/
    control/
  internal/
    config/
    database/
    migrations/
    secrets/
    supervisor/
    gateway/
    control/
    auth/
    providers/
    deployments/
    routingconfig/
    events/
    spool/
    telemetry/
    diagnostics/
    backup/
    platform/
      windows/
      linux/
  web/
    templates/
    static/
  tests/
    fakeupstream/
    integration/
    acceptance/
  scripts/
  docs/
  go.mod
  README.md
```

`routingconfig` generates LiteLLM config only; no runtime routing logic.

---

# 22. Installation and operations

## Windows
- Per-user binaries: `%LOCALAPPDATA%\\Nenifix\\OPENTRINITY\\bin`
- Data: `%LOCALAPPDATA%\\Nenifix\\OPENTRINITY`
- Optional user PATH and Scheduled Task.
- No admin required.
- Checksums published with approved releases; never recommend `irm | iex`.

## Linux
- Binaries: `~/.local/bin` or `~/.opentrinity/bin`.
- Data: `~/.opentrinity`.
- Optional `systemd --user`.

## Update and rollback
- Verify checksums.
- Keep previous version for rollback.
- Never silently upgrade pinned LiteLLM.
- Preserve DB and secrets.
- Back up before migration.

## Uninstall
- Default removes program files, PATH and autostart only.
- Preserve user data.
- `--purge` required to delete data, with explicit confirmation.
- Never touch unrelated files.

## Backup
- SQLite online backup + manifest.
- Secrets excluded.
- Restore validates integrity and creates pre-restore safety copy.

---

# 23. Release definition of done

All conditions require real evidence:

1. Fresh Windows 11 per-user install works without admin.
2. Linux user-level smoke test passes.
3. All processes bind only to loopback.
4. Ports are 3888 / 3889 / 8369.
5. LiteLLM is sole inference router; source audit confirms no Go routing/fallback.
6. A→B→C proven with fake-upstream counters.
7. Retry/cooldown behaviour matches pinned LiteLLM evidence or remains disabled.
8. Streaming commit-point/no-midstream-fallback tests pass.
9. Tool-call and streamed tool-call tests pass.
10. Gateway survives control-plane failure.
11. Process containment and PID identity checks pass.
12. Credential state machine and OS secret storage pass.
13. Secret canary scan finds no leakage.
14. SSRF, DNS rebinding, CSRF, Host/Origin, body and concurrency tests pass.
15. SQLite migrations, backup/restore and WAL recovery pass.
16. Spool recovery, quarantine, replay and disk-full tests pass.
17. Atomic apply/rollback passes.
18. Client helpers preserve unrelated settings and support rollback.
19. Installer, updater, rollback and uninstaller pass.
20. Free-tier capability tests `CORE-FREE-001…008` pass.
21. At least one optional live free-provider test is performed only if the owner supplies a key and requests it; otherwise mark it SKIP and do not claim live provider verification.
22. README contains compatibility statement and free-tier warnings.
23. Every phase report contains real commands, output and all applicable CORE test IDs.
24. Owner explicitly approves release.

## 23A. README compatibility statement

```markdown
## Compatibility

OPENTRINITY provides an OpenAI-compatible local Chat Completions endpoint.
It works with clients that allow a custom base URL, bearer key and model name
and call the endpoint from the same machine.

Verified:
- [Exact client names and versions that passed real tests]

Best-effort / untested:
- [List]

Unsupported:
- Clients that hard-code vendor endpoints or route traffic through vendor cloud
- Anthropic Messages API
- OpenAI Responses API
- Embeddings and image generation
- Any claim that it works with every AI app
```

## 23B. Owner acceptance checklist

- [ ] Fresh Windows install works without admin.
- [ ] `opentrinity status` shows gateway, LiteLLM and control plane healthy.
- [ ] Real chat completion through `nenifix-coder` succeeds (or fake upstream used for deterministic acceptance).
- [ ] A→B→C proven with visible counters.
- [ ] Streaming works and does not fallback after first byte.
- [ ] Tool calls work.
- [ ] Control plane can be stopped without stopping gateway.
- [ ] No secrets visible in logs, DB, YAML or diagnostics.
- [ ] Free-tier configuration and UI labels work.
- [ ] Any live free-provider claim has dated evidence.
- [ ] Uninstall without `--purge` preserves user data.
- [ ] All phase reports exist with real evidence.
- [ ] Compatibility statement reviewed.

Only owner may approve release.

---

# 24. Do-not-invent list

- No Go routing, retries, fallback, cooldown or provider selection.
- No second public alias.
- No Docker, Kubernetes, Redis, PostgreSQL or second DB.
- No local model downloads or GPU requirement.
- No public tunnels or LAN binding.
- No secrets in SQLite, YAML values, logs, events, HTML, URLs, argv or source.
- No embedded/shared/shipped free-tier API keys.
- No guaranteed free quota, permanent free model list or uptime promise.
- No dynamic free router described as a pinned model.
- No automatic model replacement or silent provider change.
- No unverified pricing, capability, data residency or “free” claims.
- No arbitrary provider URL fetches.
- No raw prompts, completions, tool args or source code in telemetry.
- No unsupported LiteLLM enterprise feature assumptions.
- No fabricated tests or compatibility claims.
- No autonomous sensitive agent changes.
- No destructive install/update/uninstall without confirmation and backup.
- No fallback claim without actual per-slot counters.

---

# 25. Phase report template

```text
Phase:
Date:
Git commit / working tree state:
Scope completed:
Files created/changed:
Dependency versions:
Commands executed:
Tests executed:
CORE test results (ID + PASS/FAIL/SKIP + evidence):
  CORE-ROUTE-001: PASS — counters A=1 B=0 C=0
  ...
LIVE-FREE test results (if attempted):
Manual verification:
Security observations:
Known limitations:
Open decisions requiring owner:
Evidence/log locations:
Gate result: PASS / FAIL / BLOCKED
```

A phase passes only when every required test for that phase passes. A skipped test must have a reason and may not be used to claim the property was verified.

---

# 26. First action and build order

Start immediately with repository inspection and Phase 0.

1. Inspect repository, branch, working tree and tests. Preserve existing work.
2. Record Go, OS, LiteLLM and dependency versions.
3. Execute Phase 0 spikes.
4. Paste exact working LiteLLM config with placeholder secret references, not real secrets.
5. Show actual fake upstream counters for A/B/C.
6. Prove stream commit-point behaviour and tool calls.
7. Prove at least one free-tier provider configuration pattern using a fake upstream; perform a live provider test only if owner key is available and authorized.
8. Produce Phase 0 report.
9. If gate passes, continue Phase 1 through Phase 11 automatically.
10. If a hard blocker occurs, stop with evidence and the smallest required owner decision.
11. After Phase 11, present the Owner Acceptance Checklist. Do not publish without approval.

**Build the product according to this document.**

---

**OPENTRINITY / NENIFIX — Final Build Prompt v1.6.6**  
Go-only · LiteLLM-only routing · Free-tier capable · Local-first
