#!/usr/bin/env bash
# Usage: [MAC_HOST=mac] scripts/install-mac.sh [version]
MAC_HOST="${MAC_HOST:-mac}"
set -euo pipefail
cd "$(dirname "$0")/.."
VERSION="${1:-$(git describe --tags --always 2>/dev/null || echo 0.3.0-dev)}"
mkdir -p dist
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o dist/typedeck ./cmd/typedeck
scp -q dist/typedeck macos/hud/hud.swift "$MAC_HOST":/tmp/
ssh "$MAC_HOST" 'bash -s' <<'REMOTE'
set -euo pipefail
D="$HOME/.local/share/typedeck"; OLD="$HOME/.local/share/leonardo-macros"
UIDN=$(id -u)
mkdir -p "$D" "$HOME/Library/LaunchAgents" "$HOME/Applications"
for label in cc.cristobal.leonardo-macros cc.cristobal.numdeck cc.cristobal.typedeck; do launchctl bootout "gui/$UIDN/$label" 2>/dev/null || true; done
pkill -x typedeck 2>/dev/null || true
sleep 1
install -m 755 /tmp/typedeck "$D/typedeck"
swiftc -O /tmp/hud.swift -o "$D/typedeck-hud"
rm -rf "$OLD" "$HOME/.local/share/numdeck" "$HOME/Library/LaunchAgents/cc.cristobal.leonardo-macros.plist" "$HOME/Library/LaunchAgents/cc.cristobal.numdeck.plist" "$HOME/Applications/Macros.app" "$HOME/Applications/Numdeck.app"

A="$HOME/Applications/Typedeck.app/Contents"
mkdir -p "$A/MacOS"
cat > "$A/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleName</key><string>Typedeck</string>
<key>CFBundleIdentifier</key><string>cc.cristobal.typedeck</string>
<key>CFBundleExecutable</key><string>Typedeck</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>LSUIElement</key><true/>
</dict></plist>
PLIST
printf '#!/bin/sh\nPORT=$(cat "$HOME/.config/typedeck/port" 2>/dev/null || echo 7788)\nexec open "http://127.0.0.1:$PORT/"\n' > "$A/MacOS/Typedeck"
chmod +x "$A/MacOS/Typedeck"

cat > "$HOME/Library/LaunchAgents/cc.cristobal.typedeck.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>cc.cristobal.typedeck</string>
<key>ProgramArguments</key><array><string>$D/typedeck</string></array>
<key>RunAtLoad</key><true/>
<key>KeepAlive</key><true/>
<key>ProcessType</key><string>Background</string>
<key>EnvironmentVariables</key><dict><key>PATH</key><string>/opt/homebrew/bin:/opt/homebrew/sbin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string></dict>
<key>StandardOutPath</key><string>$HOME/Library/Logs/typedeck.log</string>
<key>StandardErrorPath</key><string>$HOME/Library/Logs/typedeck.log</string>
</dict></plist>
PLIST
: > "$HOME/Library/Logs/typedeck.log"
launchctl bootstrap "gui/$UIDN" "$HOME/Library/LaunchAgents/cc.cristobal.typedeck.plist"
sleep 3
cat "$HOME/Library/Logs/typedeck.log"
REMOTE
