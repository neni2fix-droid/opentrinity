#!/usr/bin/env bash
# Start a loopback-only deterministic demo instance.
# The fake upstream is a test fixture, not a real AI model.
set -e

ROOT=$(cd "$(dirname "$0")/.." && pwd)
if command -v cygpath >/dev/null 2>&1; then
  ROOT_WIN=$(cygpath -m "$ROOT")
else
  ROOT_WIN="$ROOT"
fi

BIN="$ROOT/dist/opentrinity.exe"
FAKE="$ROOT/.phase0/harness/fakeupstream/fakeupstream.exe"
LITELLM="$ROOT_WIN/.phase0/venv2/Scripts/litellm.exe"
UD="$ROOT_WIN/.demo-userdata"

mkdir -p "$ROOT/dist"
CGO_ENABLED=0 go build -o "$BIN" ./cmd/opentrinity

export OT_USER_DATA="$UD"
export OT_LITELLM_EXE="$LITELLM"

if ! curl -sf http://127.0.0.1:11001/health >/dev/null 2>&1; then
  "$FAKE" -ports 11001,11002,11003 > "$ROOT/logs-demo-fakeupstream.log" 2>&1 &
  echo $! > "$ROOT/.demo-fake.pid"
  for i in $(seq 1 20); do
    curl -sf http://127.0.0.1:11001/health >/dev/null 2>&1 && break
    sleep 0.2
  done
fi

if [ ! -f "$UD/opentrinity.db" ]; then
  "$BIN" init
  "$BIN" providers add demo-a http://127.0.0.1:11001/v1
  "$BIN" providers add demo-b http://127.0.0.1:11002/v1
  "$BIN" providers add demo-c http://127.0.0.1:11003/v1
  printf 'demo-fake-key-a\n' | "$BIN" credentials stdin 1 primary >/dev/null
  printf 'demo-fake-key-b\n' | "$BIN" credentials stdin 2 primary >/dev/null
  printf 'demo-fake-key-c\n' | "$BIN" credentials stdin 3 primary >/dev/null
  "$BIN" models add 1 fake-model demo-a >/dev/null
  "$BIN" models add 2 fake-model demo-b >/dev/null
  "$BIN" models add 3 fake-model demo-c >/dev/null
  "$BIN" deployments set A 1 >/dev/null
  "$BIN" deployments set B 2 >/dev/null
  "$BIN" deployments set C 3 >/dev/null
  "$BIN" apply run
fi

"$BIN" start
"$BIN" status
echo "Demo ready. Retrieve the local client key with: $BIN gateway key show"
echo "Remember: the fake upstream returns fixed text; configure a real provider for actual AI features."
