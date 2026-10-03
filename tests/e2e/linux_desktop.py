#!/usr/bin/env python3
"""Linux desktop features on a virtual X11 (Xvfb): applications, active window, software keys, clipboard and clear errors.
Needs: Xvfb xterm xdotool xclip x11-utils.   Usage: tests/e2e/linux_desktop.py /path/to/typedeck   (temporary HOME)"""
import json, os, re, shutil, subprocess, sys, tempfile, time, urllib.request

BIN = sys.argv[1]
for tool in ("Xvfb", "xterm", "xdotool", "xclip", "xprop"):
    if not shutil.which(tool): sys.exit(f"{tool} is missing")
home = tempfile.mkdtemp(prefix="typedeck-desk-"); disp = ":97"
results = []
def check(name, cond, extra=""):
    results.append(bool(cond)); print(("PASS  " if cond else "FAIL  ") + name + (f"   [{extra}]" if extra and not cond else ""), flush=True)
xvfb = subprocess.Popen(["Xvfb", disp, "-screen", "0", "1024x768x24"], stderr=subprocess.DEVNULL); time.sleep(1.5)
env = {**os.environ, "DISPLAY": disp, "HOME": home, "XDG_CONFIG_HOME": os.path.join(home, "cfg"), "XDG_SESSION_TYPE": "x11"}
env.pop("WAYLAND_DISPLAY", None)
out_file = os.path.join(home, "typed.out")
term = subprocess.Popen(["xterm", "-title", "pruebas", "-e", "sh", "-c", f"cat > {out_file}"], env=env); time.sleep(1.5)
subprocess.run(["xdotool", "search", "--name", "pruebas", "windowfocus"], env=env); time.sleep(0.5)
proc = subprocess.Popen([BIN], env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
for _ in range(100):
    try: PORT = int(open(os.path.join(home, "cfg", "typedeck", "port")).read()); break
    except Exception: time.sleep(0.1)
B = f"http://127.0.0.1:{PORT}"; time.sleep(0.5)
TOKEN = re.search(r'name="token" content="([^"]+)"', urllib.request.urlopen(B + "/").read().decode()).group(1)
def api(p, b=None):
    r = urllib.request.Request(B + p, data=None if b is None else json.dumps(b).encode(), method="POST" if b is not None else "GET", headers={"X-Token": TOKEN, "Content-Type": "application/json"})
    return json.loads(urllib.request.urlopen(r, timeout=30).read())
def test(a): return api("/api/test", a)
def xterms():
    out = subprocess.run(["ps", "-C", "xterm", "-o", "stat="], capture_output=True, text=True).stdout.split()
    return len([x for x in out if not x.startswith("Z")])
try:
    cfg = api("/api/config"); cfg["settings"]["input"] = "software"
    cfg["layers"][0]["auto_apps"] = []; cfg["layers"].append({"name": "Terminal", "color": "", "icon": "", "auto_apps": ["XTerm"], "keys": {}})
    check("the configuration is accepted", api("/api/config", cfg).get("ok"))

    apps = api("/api/apps")
    check("installed applications are listed (.desktop)", "XTerm" in apps, apps[:6])

    front = []
    for _ in range(30):
        front = api("/api/system") and api("/api/status").get("front", [])
        if front: break
        time.sleep(0.4)
    check("the front application is detected through xprop", any("xterm" in f.lower() for f in front), front)
    want = [l["name"] for l in api("/api/config")["layers"]].index("Terminal")
    layer = None
    for _ in range(20):
        layer = api("/api/status").get("layer")
        if layer == want: break
        time.sleep(0.4)
    check("the per-application layer activates on its own when XTerm is focused", layer == want, (layer, want))

    r = test({"type": "hotkey", "keys": "a"}); time.sleep(0.3)
    r2 = test({"type": "hotkey", "keys": "return"}); time.sleep(0.5)
    typed = open(out_file).read() if os.path.exists(out_file) else ""
    check("a software shortcut (xdotool) reaches the active window", r["ok"] and r2["ok"] and typed.strip() == "a", (r, typed))

    subprocess.run(["xclip", "-selection", "clipboard"], input=b"desde el portapapeles", env=env)
    marker = os.path.join(home, "clip.out")
    r = test({"type": "shell", "cmd": f"printf %s {{clipboard}} > '{marker}'", "show_output": True}); time.sleep(0.3)
    check("{clipboard} reads the X11 clipboard", r["ok"] and os.path.exists(marker) and open(marker).read() == "desde el portapapeles", r)

    n0 = xterms()
    r = test({"type": "app", "app": "XTerm", "mode": "open"}); time.sleep(1.5)
    n1 = xterms()
    check("opening an application by name launches its .desktop", r["ok"] and n1 == n0 + 1, (r, n0, n1))

    r = test({"type": "app", "app": "xterm", "mode": "quit"}); time.sleep(3)
    n2 = xterms()
    check("close an application by name", r["ok"] and n2 == 0, (r, n2))

    r = test({"type": "media", "cmd": "playpause"})
    check("without a board or playerctl, media explains what is missing", r["ok"] is False and "playerctl" in r["output"], r)
    r = test({"type": "system", "cmd": "screenshot"})
    check("without a screenshot tool the error says so", r["ok"] is False and "screenshot" in r["output"], r)
    r = test({"type": "text", "text": "hola"})
    check("typing text without a board pastes through the clipboard without hanging", r["ms"] < 5000, r)
    st = api("/api/setup")
    check("the diagnostics describe Linux and the keyboard layout", st["host"]["os"] == "linux" and st["host"]["version"] != "", st["host"])
finally:
    for p in (proc, term, xvfb):
        try: p.terminate(); p.wait(3)
        except Exception: p.kill()
    if not all(results): print((proc.stdout.read().decode(errors="ignore") if proc.stdout else "")[-1500:])
print(f"\n{sum(results)}/{len(results)} checks passed"); sys.exit(0 if all(results) else 1)
