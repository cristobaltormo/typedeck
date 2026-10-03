#!/usr/bin/env bash
# Runs every test that needs a real Linux machine with a graphical session and the board plugged in: the unit tests compiled for
# Linux, the hardware tests, typing through the board into a window of our own, the desktop features, the editor tab focus,
# fail-open, the editor self test in the machine's Chrome and the user service. It uses ssh (LINUX_HOST, default
# user@host, key LINUX_KEY, default ~/.ssh/id_ed25519) and passwordless access to sudo through LINUX_SUDO_FILE, a file holding the password.
# The machine needs the session variables in /tmp/tdenv.sh (XDG_RUNTIME_DIR, WAYLAND_DISPLAY or DISPLAY, DBUS_SESSION_BUS_ADDRESS).
# Usage: scripts/e2e-linux-real.sh
set -uo pipefail
cd "$(dirname "$0")/.."
HOST="${LINUX_HOST:?set LINUX_HOST, for example user@laptop}"
KEY="${LINUX_KEY:-$HOME/.ssh/id_ed25519}"
SUDO_FILE="${LINUX_SUDO_FILE:-}"
PORT=7788
FAIL=0
step() { printf '\n===== %s =====\n' "$1"; }
rs() { ssh -i "$KEY" -o ConnectTimeout=10 "$HOST" "$@" 2>&1; }
rsudo() { if [ -n "$SUDO_FILE" ]; then cat "$SUDO_FILE" | ssh -i "$KEY" "$HOST" "sudo -S -p '' $*" 2>&1; else rs "sudo $*"; fi; }
run() { "$@" || { FAIL=$((FAIL + 1)); echo "FAILED: $*"; }; }

tunnel_up() {
  ss -ltn 2>/dev/null | grep -q ":$PORT " && return 0
  ssh -f -N -o ExitOnForwardFailure=yes -i "$KEY" -L "$PORT:127.0.0.1:$PORT" "$HOST" && sleep 1
}
wait_editor() { for _ in $(seq 40); do curl -s -o /dev/null "http://127.0.0.1:$PORT/" && return 0; sleep 0.5; done; return 1; }
stop_program() { rs '. /tmp/tdenv.sh; systemctl --user stop typedeck 2>/dev/null; kill $(pgrep -x typedeck) 2>/dev/null; sleep 2' >/dev/null; }
start_dev() { rs '. /tmp/tdenv.sh; (TYPEDECK_DEV=1 nohup typedeck >/tmp/td-dev.log 2>&1 &); sleep 14; tail -1 /tmp/td-dev.log'; }

step "build and deploy"
VERSION="$(git describe --tags --always)-linux"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o /tmp/typedeck-linux ./cmd/typedeck || exit 1
stop_program
scp -q -i "$KEY" /tmp/typedeck-linux "$HOST:/tmp/typedeck-new"
scp -q -i "$KEY" tests/e2e/linux_testbox.py "$HOST:/tmp/linux_testbox.py"
rsudo "install -m 0755 /tmp/typedeck-new /usr/bin/typedeck" >/dev/null
echo "deployed $VERSION"

step "unit tests compiled for Linux, run on the machine"
rm -rf /tmp/lt && mkdir /tmp/lt
for p in $(go list ./... 2>/dev/null); do go test -c -o "/tmp/lt/$(basename "$p").test" "$p" 2>/dev/null; done
rs "rm -rf /tmp/lt /tmp/ltrepo; mkdir -p /tmp/lt /tmp/ltrepo/a/b" >/dev/null
scp -q -i "$KEY" /tmp/lt/*.test "$HOST:/tmp/lt/"
scp -q -i "$KEY" -r examples "$HOST:/tmp/ltrepo/"
for t in $(ls /tmp/lt | sed 's/.test$//'); do
  out=$(rs "cd /tmp/ltrepo/a/b && /tmp/lt/$t.test" | tail -1)
  printf '%-10s %s\n' "$t" "$out"
  [ "$out" = "PASS" ] || FAIL=$((FAIL + 1))
done

step "hardware tests (program in development mode)"
start_dev
tunnel_up && wait_editor || { echo "the editor is not reachable"; exit 1; }
TYPEDECK_REMOTE=1 run python3 tests/e2e/hardware.py

step "typing, focus and desktop features"
run python3 tests/e2e/linux_real.py typing
run python3 tests/e2e/linux_real.py focus
run python3 tests/e2e/linux_real.py desktop

step "editor self test in Chrome on the machine"
rs 'rm -f /tmp/st.html; google-chrome --headless=new --no-sandbox --disable-gpu --user-data-dir=/tmp/chrome-test --virtual-time-budget=60000 --dump-dom "http://127.0.0.1:7788/?selftest" > /tmp/st.html 2>/dev/null; rm -rf /tmp/chrome-test' >/dev/null
scp -q -i "$KEY" "$HOST:/tmp/st.html" /tmp/st.html
grep -o '<pre id="selftest">.*</pre>' /tmp/st.html | sed 's/<[^>]*>//g' | python3 -c "
import sys, json, html
raw = sys.stdin.read().strip()
if not raw: print('no result'); sys.exit(1)
d = json.loads(html.unescape(raw)); bad = [r for r in d if not r['ok']]
for r in bad: print('FAIL', r['name'], r['extra'][:200])
print(len(d) - len(bad), '/', len(d), 'editor checks'); sys.exit(1 if bad else 0)" || FAIL=$((FAIL + 1))

step "fail-open: the board lets every key through without the program"
stop_program
sleep 8
out=$(rs 'timeout 20 python3 -c "
import os, time, termios, select
fd = os.open(\"/dev/ttyACM0\", os.O_RDWR | os.O_NOCTTY)
a = termios.tcgetattr(fd); a[4] = a[5] = termios.B115200; a[0] = a[1] = a[3] = 0; a[2] = termios.CS8 | termios.CREAD | termios.CLOCAL; termios.tcsetattr(fd, termios.TCSANOW, a)
time.sleep(1); os.write(fd, b\"STATS\n\"); time.sleep(0.8); r = b\"\"
while select.select([fd], [], [], 0.3)[0]: r += os.read(fd, 400)
print(r.decode(errors=\"replace\"))"')
echo "$out" | grep -o "capture=[01]" | head -1
echo "$out" | grep -q "capture=0" && echo "PASS  without a heartbeat the board stops capturing" || { echo "FAIL  fail-open"; FAIL=$((FAIL + 1)); }

step "user service: install, doctor, uninstall"
rs '. /tmp/tdenv.sh; typedeck install 2>&1 | tail -1; sleep 20; typedeck doctor 2>&1 | tail -6; typedeck uninstall 2>&1 | tail -1'
doctor=$(rs '. /tmp/tdenv.sh; typedeck install >/dev/null 2>&1; sleep 20; typedeck doctor 2>&1; typedeck uninstall >/dev/null 2>&1')
echo "$doctor" | grep -q "all good" || { echo "FAIL  doctor"; FAIL=$((FAIL + 1)); }

step "cleanup"
stop_program
printf '\n%s\n' "$([ "$FAIL" = 0 ] && echo 'ALL LINUX CHECKS PASSED' || echo "$FAIL CHECK GROUPS FAILED")"
exit "$FAIL"
