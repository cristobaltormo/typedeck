#!/usr/bin/env bash
# Runs every test that needs a real Windows machine with the board plugged in: the unit tests compiled for Windows, the hardware
# tests, typing through the board, the desktop features, the editor tab focus, fail-open, the editor self test in Chrome and the
# service install and uninstall. The host is an ssh alias, WIN_HOST (default "windows"); Typedeck is installed there as a scheduled
# task named Typedeck, in WIN_DIR (set WIN_DIR, for example C:\Users\me\typedeck), and Chrome is in its default place.
# Usage: scripts/e2e-windows.sh
set -uo pipefail
cd "$(dirname "$0")/.."
WIN_HOST="${WIN_HOST:-windows}"
WIN_DIR="${WIN_DIR:?set WIN_DIR, for example C:\\Users\\me\\typedeck}"
WIN_TMP='C:\typedeck'
CHROME='C:\Program Files\Google\Chrome\Application\chrome.exe'
PORT=7788
FAIL=0
step() { printf '\n===== %s =====\n' "$1"; }
win() { ssh -o ConnectTimeout=10 "$WIN_HOST" "$@" 2>&1 | tr -d '\r'; }
run() { "$@" || { FAIL=$((FAIL + 1)); echo "FAILED: $*"; }; }

tunnel_up() {
  ss -ltn 2>/dev/null | grep -q ":$PORT " && return 0
  ssh -f -N -o ExitOnForwardFailure=yes -L "$PORT:127.0.0.1:$PORT" "$WIN_HOST" && sleep 1
}
tunnel_down() { pkill -f "ssh -f -N .*-L $PORT:127.0.0.1:$PORT $WIN_HOST" 2>/dev/null || true; }
wait_editor() {
  for _ in $(seq 40); do curl -s -o /dev/null "http://127.0.0.1:$PORT/" && return 0; sleep 0.5; done
  return 1
}
stop_program() { win "taskkill /F /IM typedeck.exe" >/dev/null; sleep 2; }
start_task() { win "schtasks /Run /TN $1" >/dev/null; sleep 5; }
make_task() { win "schtasks /Create /TN $1 /TR \"$2\" /SC ONCE /ST 23:59 /IT /F" >/dev/null; }
open_chrome() {
  win "taskkill /F /IM chrome.exe" >/dev/null; sleep 2
  make_task tdopen "\\\"$CHROME\\\" --new-window http://127.0.0.1:$PORT/"
  start_task tdopen
}

step "build and deploy"
VERSION="$(git describe --tags --always)-win"
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$VERSION -H=windowsgui" -o /tmp/typedeck-win.exe ./cmd/typedeck || exit 1
stop_program
scp -q /tmp/typedeck-win.exe "$WIN_HOST:$(echo "$WIN_DIR" | sed 's|\\|/|g')/typedeck.exe" || exit 1
win "if not exist $WIN_TMP mkdir $WIN_TMP" >/dev/null
scp -q tests/e2e/windows_fg.ps1 "$WIN_HOST:C:/typedeck/fg.ps1"
scp -q tests/e2e/windows_testbox.ps1 "$WIN_HOST:C:/typedeck/testbox.ps1"
scp -q tests/e2e/windows_failopen.ps1 "$WIN_HOST:C:/typedeck/failopen.ps1"
echo "deployed $VERSION"

step "unit tests compiled for Windows, run on the machine"
rm -rf /tmp/wt && mkdir /tmp/wt
for p in $(go list ./... 2>/dev/null); do GOOS=windows GOARCH=amd64 go test -c -o "/tmp/wt/$(basename "$p").test.exe" "$p" 2>/dev/null; done
win "rmdir /S /Q C:\\typedeck\\wt & mkdir C:\\typedeck\\wt C:\\typedeck\\a\\b" >/dev/null
scp -q /tmp/wt/*.exe "$WIN_HOST:C:/typedeck/wt/"
scp -q -r examples "$WIN_HOST:C:/typedeck/"
for t in $(ls /tmp/wt | sed 's/.test.exe//'); do
  out=$(win "cd C:\\typedeck\\a\\b && C:\\typedeck\\wt\\$t.test.exe" | tail -1)
  printf '%-10s %s\n' "$t" "$out"
  [ "$out" = "PASS" ] || FAIL=$((FAIL + 1))
done

step "hardware tests (program in development mode)"
make_task tddev "cmd /c set TYPEDECK_DEV=1&& $WIN_DIR\\typedeck.exe"
start_task tddev
tunnel_up && wait_editor || { echo "the editor is not reachable"; exit 1; }
TYPEDECK_REMOTE=1 run python3 tests/e2e/hardware.py

step "typing through the board and desktop features"
open_chrome
run python3 tests/e2e/windows_real.py typing
run python3 tests/e2e/windows_real.py desktop
run python3 tests/e2e/windows_real.py focus

step "editor self test in Chrome on Windows"
win "del C:\\typedeck\\st.html" >/dev/null
win "\"$CHROME\" --headless=new --disable-gpu --user-data-dir=C:\\typedeck\\chrome-test --virtual-time-budget=60000 --dump-dom \"http://127.0.0.1:$PORT/?selftest\" > C:\\typedeck\\st.html" >/dev/null
scp -q "$WIN_HOST:C:/typedeck/st.html" /tmp/st.html
grep -o '<pre id="selftest">.*</pre>' /tmp/st.html | sed 's/<[^>]*>//g' | python3 -c "
import sys, json, html
raw = sys.stdin.read().strip()
if not raw: print('no result'); sys.exit(1)
d = json.loads(html.unescape(raw)); bad = [r for r in d if not r['ok']]
for r in bad: print('FAIL', r['name'], r['extra'][:200])
print(len(d) - len(bad), '/', len(d), 'editor checks'); sys.exit(1 if bad else 0)" || FAIL=$((FAIL + 1))

step "fail-open: the board lets every key through without the program"
stop_program
tunnel_down
win "taskkill /F /IM chrome.exe" >/dev/null
sleep 8
out=$(win "powershell -NoProfile -ExecutionPolicy Bypass -File C:\\typedeck\\failopen.ps1")
echo "$out" | grep -o "capture=[01]" | head -1
echo "$out" | grep -q "capture=0" && echo "PASS  without a heartbeat the board stops capturing" || { echo "FAIL  fail-open"; FAIL=$((FAIL + 1)); }

step "service: uninstall, install, doctor"
win "$WIN_DIR\\typedeck.exe uninstall" | tail -1
win "$WIN_DIR\\typedeck.exe install" | tail -1
sleep 5
doctor=$(win "$WIN_DIR\\typedeck.exe doctor")
echo "$doctor" | tail -8
echo "$doctor" | grep -q "all good" || { echo "FAIL  doctor"; FAIL=$((FAIL + 1)); }

step "cleanup"
win "schtasks /Delete /TN tdopen /F & schtasks /Delete /TN tddev /F & rmdir /S /Q C:\\typedeck\\chrome-test" >/dev/null
tunnel_down
printf '\n%s\n' "$([ "$FAIL" = 0 ] && echo 'ALL WINDOWS CHECKS PASSED' || echo "$FAIL CHECK GROUPS FAILED")"
exit "$FAIL"
