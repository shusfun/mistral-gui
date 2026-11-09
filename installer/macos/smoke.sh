#!/bin/sh
set -eu
test "$(uname -s)" = Darwin
test "${GITHUB_ACTIONS:-}" = true
ROOT="${RUNNER_TEMP:?}/mistral-gui-dmg-smoke"
MOUNT="$ROOT/mounted"
DATA="$ROOT/data"
APP="$ROOT/Applications/mistral-gui.app"
mkdir -p "$MOUNT" "$ROOT/Applications"
hdiutil attach "$1" -readonly -nobrowse -mountpoint "$MOUNT"
MANAGER_PID=
cleanup() {
  if [ -n "$MANAGER_PID" ]; then
    "$APP/Contents/MacOS/mistral-gui" --addr "$ADDRESS" --data-dir "$DATA" --quit || true
    kill "$MANAGER_PID" 2>/dev/null || true
    wait "$MANAGER_PID" 2>/dev/null || true
  fi
  hdiutil detach "$MOUNT" || true
}
trap cleanup EXIT HUP INT TERM
ditto "$MOUNT/mistral-gui.app" "$APP"
codesign --verify --deep --strict "$APP"
PORT=$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1]);s.close()')
ADDRESS="127.0.0.1:$PORT"
TRACE=$(python3 -c 'import uuid;print("00-"+uuid.uuid4().hex+"-"+uuid.uuid4().hex[:16]+"-01")')
"$APP/Contents/MacOS/mistral-gui" --addr "$ADDRESS" --data-dir "$DATA" --no-open > "$ROOT/manager-output.log" 2>&1 &
MANAGER_PID=$!
HEALTHY=false
for attempt in 1 2 3 4 5 6 7 8 9 10; do
  if curl --fail --silent --header "traceparent: $TRACE" "http://$ADDRESS/healthz" > "$ROOT/health.json"; then
    HEALTHY=true
    break
  fi
  sleep 1
done
test "$HEALTHY" = true
python3 - "$ROOT/health.json" <<'PY'
import json,sys
with open(sys.argv[1]) as f: health=json.load(f)
assert health['ok'] and health['service']=='mistral-gui'
print('Mounted DMG application health passed:', health['version'])
PY
echo "operation=$TRACE phase=installed-app-health result=passed"
