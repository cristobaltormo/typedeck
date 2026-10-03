#!/usr/bin/env python3
"""Tests on a REAL Linux machine with the board plugged in and a graphical session (not run in CI).
Needs Typedeck running in the user session with TYPEDECK_DEV=1, the editor reachable on 127.0.0.1:7788 (for example
ssh -L 7788:127.0.0.1:7788 laptop) and tests/e2e/linux_testbox.py copied to LINUX_TESTBOX. It types only into its own test box.
Usage: linux_real.py typing|focus|desktop"""
import sys, re, json, time, os, urllib.request
BOX = os.environ.get("LINUX_TESTBOX", "/tmp/linux_testbox.py")
DIR = os.environ.get("LINUX_TESTBOX_DIR", "/tmp/typedeck-testbox")
B = "http://127.0.0.1:7788"
T = re.search(r'name="token" content="([^"]+)"', urllib.request.urlopen(B + "/").read().decode()).group(1)
def api(p, b=None):
    r = urllib.request.Request(B + p, data=None if b is None else json.dumps(b).encode(), headers={"X-Token": T, "Content-Type": "application/json"})
    return json.loads(urllib.request.urlopen(r, timeout=30).read())
def sh(cmd, out=True): return api("/api/test", {"type": "shell", "cmd": cmd, "show_output": out})["output"]

res = []
def check(n, c, x=""): res.append(bool(c)); print(("PASS  " if c else "FAIL  ") + n + (f"   [{x}]" if x and not c else ""), flush=True)

def open_box():
    sh("pkill -f linux_testbox.py", True) if False else None
    sh(f"rm -rf {DIR}; mkdir -p {DIR}", True)
    api("/api/test", {"type": "shell", "cmd": f"python3 {BOX} {DIR}", "show_output": False})
    for _ in range(30):
        time.sleep(0.5)
        if "ok" in sh(f"test -f {DIR}/keys.txt && echo ok"): break
    time.sleep(1.5)

def text(): return sh(f"cat {DIR}/text.txt 2>/dev/null")
def hk(k): return api("/api/test", {"type": "hotkey", "keys": k})
def type_text(t):
    hk("ctrl+a"); hk("backspace"); time.sleep(0.4)
    api("/api/test", {"type": "text", "text": t}); time.sleep(0.8)
    return text()

def close_box():
    sh("python3 -c \"import os,signal,subprocess; [os.kill(int(p), signal.SIGTERM) for p in subprocess.run(['pgrep','-f','python3 .*linux_testbox'],capture_output=True,text=True).stdout.split() if int(p)!=os.getpid()]\"")

def run_typing():
    cfg = api("/api/config"); cfg["settings"]["typing_layout"] = "auto"; cfg["settings"]["input"] = "auto"; api("/api/config", cfg)
    check("the board is connected", api("/api/status")["connected"])
    print("layout:", api("/api/setup")["host"]["typing_layout"])
    open_box()
    got = type_text("Hello 123")
    check("the test box received text from the board (it has the focus)", got == "Hello 123", repr(got))
    if got != "Hello 123": close_box(); raise SystemExit("abort: the test box did not get the focus")
    for t in ["Hola mundo 123", "Hola, ¿qué tal? Ñandú ñ ç", "@#{}[]\\|~ <> - _ ; : . , ' \" !", "á é í ó ú ü Á É", "ABC def GHI", "€ ¬ º ª", "Línea con ~/tmp y C:\\Users\\x"]:
        got = type_text(t); check(f"typing through the board: {t!r}", got == t, repr(got))
    type_text("x"); hk("ctrl+a"); hk("ctrl+x"); time.sleep(0.6)
    check("ctrl+x through the board cuts the selection", text() == "", repr(text()))
    keys = sh(f"tail -n 4 {DIR}/keys.txt")
    check("the box received the control key with the letter", "x|ctrl=True" in keys, keys[-120:])
    close_box()

def run_focus():
    st = api("/api/status")
    r = api("/api/test", {"type": "url", "url": "{editor}"})
    check("the key that opens the editor either focuses the open tab or opens the page and says which", r["ok"] and r["output"] in ("focused", "opened"), r)
    print("editors:", st.get("editors"), "result:", r["output"])

def run_desktop():
    apps = api("/api/apps")
    check("the installed applications are listed from the .desktop files", isinstance(apps, list) and len(apps) > 5, len(apps))
    app = next((a for a in ("Calculator", "Text Editor", "Files") if a in apps), None)
    check("there is a harmless application to open and close", app is not None, apps[:10])
    if app:
        sh("pkill -f gnome-calculator; pkill -f gnome-text-editor", True)
        r = api("/api/test", {"type": "app", "app": app, "mode": "open"}); time.sleep(3)
        check(f"opening {app} by its name works", r["ok"], r)
        r = api("/api/test", {"type": "app", "app": app, "mode": "quit"}); time.sleep(2)
        check(f"closing {app} by its name works", r["ok"], r)
    r = api("/api/test", {"type": "app", "app": "this-app-does-not-exist-123", "mode": "open"})
    check("opening a missing app gives a clear error", not r["ok"] and "not found" in r["output"], r)
    cfg = api("/api/config"); cfg["settings"]["input"] = "software"; api("/api/config", cfg)
    r = api("/api/test", {"type": "hotkey", "keys": "ctrl+shift+f12"})
    check("a software shortcut on Wayland without wtype says what is missing", not r["ok"] and ("wtype" in r["output"] or "xdotool" in r["output"]), r)
    cfg["settings"]["input"] = "auto"; api("/api/config", cfg)
    r = api("/api/test", {"type": "hud", "text": "Typedeck test"})
    check("a popup is shown", r["ok"], r)
    r = api("/api/test", {"type": "system", "cmd": "darkmode"}); check("a system command answers (dark mode is not changed twice)", r["ok"] or r["output"], r)
    api("/api/test", {"type": "system", "cmd": "darkmode"})

if __name__ == "__main__":
    {"typing": run_typing, "focus": run_focus, "desktop": run_desktop}[sys.argv[1]]()
    print(f"\n{sum(res)}/{len(res)}")
