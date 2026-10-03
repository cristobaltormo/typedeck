#!/usr/bin/env python3
"""Manual tests on a REAL Windows machine with the board plugged in (not run in CI).
Needs Typedeck running in the user session (typedeck install), the session unlocked, the editor reachable on 127.0.0.1:7788
(for example ssh -L 7788:127.0.0.1:7788 windows) and tests/e2e/windows_fg.ps1 copied to WIN_FG_SCRIPT.
Types only into its own test box (tests/e2e/windows_testbox.ps1 copied to WIN_TESTBOX_SCRIPT), never into a real application, and only when the box is in front. Usage: windows_real.py typing|desktop|focus|failopen
focus needs Chrome open on the editor in the user session; failopen stops the program (over ssh, host alias WIN_HOST=windows) and reads the
board straight from COM4 with tests/e2e/windows_failopen.ps1 copied to WIN_FAILOPEN_SCRIPT."""
import sys, re, json, time, os, subprocess, urllib.request
FG_SCRIPT = os.environ.get("WIN_FG_SCRIPT", r"C:\typedeck\fg.ps1")
B="http://127.0.0.1:7788"
T=re.search(r'name="token" content="([^"]+)"',urllib.request.urlopen(B+"/").read().decode()).group(1)
def api(p,b=None):
    r=urllib.request.Request(B+p,data=None if b is None else json.dumps(b).encode(),method="POST" if b is not None else "GET",headers={"X-Token":T,"Content-Type":"application/json"})
    return json.loads(urllib.request.urlopen(r,timeout=30).read())

TESTBOX = os.environ.get("WIN_TESTBOX_SCRIPT", r"C:\typedeck\testbox.ps1")
BOX_TEXT = r"C:\typedeck\testbox.txt"
BOX_KEYS = r"C:\typedeck\testbox.keys"

class Box:
    """A window of our own that records its text and every key it receives, so no real application is touched."""
    def __init__(self):
        self.res = []
        self.fg = "powershell -NoProfile -ExecutionPolicy Bypass -File " + FG_SCRIPT
    def sh(self, cmd): return api("/api/test", {"type": "shell", "cmd": cmd, "show_output": True})["output"].strip()
    def front(self): return self.sh(self.fg).lower()
    def check(self, n, c, x=""): self.res.append(bool(c)); print(("PASS  " if c else "FAIL  ") + n + (f"   [{x}]" if x and not c else ""), flush=True)
    def open(self):
        self.sh("taskkill /F /FI \"WINDOWTITLE eq Typedeck test box\"")
        api("/api/test", {"type": "shell", "cmd": f'start "" powershell -NoProfile -ExecutionPolicy Bypass -File {TESTBOX}', "show_output": False})
        for _ in range(25):
            time.sleep(0.6)
            self.sh('powershell -NoProfile -Command "$w=New-Object -ComObject WScript.Shell; $w.SendKeys(\'%\'); Start-Sleep -Milliseconds 200; [void]$w.AppActivate(\'Typedeck test box\')"')
            if "powershell" in self.front(): return True
        return False
    def text(self):
        return self.sh('powershell -NoProfile -Command "[Console]::OutputEncoding=[Text.Encoding]::UTF8; Get-Content -Raw -Encoding UTF8 ' + BOX_TEXT + '"').replace("\r\n", "\n").rstrip("\n")
    def keys(self):
        return self.sh('powershell -NoProfile -Command "Get-Content -Tail 15 ' + BOX_KEYS + '"').splitlines()
    def wait_key(self, prefix, secs=10):
        for _ in range(int(secs * 2)):
            if any(k.startswith(prefix) for k in self.keys()): return True
            time.sleep(0.5)
        return False
    def hk(self, k): return api("/api/test", {"type": "hotkey", "keys": k})
    def type_text(self, t):
        self.hk("ctrl+a"); self.hk("backspace"); time.sleep(0.4)
        api("/api/test", {"type": "text", "text": t}); time.sleep(0.7)
        return self.text()
    def close(self):
        self.sh('taskkill /F /FI "WINDOWTITLE eq Typedeck test box"')

def run_typing():
    b = Box()
    cfg = api("/api/config"); cfg["settings"]["typing_layout"] = "auto"; cfg["settings"]["input"] = "auto"; api("/api/config", cfg)
    b.check("the board is connected on COM4", api("/api/status")["connected"])
    print("layout:", api("/api/setup")["host"]["typing_layout"])
    opened = b.open()
    b.check("the test box is in front (needed to type safely)", opened, b.front())
    if not opened: raise SystemExit("abort: not typing unless the test box is in front")
    for t in ["Hola mundo 123", "Hola, ¿qué tal? Ñandú ñ ç", "@#{}[]\\|~ <> - _ ; : . , ' \" !", "á é í ó ú ü Á É", "ABC def GHI", "€ ¬ º ª", "` ^ ´ ¨ ~n ^a", "Línea con ~/tmp y C:\\Users\\x"]:
        got = b.type_text(t); b.check(f"typing through the board: {t!r}", got == t, repr(got))
    b.type_text("x")
    b.hk("ctrl+a"); b.hk("ctrl+x"); time.sleep(0.5)
    clip = b.sh('powershell -NoProfile -Command "Get-Clipboard -Raw"').strip()
    b.check("ctrl+x shortcut through the board cuts the selection", clip == "x" and b.text() == "", (clip, b.text()))
    b.check("the box received the control key with the letter", b.wait_key("X|ctrl=True"), b.keys()[-3:])
    b.close()
    print(f"\n{sum(b.res)}/{len(b.res)}")
    if not all(b.res): raise SystemExit(1)

def run_desktop():
    b = Box(); sh = b.sh; front = b.front; hk = b.hk
    def activate_proc(proc):
        sh('powershell -NoProfile -Command "$t=(Get-Process %s -ErrorAction SilentlyContinue | Where-Object MainWindowTitle | Select -First 1).MainWindowTitle; if($t){$w=New-Object -ComObject WScript.Shell; $w.SendKeys(\'%%\'); Start-Sleep -Milliseconds 200; [void]$w.AppActivate($t)}"' % proc)
    orig = api("/api/config")
    try:
        cfg = json.loads(json.dumps(orig)); cfg["layers"].append({"name": "ChromeTest", "color": "", "icon": "", "auto_apps": ["Chrome"], "keys": {}}); cfg["settings"]["hud"]["on_auto"] = True
        api("/api/config", cfg); want = len(cfg["layers"]) - 1
        activate_proc("chrome"); time.sleep(1.5)
        names = api("/api/status").get("front")
        b.check("the front application is detected (GetForegroundWindow + Start menu)", names and any("chrome" in n.lower() for n in names), names)
        lay = None
        for _ in range(12):
            lay = api("/api/status").get("layer")
            if lay == want: break
            time.sleep(0.5)
        b.check("the per-application layer activates on its own with Chrome in front", lay == want, (lay, want))
        api("/api/config", orig)

        opened = b.open()
        b.check("the test box is in front", opened, front())
        if not opened: raise SystemExit("abort: not typing unless the test box is in front")
        c2 = api("/api/config"); c2["settings"]["input"] = "software"; api("/api/config", c2)
        got = b.type_text("Hola ñ @ € por software")
        b.check("software text (clipboard + Ctrl+V)", got == "Hola ñ @ € por software", repr(got))
        got = b.type_text("abc")
        hk("ctrl+a"); hk("ctrl+c"); time.sleep(0.4)
        clip = sh('powershell -NoProfile -Command "Get-Clipboard -Raw"').strip()
        b.check("software shortcut (ctrl+c with keybd_event)", clip == "abc", clip)
        c3 = api("/api/config"); c3["settings"]["input"] = "auto"; api("/api/config", c3)

        time.sleep(1)
        r = api("/api/test", {"type": "app", "app": "powershell", "mode": "toggle"}); time.sleep(1.2)
        b.check("toggle: with the app in front it minimizes it", r["ok"] and "powershell" not in front(), (r, front()))
        r = api("/api/test", {"type": "app", "app": "Typedeck test box", "mode": "quit"}); time.sleep(2)
        left = sh('tasklist /FI "WINDOWTITLE eq Typedeck test box" /NH')
        b.check("close the app by its window title (WM_CLOSE)", r["ok"] and "powershell" not in left.lower(), (r, left))
    finally:
        b.close(); api("/api/config", orig)
    print(f"\n{sum(b.res)}/{len(b.res)}")
    if not all(b.res): raise SystemExit(1)

def run_focus():
    sh = lambda c: api("/api/test", {"type": "shell", "cmd": c, "show_output": True})["output"].strip()
    fg = "powershell -NoProfile -ExecutionPolicy Bypass -File " + FG_SCRIPT
    res = []
    def check(n, c, x=""): res.append(bool(c)); print(("PASS  " if c else "FAIL  ") + n + (f"   [{x}]" if x and not c else ""), flush=True)
    check("an editor tab is connected over the WebSocket", api("/api/status").get("editors", 0) >= 1, api("/api/status").get("editors"))
    box = Box(); opened = box.open()
    check("the test box is in front", opened)
    r = api("/api/test", {"type": "url", "url": "{editor}"})
    time.sleep(1)
    check("the key that opens the editor focuses the open tab instead of opening another", r["ok"] and r["output"] == "focused", r)
    check("the browser is now in front", "chrome" in sh(fg).lower() or "msedge" in sh(fg).lower(), sh(fg))
    box.close()
    print(f"\n{sum(res)}/{len(res)}")
    if not all(res): raise SystemExit(1)

def run_failopen():
    host = os.environ.get("WIN_HOST", "windows")
    script = os.environ.get("WIN_FAILOPEN_SCRIPT", r"C:\typedeck\failopen.ps1")
    ssh = lambda c: subprocess.run(["ssh", host, c], capture_output=True, text=True).stdout
    res = []
    def check(n, c, x=""): res.append(bool(c)); print(("PASS  " if c else "FAIL  ") + n + (f"   [{x}]" if x and not c else ""), flush=True)
    ssh("taskkill /F /IM typedeck.exe")
    time.sleep(8)
    out = ssh("powershell -NoProfile -ExecutionPolicy Bypass -File " + script)
    check("without a heartbeat for 5 s the board stops capturing (fail-open)", "capture=0" in out, out)
    ssh("schtasks /Run /TN Typedeck")
    time.sleep(6)
    print(f"\n{sum(res)}/{len(res)}")
    if not all(res): raise SystemExit(1)

if __name__ == "__main__":
    {"typing": run_typing, "desktop": run_desktop, "focus": run_focus, "failopen": run_failopen}[sys.argv[1]]()
