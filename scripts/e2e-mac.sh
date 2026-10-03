#!/usr/bin/env bash
# Usage: [MAC_HOST=mac] scripts/e2e-mac.sh [--typing]
MAC_HOST="${MAC_HOST:-mac}"
set -euo pipefail
cd "$(dirname "$0")/.."
scp -q tests/e2e/hardware.py tests/e2e/ui_selftest.sh "$MAC_HOST":/tmp/
ssh "$MAC_HOST" 'bash -s' -- "${1:-}" <<'REMOTE'
set -u
UIDN=$(id -u); D="$HOME/.local/share/typedeck"
launchctl bootout "gui/$UIDN/cc.cristobal.typedeck" 2>/dev/null; pkill -x typedeck 2>/dev/null; sleep 1
(cd "$D" && TYPEDECK_DEV=1 nohup ./typedeck > /tmp/typedeck-e2e.log 2>&1 &)
sleep 5
echo "===== hardware ====="
/opt/homebrew/bin/python3 /tmp/hardware.py ${1:-}; HW=$?
echo "===== interface ====="
zsh /tmp/ui_selftest.sh | /opt/homebrew/bin/python3 -c "
import sys,json,html
raw=sys.stdin.read().strip()
if not raw: print('NO RESULT'); sys.exit(1)
d=json.loads(html.unescape(raw)); bad=[r for r in d if not r['ok']]
for r in bad: print('FAIL', r['name'], r['extra'][:300])
print(len(d)-len(bad),'/',len(d),'UI checks'); sys.exit(1 if bad else 0)"; UI=$?
pkill -x typedeck; sleep 1
launchctl bootstrap "gui/$UIDN" "$HOME/Library/LaunchAgents/cc.cristobal.typedeck.plist" 2>/dev/null
exit $((HW+UI))
REMOTE
