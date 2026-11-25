#!/usr/bin/env bash
# End-to-end MVP acceptance: real Go CLI, real supervisor, real LiteLLM,
# deterministic fake upstreams with observable A/B/C counters.
set -uo pipefail

ROOT=$(cd "$(dirname "$0")/../.." && pwd)
if command -v cygpath >/dev/null 2>&1; then
  ROOT_WIN=$(cygpath -m "$ROOT")
else
  ROOT_WIN="$ROOT"
fi

BIN="$ROOT/dist/opentrinity.exe"
FAKE="$ROOT/tests/fakeupstream/fakeupstream.exe"
UD="$ROOT/.acceptance-userdata"
EVIDENCE="$ROOT/docs/PHASE_REPORTS/evidence"
SUMMARY="$EVIDENCE/acceptance-summary.txt"
LITELLM="$ROOT_WIN/.phase0/venv2/Scripts/litellm.exe"

mkdir -p "$EVIDENCE"
rm -rf "$UD"
mkdir -p "$UD"

export OT_USER_DATA="$UD"
export OT_LITELLM_EXE="$LITELLM"

FAKE_PID=""
cleanup() {
  if [ -n "$FAKE_PID" ]; then kill "$FAKE_PID" 2>/dev/null || true; fi
}
trap cleanup EXIT

fail=0
check() {
  local label="$1" got="$2" want="$3"
  if [ "$got" = "$want" ]; then
    echo "PASS $label (got=$got)" | tee -a "$SUMMARY"
  else
    echo "FAIL $label (got=$got want=$want)" | tee -a "$SUMMARY"
    fail=1
  fi
}

echo "OPENTRINITY MVP acceptance $(date -u +%Y-%m-%dT%H:%M:%SZ)" > "$SUMMARY"
echo "root=$ROOT" >> "$SUMMARY"

echo "== build =="
CGO_ENABLED=0 go build -o "$BIN" ./cmd/opentrinity || exit 1

echo "== start fake upstreams =="
"$FAKE" -ports 11001,11002,11003 > "$EVIDENCE/fakeupstream.log" 2>&1 &
FAKE_PID=$!
for p in 11001 11002 11003; do
  for i in 1 2 3 4 5 6 7 8 9 10; do
    curl -sf "http://127.0.0.1:$p/health" >/dev/null && break
    sleep 0.2
  done
done

echo "== initialize and configure =="
"$BIN" init || exit 1
"$BIN" providers add fake-a http://127.0.0.1:11001/v1 || exit 1
"$BIN" providers add fake-b http://127.0.0.1:11002/v1 || exit 1
"$BIN" providers add fake-c http://127.0.0.1:11003/v1 || exit 1
printf 'phase0-fake-key-a\n' | "$BIN" credentials stdin 1 primary >/dev/null || exit 1
printf 'phase0-fake-key-b\n' | "$BIN" credentials stdin 2 primary >/dev/null || exit 1
printf 'phase0-fake-key-c\n' | "$BIN" credentials stdin 3 primary >/dev/null || exit 1
"$BIN" models add 1 fake-model fake-a >/dev/null || exit 1
"$BIN" models add 2 fake-model fake-b >/dev/null || exit 1
"$BIN" models add 3 fake-model fake-c >/dev/null || exit 1
"$BIN" deployments set A 1 >/dev/null || exit 1
"$BIN" deployments set B 2 >/dev/null || exit 1
"$BIN" deployments set C 3 >/dev/null || exit 1
"$BIN" apply validate || exit 1
"$BIN" apply run || exit 1
"$BIN" start || exit 1

echo "== wait for readiness =="
READY=0
for i in $(seq 1 120); do
  if curl -sf http://127.0.0.1:3888/ready >/dev/null; then READY=1; break; fi
  sleep 0.5
done
check "gateway-ready" "$READY" "1"
if [ "$READY" != "1" ]; then
  echo "supervisor log tail:" >> "$SUMMARY"
  tail -40 "$UD/logs/supervisor.log" >> "$SUMMARY" 2>/dev/null || true
  echo "litellm log tail:" >> "$SUMMARY"
  tail -40 "$UD/logs/litellm.log" >> "$SUMMARY" 2>/dev/null || true
  "$BIN" stop || true
  exit 1
fi

GATEWAY_KEY=$(printf 'SHOW\n' | "$BIN" gateway key show | tail -1)
if [ -z "$GATEWAY_KEY" ]; then echo "FAIL gateway-key-reveal" | tee -a "$SUMMARY"; exit 1; fi
AUTH="Authorization: Bearer $GATEWAY_KEY"

count() { curl -s "http://127.0.0.1:$1/_stats" | sed -n 's/.*"count":\([0-9]*\).*/\1/p'; }
set_behavior() { curl -s -X POST "http://127.0.0.1:$1/_config" -H 'Content-Type: application/json' -d "$2" >/dev/null; }
reset_slots() {
  for p in 11001 11002 11003; do
    curl -s -X POST "http://127.0.0.1:$p/_reset" >/dev/null
    set_behavior "$p" '{"status":0,"mode":"ok"}'
  done
}
counters() { echo "$(count 11001)/$(count 11002)/$(count 11003)"; }
chat() {
  curl -s -o /dev/null -w '%{http_code}' -X POST http://127.0.0.1:3888/v1/chat/completions \
    -H "$AUTH" -H 'Content-Type: application/json' \
    -d "{\"model\":\"$1\",\"messages\":[{\"role\":\"user\",\"content\":\"hello\"}]}"
}

echo "== routing matrix =="
reset_slots
STATUS=$(chat nenifix-coder)
check "ROUTE-001-status" "$STATUS" "200"
check "ROUTE-001-counters" "$(counters)" "1/0/0"

reset_slots
set_behavior 11001 '{"status":500,"mode":"ok"}'
STATUS=$(chat nenifix-coder)
check "ROUTE-002-status" "$STATUS" "200"
check "ROUTE-002-counters" "$(counters)" "1/1/0"

reset_slots
set_behavior 11001 '{"status":500,"mode":"ok"}'
set_behavior 11002 '{"status":500,"mode":"ok"}'
STATUS=$(chat nenifix-coder)
check "ROUTE-003-status" "$STATUS" "200"
check "ROUTE-003-counters" "$(counters)" "1/1/1"

reset_slots
set_behavior 11001 '{"status":500,"mode":"ok"}'
set_behavior 11002 '{"status":500,"mode":"ok"}'
set_behavior 11003 '{"status":500,"mode":"ok"}'
STATUS=$(chat nenifix-coder)
check "ROUTE-004-status-sanitized-502" "$STATUS" "502"
check "ROUTE-004-counters" "$(counters)" "1/1/1"

reset_slots
STATUS=$(chat other-model)
check "ROUTE-005-invalid-alias-status" "$STATUS" "400"
check "ROUTE-005-invalid-alias-counters" "$(counters)" "0/0/0"

reset_slots
STATUS=$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:3888/admin)
check "ROUTE-006-unsupported-path-status" "$STATUS" "404"
check "ROUTE-006-unsupported-path-counters" "$(counters)" "0/0/0"

reset_slots
STATUS=$(curl -s -o /dev/null -w '%{http_code}' -X POST http://127.0.0.1:3888/v1/chat/completions \
  -H "$AUTH" -H 'Content-Type: application/json' -d '{not-json')
check "ROUTE-007-client-error-status" "$STATUS" "400"
check "ROUTE-007-client-error-counters" "$(counters)" "0/0/0"

reset_slots
STATUS=$(curl -s -o /dev/null -w '%{http_code}' -X POST http://127.0.0.1:3888/v1/chat/completions \
  -H 'Content-Type: application/json' -d '{"model":"nenifix-coder"}')
check "SEC-001-missing-key-status" "$STATUS" "401"
check "SEC-001-missing-key-counters" "$(counters)" "0/0/0"

reset_slots
STATUS=$(curl -s -o /dev/null -w '%{http_code}' -X POST http://127.0.0.1:3888/v1/chat/completions \
  -H 'Authorization: Bearer wrong-key' -H 'Content-Type: application/json' -d '{"model":"nenifix-coder"}')
check "SEC-002-invalid-key-status" "$STATUS" "401"
check "SEC-002-invalid-key-counters" "$(counters)" "0/0/0"

reset_slots
BIG_FILE="$EVIDENCE/oversized-body.json"
printf '{"model":"nenifix-coder","padding":"' > "$BIG_FILE"
head -c 11000000 /dev/zero | tr '\0' 'x' >> "$BIG_FILE"
printf '"}' >> "$BIG_FILE"
STATUS=$(curl -s -o /dev/null -w '%{http_code}' -X POST http://127.0.0.1:3888/v1/chat/completions \
  -H "$AUTH" -H 'Content-Type: application/json' --data-binary "@$BIG_FILE")
check "SEC-009-oversized-status" "$STATUS" "413"
check "SEC-009-oversized-counters" "$(counters)" "0/0/0"

reset_slots
STATUS=$(curl -s -o /dev/null -w '%{http_code}' -X POST http://127.0.0.1:3888/v1/chat/completions \
  -H "Host: evil.example:3888" -H "$AUTH" -H 'Content-Type: application/json' -d '{"model":"nenifix-coder"}')
check "SEC-host-validation-status" "$STATUS" "421"
check "SEC-host-validation-counters" "$(counters)" "0/0/0"

echo "== streaming =="
reset_slots
set_behavior 11001 '{"status":500,"mode":"ok"}'
STATUS=$(curl -s -o "$EVIDENCE/stream-precommit.txt" -w '%{http_code}' -N -X POST http://127.0.0.1:3888/v1/chat/completions \
  -H "$AUTH" -H 'Content-Type: application/json' -d '{"model":"nenifix-coder","stream":true,"messages":[{"role":"user","content":"hi"}]}')
check "STREAM-001-precommit-status" "$STATUS" "200"
check "STREAM-001-precommit-counters" "$(counters)" "1/1/0"
if grep -q 'data: \[DONE\]' "$EVIDENCE/stream-precommit.txt"; then
  echo "PASS STREAM-006-done-marker" | tee -a "$SUMMARY"
else
  echo "FAIL STREAM-006-done-marker" | tee -a "$SUMMARY"; fail=1
fi

reset_slots
set_behavior 11001 '{"mode":"sse_fail_after_content"}'
curl -s -o "$EVIDENCE/stream-postcommit.txt" -N -X POST http://127.0.0.1:3888/v1/chat/completions \
  -H "$AUTH" -H 'Content-Type: application/json' -d '{"model":"nenifix-coder","stream":true,"messages":[{"role":"user","content":"hi"}]}'
check "STREAM-003-postcommit-counters" "$(counters)" "1/0/0"

reset_slots
set_behavior 11001 '{"delay_ms":4000,"mode":"ok"}'
curl -s -o /dev/null --max-time 0.5 -X POST http://127.0.0.1:3888/v1/chat/completions \
  -H "$AUTH" -H 'Content-Type: application/json' -d '{"model":"nenifix-coder","messages":[{"role":"user","content":"hi"}]}' || true
check "ROUTE-008-deadline-counters" "$(counters)" "1/0/0"

echo "== tools =="
reset_slots
curl -s -o "$EVIDENCE/tool-response.json" -X POST http://127.0.0.1:3888/v1/chat/completions \
  -H "$AUTH" -H 'Content-Type: application/json' \
  -d '{"model":"nenifix-coder","messages":[{"role":"user","content":"weather"}],"tools":[{"type":"function","function":{"name":"get_weather","parameters":{"type":"object","properties":{"city":{"type":"string"}}}}}]}'
if grep -q '"tool_calls"' "$EVIDENCE/tool-response.json"; then
  echo "PASS TOOL-001-nonstream-pass-through" | tee -a "$SUMMARY"
else
  echo "FAIL TOOL-001-nonstream-pass-through" | tee -a "$SUMMARY"; fail=1
fi
check "TOOL-001-counters" "$(counters)" "1/0/0"

reset_slots
set_behavior 11001 '{"mode":"sse_tool_ok"}'
curl -sS -o "$EVIDENCE/stream-tool-response.txt" -N -X POST http://127.0.0.1:3888/v1/chat/completions \
  -H "$AUTH" -H 'Content-Type: application/json' \
  -d '{"model":"nenifix-coder","stream":true,"messages":[{"role":"user","content":"weather"}],"tools":[{"type":"function","function":{"name":"get_weather","parameters":{"type":"object","properties":{"city":{"type":"string"}}}}}]}'
check "TOOL-002-streamed-counters" "$(counters)" "1/0/0"
if grep -q '"tool_calls"' "$EVIDENCE/stream-tool-response.txt" && grep -q 'Accra' "$EVIDENCE/stream-tool-response.txt" && grep -q 'data: \[DONE\]' "$EVIDENCE/stream-tool-response.txt"; then
  echo "PASS TOOL-002-streamed-tool-deltas" | tee -a "$SUMMARY"
else
  echo "FAIL TOOL-002-streamed-tool-deltas" | tee -a "$SUMMARY"; fail=1
fi

echo "== process topology =="
PORTS=$(netstat -ano 2>/dev/null | grep -E '127\.0\.0\.1:(3888|3889|8369)\s' | grep LISTENING | wc -l | tr -d ' ')
check "PROC-loopback-listeners" "$PORTS" "3"
PUBLIC=$(netstat -ano 2>/dev/null | grep -E '0\.0\.0\.0:(3888|3889|8369)\s' | grep LISTENING | wc -l | tr -d ' ')
check "SEC-loopback-only" "$PUBLIC" "0"

echo "== process lifecycle =="
SECOND_START=$("$BIN" start 2>&1 || true)
if printf '%s' "$SECOND_START" | grep -q "already running"; then
  echo "PASS PROC-001-singleton" | tee -a "$SUMMARY"
else
  echo "FAIL PROC-001-singleton ($SECOND_START)" | tee -a "$SUMMARY"; fail=1
fi

state_pid() { sed -n "s/.*\"$1\": \([0-9]*\).*/\1/p" "$UD/state.json" | head -1; }
wait_new_pid() {
  local name="$1" old="$2" port="$3" path="${4:-/health}" new=""
  for i in $(seq 1 40); do
    new=$(state_pid "$name")
    if [ -n "$new" ] && [ "$new" != "$old" ] && curl -sf "http://127.0.0.1:$port$path" >/dev/null 2>&1; then
      echo "$new"; return 0
    fi
    sleep 0.5
  done
  echo "$new"; return 1
}

GATEWAY_OLD=$(state_pid gateway_pid)
taskkill //PID "$GATEWAY_OLD" //F >/dev/null 2>&1 || true
GATEWAY_NEW=$(wait_new_pid gateway_pid "$GATEWAY_OLD" 3888 || true)
if [ -n "$GATEWAY_NEW" ] && [ "$GATEWAY_NEW" != "$GATEWAY_OLD" ]; then
  echo "PASS PROC-004-gateway-independent-restart ($GATEWAY_OLD -> $GATEWAY_NEW)" | tee -a "$SUMMARY"
else
  echo "FAIL PROC-004-gateway-independent-restart" | tee -a "$SUMMARY"; fail=1
fi
if curl -sf http://127.0.0.1:3889/health/liveliness >/dev/null; then
  echo "PASS PROC-litellm-survives-gateway-restart" | tee -a "$SUMMARY"
else
  echo "FAIL PROC-litellm-survives-gateway-restart" | tee -a "$SUMMARY"; fail=1
fi

CONTROL_OLD=$(state_pid control_pid)
taskkill //PID "$CONTROL_OLD" //F >/dev/null 2>&1 || true
CONTROL_NEW=$(wait_new_pid control_pid "$CONTROL_OLD" 8369 || true)
if [ -n "$CONTROL_NEW" ] && [ "$CONTROL_NEW" != "$CONTROL_OLD" ]; then
  echo "PASS PROC-control-restart ($CONTROL_OLD -> $CONTROL_NEW)" | tee -a "$SUMMARY"
else
  echo "FAIL PROC-control-restart" | tee -a "$SUMMARY"; fail=1
fi
if curl -sf http://127.0.0.1:3888/health >/dev/null; then
  echo "PASS PROC-003-control-crash-does-not-stop-gateway" | tee -a "$SUMMARY"
else
  echo "FAIL PROC-003-control-crash-does-not-stop-gateway" | tee -a "$SUMMARY"; fail=1
fi

LITELLM_OLD=$(state_pid litellm_pid)
taskkill //PID "$LITELLM_OLD" //F >/dev/null 2>&1 || true
LITELLM_NEW=$(wait_new_pid litellm_pid "$LITELLM_OLD" 3889 /health/liveliness || true)
if [ -n "$LITELLM_NEW" ] && [ "$LITELLM_NEW" != "$LITELLM_OLD" ] && curl -sf http://127.0.0.1:3888/ready >/dev/null; then
  echo "PASS PROC-005-litellm-crash-recovery ($LITELLM_OLD -> $LITELLM_NEW)" | tee -a "$SUMMARY"
else
  echo "FAIL PROC-005-litellm-crash-recovery" | tee -a "$SUMMARY"; fail=1
fi

echo "== concurrency limit verified by go test ./internal/gateway =="

echo "== stop and secret canary =="
"$BIN" stop || true
sleep 1
LEAK=0
for needle in 'phase0-fake-key-a' 'phase0-fake-key-b' 'phase0-fake-key-c' "$GATEWAY_KEY"; do
  if grep -R -q -F "$needle" "$UD" "$EVIDENCE" 2>/dev/null; then LEAK=1; fi
done
check "SEC-canary-no-secret-on-disk" "$LEAK" "0"
if grep -R -q -F 'fake-upstream status' "$EVIDENCE" 2>/dev/null; then
  echo "FAIL SEC-no-raw-upstream-error-in-evidence" | tee -a "$SUMMARY"; fail=1
else
  echo "PASS SEC-no-raw-upstream-error-in-evidence" | tee -a "$SUMMARY"
fi

echo "== port conflict detection =="
HOLDER_PID=""
"$ROOT/.phase0/venv2/Scripts/python.exe" -c 'import socket,time; s=socket.socket(); s.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1); s.bind(("127.0.0.1",3888)); s.listen(); time.sleep(25)' >/dev/null 2>&1 &
HOLDER_PID=$!
sleep 1
CONFLICT_OUT=$("$BIN" start 2>&1 || true)
if printf '%s' "$CONFLICT_OUT" | grep -q "port 3888 required by gateway is already in use by PID"; then
  echo "PASS PROC-007-port-conflict-report" | tee -a "$SUMMARY"
else
  echo "FAIL PROC-007-port-conflict-report ($CONFLICT_OUT)" | tee -a "$SUMMARY"; fail=1
fi
if [ -n "$HOLDER_PID" ]; then kill "$HOLDER_PID" 2>/dev/null || true; fi

echo "== result =="
if [ "$fail" -eq 0 ]; then
  echo "GATE RESULT: PASS" | tee -a "$SUMMARY"
else
  echo "GATE RESULT: FAIL" | tee -a "$SUMMARY"
fi
exit "$fail"
