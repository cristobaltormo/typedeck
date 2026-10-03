#!/usr/bin/env bash
# Runs the editor self test in a headless Chromium or Chrome against a throwaway configuration.
# Usage: [CHROME=chromium] scripts/selftest.sh
set -euo pipefail
cd "$(dirname "$0")/.."
CHROME="${CHROME:-$(command -v chromium || command -v chromium-browser || command -v google-chrome || command -v google-chrome-stable || true)}"
[ -n "$CHROME" ] || { echo "Chromium or Chrome not found (set CHROME)"; exit 1; }
make -s build
HOME_DIR=$(mktemp -d)
PID=""
trap '[ -n "$PID" ] && kill "$PID" 2>/dev/null || true; rm -rf "$HOME_DIR"' EXIT
XDG_CONFIG_HOME="$HOME_DIR" HOME="$HOME_DIR" ./dist/typedeck >/dev/null 2>&1 &
PID=$!
for _ in $(seq 50); do [ -s "$HOME_DIR/typedeck/port" ] && break; sleep 0.1; done
PORT=$(cat "$HOME_DIR/typedeck/port")
"$CHROME" --headless=new --no-sandbox --disable-gpu --virtual-time-budget=60000 --dump-dom "http://127.0.0.1:$PORT/?selftest" 2>/dev/null \
  | grep -o '<pre id="selftest">.*</pre>' | sed 's/<[^>]*>//g' \
  | python3 -c "
import sys, json, html
raw = sys.stdin.read().strip()
if not raw:
    print('no result: the page did not finish'); sys.exit(1)
results = json.loads(html.unescape(raw))
bad = [r for r in results if not r['ok']]
for r in bad:
    print('FAIL', r['name'], r['extra'][:300])
print(len(results) - len(bad), '/', len(results), 'editor checks passed')
sys.exit(1 if bad else 0)"
