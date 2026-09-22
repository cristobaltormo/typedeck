#!/usr/bin/env python3
"""Windows build under Wine with a fake board: Wine's COM3 points at a pseudo terminal where a fake firmware speaks the
protocol. Checks the native Windows serial port (DCB, DTR, read timeouts), the API, the Windows Spanish layout and a clean
shutdown. Needs: wine, Xvfb.   Usage: fakeboard_wine.py /path/to/typedeck.exe"""
import json, os, pty, re, subprocess, sys, tempfile, threading, time, tty, urllib.request

EXE = os.path.abspath(sys.argv[1])
prefix = os.environ.get("WINEPREFIX", os.path.expanduser("~/.wine"))
work = tempfile.mkdtemp(prefix="typedeck-wine-")
results = []
def check(name, cond, extra=""):
    results.append(bool(cond)); print(("PASS  " if cond else "FALLA ") + name + (f"   [{extra}]" if extra and not cond else ""), flush=True)

master, slave = pty.openpty(); tty.setraw(master)
com = os.path.join(prefix, "dosdevices", "com3")
if os.path.lexists(com): os.remove(com)
os.symlink(os.ttyname(slave), com)
got, lock = [], threading.Lock()
def say(s): os.write(master, (s + "\n").encode())
def firmware():
    buf = b""
    while True:
        try: d = os.read(master, 256)
        except OSError: return
        buf += d
        while b"\n" in buf:
            line, buf = buf.split(b"\n", 1); l = line.decode(errors="ignore").strip()
            with lock: got.append(l)
            if l == "WHO": say("TYPEDECK-FW 4")
            elif l == "INFO":
                say('vid=258A pid=0016 bcd=0001 usb=0200 class=00 ep0=8 mfr="BY Tech" prod="Usb Gaming Keyboard" serial="" cfglen=59 ifaces=2 power=500 hidlen0=67 hidlen1=227'); say("END")
            elif l.startswith("RDESC 0"): say("05 01 09 06 A1 01 05 07 19 E0 29 E7 15 00 25 01 95 08 75 01 81 02 95 01 75 08 81 03 95 06 75 08 15 00 26 FF 00 05 07 19 00 2A FF 00 81 00 C0"); say("END")
            elif l.startswith("RDESC"): say("05 0C 09 01 A1 01 85 02 19 00 2A FF 02 15 00 26 FF 7F 95 01 75 10 81 00 C0"); say("END")
            elif l == "SYS": say("mcu=atmega32u4 f_cpu=16000000 board=leonardo fw=4 vcc_mv=5000 free_ram=900 max3421e_rev=3 uptime_s=10")
            elif l == "STATS": say("n=1 media_us=120 max_us=150 hid_listo=1 estado=0x90 captura=1")
            elif l.startswith(("MASK ", "KEY ", "CONS ", "WATCH ", "LAYER ", "LEDS ", "DBG ", "L ")): say("OK")
threading.Thread(target=firmware, daemon=True).start()
def sent(prefix):
    with lock: return [l for l in got if l.startswith(prefix)]

import glob
winpath = lambda p: "Z:" + p.replace("/", "\\")
env = {**os.environ, "WINEDEBUG": "-all", "TYPEDECK_PORT": "COM3"}
for old in glob.glob(os.path.join(prefix, "drive_c", "users", "*", "AppData", "Roaming", "typedeck")):
    subprocess.run(["rm", "-rf", old])
proc = subprocess.Popen(["wine", EXE], env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
portfile = None
for _ in range(300):
    m = glob.glob(os.path.join(prefix, "drive_c", "users", "*", "AppData", "Roaming", "typedeck", "port"))
    if m: portfile = m[0]; break
    time.sleep(0.2)
else: sys.exit("el programa no arranco bajo Wine")
B = f"http://127.0.0.1:{int(open(portfile).read())}"; time.sleep(0.5)
TOKEN = re.search(r'name="token" content="([^"]+)"', urllib.request.urlopen(B + "/").read().decode()).group(1)
def api(p, b=None):
    r = urllib.request.Request(B + p, data=None if b is None else json.dumps(b).encode(), method="POST" if b is not None else "GET", headers={"X-Token": TOKEN, "Content-Type": "application/json"})
    return json.loads(urllib.request.urlopen(r, timeout=30).read())
def wait(cond, secs=8):
    t = time.time()
    while time.time() - t < secs:
        if cond(): return True
        time.sleep(0.1)
    return False

try:
    check("la placa se encuentra en COM3 con el puerto nativo de Windows", wait(lambda: api("/api/status")["connected"], 15), proc.poll())
    wait(lambda: api("/api/setup")["keyboard"], 15)
    st = api("/api/setup")
    check("el sistema se identifica como windows", st["host"]["os"] == "windows", st["host"])
    check("el teclado se reconoce", st["keyboard"]["present"] and "258A" in json.dumps(st["keyboard"]))
    check("no hay errores de compatibilidad", not [c for c in st["checks"] if c["level"] == "error"], st["checks"])

    marker = os.path.join(work, "marker.txt")
    cfg = api("/api/config")
    cfg["settings"]["typing_layout"] = "es-win"
    cfg["layers"][0]["keys"] = {
        "04": {"tap": {"type": "shell", "cmd": f'echo hola> "{winpath(marker)}"'}},
        "05": {"tap": {"type": "hotkey", "keys": "cmd+shift+4"}},
        "06": {"tap": {"type": "text", "text": "Añ@~€\\{"}},
        "07": {"tap": {"type": "media", "cmd": "playpause"}},
    }
    check("la configuración se acepta", api("/api/config", cfg).get("ok"))
    check("la placa recibe la máscara de teclas capturadas", wait(lambda: any(l.startswith("MASK ") and l != "MASK " + "0" * 64 for l in sent("MASK "))))
    time.sleep(0.5)
    if os.environ.get("DEBUG"): print(api("/api/test", {"type": "shell", "cmd": f'echo hola> "{winpath(marker)}" & echo %CD%', "show_output": True}))
    say("D 04 00"); say("U 04")
    check("una tecla capturada ejecuta su comando con cmd /C", wait(lambda: os.path.exists(marker), 10), os.listdir(work))
    say("D 05 00"); say("U 05")
    check("un atajo sale por la placa (cmd = tecla GUI, 08+02 = 0A)", wait(lambda: "KEY 0A 21" in sent("KEY ")), sent("KEY "))
    n = len(sent("KEY "))
    say("D 06 00"); say("U 06")
    wait(lambda: len(sent("KEY ")) >= n + 8)
    want = ["KEY 02 04", "KEY 00 33", "KEY 40 1F", "KEY 40 21", "KEY 00 2C", "KEY 40 22", "KEY 40 35", "KEY 40 34"]
    check("el texto se teclea con Español (Windows): Alt Gr para @ ~ € \\ {", sent("KEY ")[n:n + 8] == want, sent("KEY ")[n:n + 9])
    say("D 07 00"); say("U 07")
    check("la tecla multimedia sale por la placa", wait(lambda: "CONS CD" in sent("CONS ")), sent("CONS "))
    r = api("/api/test", {"type": "system", "cmd": "caffeinate"})
    check("caffeinate (SetThreadExecutionState) responde", r["ok"], r)
    r = api("/api/test", {"type": "system", "cmd": "caffeinate"})
    check("caffeinate se desactiva", r["ok"], r)
    apps = api("/api/apps")
    check("la lista de aplicaciones es una lista", isinstance(apps, list))
    r = api("/api/test", {"type": "app", "app": "notepad", "mode": "open"}); time.sleep(2)
    check("abrir un ejecutable del sistema (cmd /C start)", r["ok"], r)
    r = api("/api/test", {"type": "app", "app": "notepad", "mode": "quit"}); time.sleep(1)
    check("cerrar con taskkill responde sin colgarse", r["ms"] < 8000, r)
    r = api("/api/test", {"type": "url", "url": "ftp://x"})
    check("un enlace que no es web se rechaza", r["ok"] is False, r)
    proc.terminate(); proc.wait(10)
    check("termina al recibir la señal", proc.poll() is not None)
finally:
    if proc.poll() is None: proc.kill()
    subprocess.run(["wineserver", "-k"], env=env)
    if os.path.lexists(com): os.remove(com)
    if not all(results): print("--- registro ---\n" + (proc.stdout.read().decode(errors="ignore")[-1500:] if proc.stdout else ""))
print(f"\n{sum(results)}/{len(results)} comprobaciones correctas"); sys.exit(0 if all(results) else 1)
