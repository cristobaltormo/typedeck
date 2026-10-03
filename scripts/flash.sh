#!/usr/bin/env bash
# Usage: [MAC_HOST=mac] scripts/flash.sh [file.hex]   (compiles firmware/ by default)
MAC_HOST="${MAC_HOST:-mac}"
set -euo pipefail
cd "$(dirname "$0")/.."
HEX="${1:-}"
if [ -z "$HEX" ]; then
  export PATH="$HOME/.local/bin:$PATH"
  arduino-cli compile --fqbn arduino:avr:leonardo --output-dir /tmp/typedeck-fw firmware >/dev/null
  HEX=/tmp/typedeck-fw/firmware.ino.hex
fi
scp -q "$HEX" "$MAC_HOST":/tmp/typedeck-fw.hex
ssh "$MAC_HOST" 'bash -s' <<'REMOTE'
set -u
export PATH=/opt/homebrew/bin:$PATH
UIDN=$(id -u)
for label in cc.cristobal.typedeck cc.cristobal.numdeck cc.cristobal.leonardo-macros; do launchctl bootout "gui/$UIDN/$label" 2>/dev/null || true; done
pkill -x typedeck 2>/dev/null || true
sleep 1
P=$(ls /dev/cu.usbmodem* 2>/dev/null | head -1)
[ -z "$P" ] && { echo "board not found"; exit 1; }
stty -f "$P" 1200; sleep 1.5
for i in 1 2 3 4 5 6 7 8 9 10; do BP=$(ls /dev/cu.usbmodem* 2>/dev/null | head -1); [ -n "$BP" ] && break; sleep 0.5; done
avrdude -p atmega32u4 -c avr109 -P "$BP" -b 57600 -U flash:w:/tmp/typedeck-fw.hex:i 2>&1 | tail -3
sleep 3
launchctl bootstrap "gui/$UIDN" "$HOME/Library/LaunchAgents/cc.cristobal.typedeck.plist" 2>/dev/null || true
ls /dev/cu.usbmodem*
REMOTE
