#!/usr/bin/env python3
"""Manual tests on a REAL Windows machine with the board plugged in (not run in CI).
Needs Typedeck running in the user session (typedeck install), the session unlocked, the editor reachable on 127.0.0.1:7788
(for example ssh -L 7788:127.0.0.1:7788 windows) and tests/e2e/windows_fg.ps1 copied to WIN_FG_SCRIPT.
Types into Notepad, and only when it is in front. Usage: windows_real.py typing|desktop"""
import sys, re, json, time, os, urllib.request
FG_SCRIPT = os.environ.get("WIN_FG_SCRIPT", r"C:\typedeck\fg.ps1")
B="http://127.0.0.1:7788"
T=re.search(r'name="token" content="([^"]+)"',urllib.request.urlopen(B+"/").read().decode()).group(1)
def api(p,b=None):
    r=urllib.request.Request(B+p,data=None if b is None else json.dumps(b).encode(),method="POST" if b is not None else "GET",headers={"X-Token":T,"Content-Type":"application/json"})
    return json.loads(urllib.request.urlopen(r,timeout=30).read())

def run_typing():
    FG = 'powershell -NoProfile -ExecutionPolicy Bypass -File ' + FG_SCRIPT
    def sh(cmd): return api("/api/test", {"type": "shell", "cmd": cmd, "show_output": True})
    def front(): return sh(FG)["output"].strip().lower()
    res = []
    def check(n, c, x=""): res.append(bool(c)); print(("PASS  " if c else "FAIL  ") + n + (f"   [{x}]" if x and not c else ""), flush=True)
    cfg = api("/api/config"); cfg["settings"]["typing_layout"] = "auto"; cfg["settings"]["input"] = "auto"; api("/api/config", cfg)
    check("the board is connected en COM4", api("/api/status")["connected"])
    print("layout:", api("/api/setup")["host"]["typing_layout"])
    r = api("/api/test", {"type": "app", "app": "notepad", "mode": "open"}); print("abrir notepad:", r)
    for _ in range(20):
        time.sleep(0.7)
        sh('powershell -NoProfile -Command "$w=New-Object -ComObject WScript.Shell; $w.SendKeys(\'%\'); Start-Sleep -Milliseconds 200; $w.AppActivate(\'Bloc de notas\')"')
        if "notepad" in front(): break
    f = front(); check("Notepad is in front (needed to type)", "notepad" in f, f)
    if "notepad" not in f: raise SystemExit("aborto: no escribo si no esta delante")
    def hk(k): return api("/api/test", {"type": "hotkey", "keys": k})
    def clip():
        return sh('powershell -NoProfile -Command "[Console]::OutputEncoding=[Text.Encoding]::UTF8; Get-Clipboard -Raw"')["output"]
    def typed(text):
        hk("ctrl+a"); hk("backspace"); time.sleep(0.2)
        api("/api/test", {"type": "text", "text": text}); time.sleep(0.5)
        hk("ctrl+a"); hk("ctrl+c"); time.sleep(0.5)
        return clip().replace("\r\n", "\n").rstrip("\n")
    for t in ["Hola mundo 123", "Hola, ¿qué tal? Ñandú ñ ç", "@#{}[]\\|~ <> - _ ; : . , ' \" !", "á é í ó ú ü Á É", "ABC def GHI", "€ ¬ º ª", "` ^ ´ ¨ ~n ^a", "Línea con ~/tmp y C:\\Users\\x"]:
        got = typed(t); check(f"escribir por la placa: {t!r}", got == t, repr(got))
    hk("ctrl+a"); hk("backspace")
    api("/api/test", {"type": "text", "text": "x"}); hk("ctrl+a"); hk("ctrl+x")
    check("ctrl+x shortcut through the board (cmd=ctrl does not apply: ctrl is used)", clip().strip() == "x")
    hk("ctrl+a"); hk("backspace")
    sh('taskkill /F /IM notepad.exe')
    print(f"\n{sum(res)}/{len(res)}")

def run_desktop():
    FG = 'powershell -NoProfile -ExecutionPolicy Bypass -File ' + FG_SCRIPT
    def sh(cmd): return api("/api/test", {"type": "shell", "cmd": cmd, "show_output": True})["output"].strip()
    def front(): return sh(FG).lower()
    res = []
    def check(n, c, x=""): res.append(bool(c)); print(("PASS  " if c else "FAIL  ") + n + (f"   [{x}]" if x and not c else ""), flush=True)
    def activate_proc(proc):
        sh('powershell -NoProfile -Command "$t=(Get-Process %s -ErrorAction SilentlyContinue | Where-Object MainWindowTitle | Select -First 1).MainWindowTitle; if($t){$w=New-Object -ComObject WScript.Shell; $w.SendKeys(\'%%\'); Start-Sleep -Milliseconds 200; [void]$w.AppActivate($t)}"' % proc)
    def clip(): return sh('powershell -NoProfile -Command "[Console]::OutputEncoding=[Text.Encoding]::UTF8; Get-Clipboard -Raw"').replace("\r\n", "\n").rstrip("\n")
    def hk(k): return api("/api/test", {"type": "hotkey", "keys": k})
    orig = api("/api/config")
    try:
        cfg = json.loads(json.dumps(orig)); cfg["layers"].append({"name": "PruebaChrome", "color": "", "icon": "", "auto_apps": ["Chrome"], "keys": {}}); cfg["settings"]["hud"]["on_auto"] = True
        api("/api/config", cfg); want = len(cfg["layers"]) - 1
        activate_proc("chrome"); time.sleep(1.5)
        names = api("/api/status").get("front")
        check("the front application is detected (GetForegroundWindow + Start menu)", names and any("chrome" in n.lower() for n in names), names)
        lay = None
        for _ in range(12):
            lay = api("/api/status").get("layer")
            if lay == want: break
            time.sleep(0.5)
        check("the per-application layer activates on its own with Chrome in front", lay == want, (lay, want))
        api("/api/config", orig)

        sh("taskkill /F /IM notepad.exe"); time.sleep(1)
        r = api("/api/test", {"type": "app", "app": "notepad", "mode": "open"})
        for _ in range(15):
            time.sleep(0.8); activate_proc("Notepad")
            if "notepad" in front(): break
        check("open notepad and keep it in front", r["ok"] and "notepad" in front(), (r, front()))
        if "notepad" not in front(): raise SystemExit("aborto: no escribo si Notepad no esta delante")
        c2 = api("/api/config"); c2["settings"]["input"] = "software"; api("/api/config", c2)
        hk("ctrl+a"); hk("backspace"); time.sleep(0.3)
        api("/api/test", {"type": "text", "text": "Hola ñ @ € por software"}); time.sleep(0.8)
        hk("ctrl+a"); hk("ctrl+c"); time.sleep(0.5)
        got = clip(); check("software text (clipboard + Ctrl+V)", got == "Hola ñ @ € por software", repr(got))
        hk("ctrl+a"); hk("backspace")
        api("/api/test", {"type": "text", "text": "abc"}); hk("ctrl+a"); hk("ctrl+c"); time.sleep(0.4)
        check("software shortcut (ctrl+a, ctrl+c with keybd_event)", clip() == "abc", clip())
        hk("ctrl+a"); hk("backspace")
        c3 = api("/api/config"); c3["settings"]["input"] = "auto"; api("/api/config", c3)

        time.sleep(1)
        r = api("/api/test", {"type": "app", "app": "Notepad", "mode": "toggle"}); time.sleep(1.2)
        check("toggle: with the app in front it minimizes it", r["ok"] and "notepad" not in front(), (r, front()))
        r = api("/api/test", {"type": "app", "app": "Bloc de notas", "mode": "quit"}); time.sleep(2)
        left = sh('powershell -NoProfile -Command "(Get-Process Notepad -ErrorAction SilentlyContinue | Where-Object MainWindowTitle | Measure-Object).Count"')
        check("close the app by its window title (WM_CLOSE)", r["ok"] and left.strip() == "0", (r, left))
    finally:
        sh("taskkill /F /IM notepad.exe"); api("/api/config", orig)
    print(f"\n{sum(res)}/{len(res)}")

if __name__ == "__main__":
    {"typing": run_typing, "desktop": run_desktop}[sys.argv[1]]()
