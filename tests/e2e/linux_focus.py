#!/usr/bin/env python3
"""The key that opens the editor focuses the tab that is already open, on X11 with a window manager.
Needs: Xvfb openbox xdotool xterm. An xterm of class Chromium titled Typedeck stands in for the browser tab.   Usage: tests/e2e/linux_focus.py /path/to/typedeck   (temporary HOME)"""
import json, os, re, shutil, socket, subprocess, sys, tempfile, time, urllib.request

BIN = sys.argv[1]
for tool in ("Xvfb", "openbox", "xdotool", "xterm"):
    if not shutil.which(tool): sys.exit(f"{tool} is missing")
home = tempfile.mkdtemp(prefix="typedeck-focus-"); disp = ":96"
results = []
def check(name, cond, extra=""):
    results.append(bool(cond)); print(("PASS  " if cond else "FAIL  ") + name + (f"   [{extra}]" if extra and not cond else ""), flush=True)
env = {**os.environ, "HOME": home, "XDG_CONFIG_HOME": home + "/.config", "DISPLAY": disp}
procs = []
def start(cmd, **kw):
    p = subprocess.Popen(cmd, env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, **kw); procs.append(p); return p
try:
    start(["Xvfb", disp, "-screen", "0", "1280x800x24"]); time.sleep(1.5)
    start(["openbox"]); time.sleep(1)
    prog = start([BIN]); time.sleep(2)
    port = open(home + "/.config/typedeck/port").read().strip()
    base = f"http://127.0.0.1:{port}"
    token = re.search(r'name="token" content="([^"]+)"', urllib.request.urlopen(base + "/").read().decode()).group(1)
    def api(p, b=None):
        r = urllib.request.Request(base + p, data=None if b is None else json.dumps(b).encode(), headers={"X-Token": token, "Content-Type": "application/json"})
        return json.loads(urllib.request.urlopen(r, timeout=20).read())
    start(["xterm", "-class", "Chromium", "-T", "Typedeck - editor"]); time.sleep(1)
    ws = socket.create_connection(("127.0.0.1", int(port)))
    ws.sendall(("GET /api/ws HTTP/1.1\r\nHost: 127.0.0.1:%s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\n"
                "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Protocol: typedeck.%s\r\n\r\n" % (port, token)).encode())
    time.sleep(1)
    check("an editor tab is connected over the WebSocket", api("/api/status").get("editors", 0) >= 1, api("/api/status"))
    start(["xterm", "-T", "other"]); time.sleep(1.5)
    front = subprocess.run(["xdotool", "getactivewindow", "getwindowname"], env=env, capture_output=True, text=True).stdout.strip()
    check("another window is in front", "Typedeck" not in front, front)
    r = api("/api/test", {"type": "url", "url": "{editor}"}); time.sleep(1)
    front = subprocess.run(["xdotool", "getactivewindow", "getwindowname"], env=env, capture_output=True, text=True).stdout.strip()
    check("the key that opens the editor focuses the open tab instead of opening another", r["ok"] and r["output"] == "focused", r)
    check("the editor window is now in front", "Typedeck" in front, front)
finally:
    for p in procs[::-1]:
        p.terminate()
    shutil.rmtree(home, ignore_errors=True)
print(f"\n{sum(results)}/{len(results)} checks passed"); sys.exit(0 if all(results) else 1)
