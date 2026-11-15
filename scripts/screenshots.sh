#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
CHROME="${CHROME:-$(command -v chromium || command -v chromium-browser || command -v google-chrome || true)}"
[ -x "$CHROME" ] || { echo "Chromium or Chrome not found (set CHROME)"; exit 1; }
if [ -z "${TYPEDECK_BIN:-}" ]; then
  cargo build --release --locked --bin typedeck
  TYPEDECK_BIN=target/release/typedeck
fi
[ -x "$TYPEDECK_BIN" ] || { echo "$TYPEDECK_BIN is not an executable"; exit 1; }
HOME_DIR=$(mktemp -d)
PID=""
trap '[ -n "$PID" ] && kill "$PID" 2>/dev/null || true; rm -rf "$HOME_DIR"' EXIT
mkdir -p "$HOME_DIR/typedeck"
python3 - "$HOME_DIR/typedeck" <<'PY'
import json, sys, time
d = sys.argv[1]
text = "Typedeck turns the keyboard you already own into a macro pad. Tap, hold or double tap any key."
us = {c: (0x04 + i, 0) for i, c in enumerate("abcdefghijklmnopqrstuvwxyz")}
us.update({c.upper(): (0x04 + i, 2) for i, c in enumerate("abcdefghijklmnopqrstuvwxyz")})
us.update({" ": (0x2C, 0), ".": (0x37, 0), ",": (0x36, 0)})
now = int(time.time() * 1000) - len(text) * 190
entries, t = [], now
for ch in text:
    u, mods = us[ch]
    if mods:
        entries.append({"t": t - 40, "u": 0xE1, "k": "shift", "d": 120})
    entries.append({"t": t, "u": u, "k": ch.upper(), "d": 55 + (u * 7) % 70})
    t += 120 + (u * 13) % 160
open(d + "/keystrokes.jsonl", "w").write("\n".join(json.dumps(e) for e in entries) + "\n")
PY
(XDG_CONFIG_HOME="$HOME_DIR" HOME="$HOME_DIR" "$TYPEDECK_BIN" >/dev/null 2>&1 & echo $! > "$HOME_DIR/pid")
PID=$(cat "$HOME_DIR/pid")
for _ in $(seq 50); do [ -s "$HOME_DIR/typedeck/config.json" ] && break; sleep 0.1; done
python3 - "$HOME_DIR/typedeck/config.json" <<'PY'
import json, sys
p = sys.argv[1]
c = json.load(open(p))
c["settings"]["key_history"] = True
c["settings"]["theme"] = "light"
json.dump(c, open(p, "w"))
PY
kill $PID; while kill -0 $PID 2>/dev/null; do sleep 0.1; done
rm -f "$HOME_DIR/typedeck/port"
(XDG_CONFIG_HOME="$HOME_DIR" HOME="$HOME_DIR" "$TYPEDECK_BIN" >/dev/null 2>&1 & echo $! > "$HOME_DIR/pid")
PID=$(cat "$HOME_DIR/pid")
for _ in $(seq 50); do [ -s "$HOME_DIR/typedeck/port" ] && break; sleep 0.1; done
PORT=$(cat "$HOME_DIR/typedeck/port")
shot() { "$CHROME" --headless=new --no-sandbox --disable-gpu --hide-scrollbars --window-size="$2" --virtual-time-budget=9000 --screenshot="docs/images/$1.png" "http://127.0.0.1:$PORT/$3" >/dev/null 2>&1; }
shot editor 1440,900 "?scene=default"
shot editor-light 1440,900 "?scene=light"
shot sequence 1440,900 "?scene=seq"
shot gallery 1440,1100 "?scene=default&view=gallery"
shot palette 1440,900 "?scene=palette"
shot history 1440,1250 "?nolive#/history"
ls -la docs/images
