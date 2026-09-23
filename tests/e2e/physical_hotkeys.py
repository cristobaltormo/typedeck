"""Guided test of a real application's shortcuts: brings the app to the front and fires each shortcut with a pause.
Usage (on the Mac): python3 physical_hotkeys.py "Discord" "cmd+k:Switcher" "esc:Close" ...   (shortcut:description)"""
import json, re, subprocess, sys, time, urllib.request
B = "http://127.0.0.1:7788"
T = re.search(r'name="token" content="([^"]+)"', urllib.request.urlopen(B + "/").read().decode()).group(1)
def api(p, b=None):
    r = urllib.request.Request(B + p, data=None if b is None else json.dumps(b).encode(), method="POST" if b is not None else "GET", headers={"X-Token": T, "Content-Type": "application/json"})
    return json.loads(urllib.request.urlopen(r, timeout=20).read())
app, steps = sys.argv[1], sys.argv[2:]
subprocess.run(["open", "-a", app]); time.sleep(3)
front = subprocess.run(["lsappinfo", "info", "-only", "name", subprocess.run(["lsappinfo", "front"], capture_output=True, text=True).stdout.strip()], capture_output=True, text=True).stdout
if app.lower() not in front.lower():
    print("ABORTO: la aplicacion no esta delante:", front.strip()); sys.exit(1)
for s in steps:
    keys, label = s.split(":", 1)
    r = api("/api/test", {"type": "hotkey", "keys": keys})
    print(f"{time.strftime('%H:%M:%S')} {label:28s} {keys:16s} ok={r.get('ok')} {r.get('output','')[:60]}", flush=True)
    time.sleep(4)
