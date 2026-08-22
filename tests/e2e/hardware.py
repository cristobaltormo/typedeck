#!/usr/bin/env python3
"""End-to-end tests against the real board. Run ON the Mac with Typedeck started with TYPEDECK_DEV=1.
Usage: python3 hardware.py [--typing]   (--typing types into TextEdit)"""
import json, os, re, subprocess, sys, time, urllib.request, glob, select, termios, signal

BASE = "http://127.0.0.1:7788"
html = urllib.request.urlopen(BASE + "/").read().decode()
TOKEN = re.search(r'name="token" content="([^"]+)"', html).group(1)
results = []

def api(path, body=None, method=None):
    req = urllib.request.Request(BASE + path, data=None if body is None else json.dumps(body).encode(),
                                 method=method or ("POST" if body is not None else "GET"),
                                 headers={"X-Token": TOKEN, "Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=20) as r:
        return json.loads(r.read())

def check(name, cond, extra=""):
    results.append((name, bool(cond)))
    print(("PASS  " if cond else "FALLA ") + name + (f"   [{extra}]" if extra and not cond else ""), flush=True)

def raw(line, multi=False):
    r = api("/api/dev/raw", {"Line": line, "Multi": multi})
    return r["reply"] or []

def events_since(i):
    return api(f"/api/events?since={i}")["events"]

def last_id():
    return api("/api/status")["last_id"]

def wait_events(since, pred, timeout=3):
    end = time.time() + timeout
    while time.time() < end:
        evs = events_since(since)
        if pred(evs):
            return evs
        time.sleep(0.05)
    return events_since(since)

def osa(script):
    return subprocess.run(["osascript", "-e", script], capture_output=True, text=True, timeout=15).stdout.strip()

st = api("/api/status")
check("la placa esta conectada", st["connected"] and int(st["firmware"]) >= 4, st)
kb = api("/api/keyboard")
info = kb.get("info", {})
check("el teclado se identifica (VID, PID, fabricante, producto)", info.get("vid") == "258A" and info.get("pid") == "0016" and info.get("prod") and info.get("mfr"), info)
kinds = {r["kind"] for r in info.get("reports", [])}
check("los descriptores HID se interpretan (teclado, multimedia, NKRO, ratón, sistema)", {"keyboard", "consumer", "nkro", "mouse", "system"} <= kinds, kinds)

setup = api("/api/setup")
sysinfo = (setup.get("board") or {}).get("sys") or {}
check("SYS: microcontrolador y tension de la placa", sysinfo.get("mcu") == "atmega32u4" and 4500 <= sysinfo.get("vcc_mv", 0) <= 5400, sysinfo)
check("SYS: chip del shield MAX3421E", sysinfo.get("max3421e_rev") in ("12", "13"), sysinfo)
ident = setup["keyboard"]["identity"]
check("identidad del teclado: marca, modelo y fabricante del chip", ident["brand"] == "BY Tech" and "Gaming" in ident["model"] and ident["vendor"] == "SINO WEALTH", ident)
check("disposicion deducida: 100% ISO verificada", setup["keyboard"]["layout"]["id"] == "full-iso" and setup["keyboard"]["layout"]["confident"], setup["keyboard"]["layout"])
check("las comprobaciones de compatibilidad no dan errores", not [c for c in setup["checks"] if c["level"] == "error"], setup["checks"])
check("hay 11 disposiciones fisicas", len(api("/api/layouts")) == 11)
packs = api("/api/packs")["packs"]
bad = []
for p in packs:
    r = api("/api/packs/resolve", {"id": p["id"], "layout": "full-iso", "lang": "es"})
    if not r["layer"]["keys"] or r["dropped"] > len(p["macros"]) // 2:
        bad.append(p["id"])
check(f"los {len(packs)} paquetes se colocan sobre tu teclado", not bad, bad)
cfg0 = api("/api/config")
r = api("/api/packs/resolve", {"id": "obs-discord", "layout": "full-iso", "lang": "es"})
trial = json.loads(json.dumps(cfg0)); trial["layers"].append(r["layer"])
check("un paquete aplicado produce una configuracion que el servidor acepta", api("/api/config", trial).get("ok"))
api("/api/config", cfg0)

cfg = api("/api/config")
base_cfg = json.loads(json.dumps(cfg))
cfg["layers"][0]["keys"] = {"04": {"tap": {"type": "wait", "ms": 10}, "label": "prueba A"},
                            "05": {"tap": {"type": "wait", "ms": 10}, "double": {"type": "wait", "ms": 11}}}
r = api("/api/config", cfg)
check("la configuracion se guarda y valida", r.get("ok"), r)
time.sleep(0.6)
stats = raw("STATS")[0]
check("la placa captura teclas mientras hay latido", "captura=1" in stats, stats)

i0 = last_id()
raw("SIMQ 0000040000000000")
evs = wait_events(i0, lambda e: any(x["kind"] == "exec" for x in e))
kinds_seen = [(e["kind"], e.get("key")) for e in evs]
check("una tecla capturada llega como evento 'down' y ejecuta su accion", ("down", "04") in kinds_seen and any(e["kind"] == "exec" and e.get("ok") for e in evs), kinds_seen)
raw("SIMQ 0000000000000000")
evs = wait_events(i0, lambda e: any(x["kind"] == "up" for x in e))
check("al soltarla llega 'up'", any(e["kind"] == "up" and e["key"] == "04" for e in evs))

i1 = last_id()
raw("SIMQ 00000B0000000000")
time.sleep(0.4)
raw("SIMQ 0000000000000000")
evs = events_since(i1)
check("una tecla sin accion no se captura ni genera eventos", not any(e["kind"] in ("down", "exec") for e in evs), evs)

cfg2 = json.loads(json.dumps(cfg))
cfg2["layers"][1]["keys"] = {"06": {"tap": {"type": "wait", "ms": 10}}}
api("/api/config", cfg2)
time.sleep(0.5)
i2 = last_id()
raw("SIMQ 0000060000000000"); time.sleep(0.3); raw("SIMQ 0000000000000000")
check("una tecla que solo existe en otra capa pasa de largo", not any(e["kind"] == "down" for e in events_since(i2)))
api("/api/layer", {"index": 1}); time.sleep(0.6)
i3 = last_id()
raw("SIMQ 0000060000000000"); time.sleep(0.4); raw("SIMQ 0000000000000000")
check("al cambiar de capa se captura la nueva tecla (mascara sincronizada)", any(e["kind"] == "down" and e["key"] == "06" for e in events_since(i3)))
api("/api/layer", {"index": 0}); time.sleep(0.4)

api("/api/config", cfg); time.sleep(0.5)
i4 = last_id()
for _ in range(2):
    raw("SIMQ 0000050000000000"); time.sleep(0.04); raw("SIMQ 0000000000000000"); time.sleep(0.05)
evs = wait_events(i4, lambda e: any(x["kind"] == "exec" for x in e), 2)
ex = [e for e in evs if e["kind"] == "exec"]
check("doble pulsacion detectada de verdad (via placa)", len(ex) == 1 and ex[0]["gesture"] == "double", ex)

raw("STATS")
for _ in range(30):
    raw("SIMQ 0000040000000000"); raw("SIMQ 0000000000000000")
s = raw("STATS")[0]
m = re.search(r"media_us=(\d+) max_us=(\d+)", s)
check("latencia interna del firmware < 2 ms de media", m and int(m.group(1)) < 2000, s)
print("      latencia interna:", s)

pid = subprocess.run(["pgrep", "-x", "typedeck"], capture_output=True, text=True).stdout.split()[0]
os.kill(int(pid), signal.SIGSTOP)
time.sleep(7)
port = glob.glob("/dev/cu.usbmodem*")[0]
fd = os.open(port, os.O_RDWR | os.O_NOCTTY)
a = termios.tcgetattr(fd); a[4] = a[5] = termios.B115200; a[0] = a[1] = a[3] = 0; a[2] = termios.CS8 | termios.CREAD | termios.CLOCAL; termios.tcsetattr(fd, termios.TCSANOW, a)
os.write(fd, b"STATS\n")
out = b""; end = time.time() + 1.5
while time.time() < end:
    if select.select([fd], [], [], 0.1)[0]:
        out += os.read(fd, 512)
os.close(fd)
os.kill(int(pid), signal.SIGCONT)
check("sin latido durante 5 s la placa deja de capturar (fail-open)", b"captura=0" in out, out)
time.sleep(2.5)
check("al volver el programa se recupera la captura", "captura=1" in raw("STATS")[0])

def front():
    asn = subprocess.run(["lsappinfo", "front"], capture_output=True, text=True).stdout.strip()
    out = subprocess.run(["lsappinfo", "info", "-only", "name", asn], capture_output=True, text=True).stdout
    m = re.match(r'\s*"([^"]+)"', out)
    return m.group(1) if m else ""

if "--typing" in sys.argv:
    # Seguridad: solo se teclea si TextEdit esta delante; si no, se omite todo (nunca se escribe en otra app).
    prefs = ["NSAutomaticQuoteSubstitutionEnabled", "NSAutomaticDashSubstitutionEnabled", "NSAutomaticSpellingCorrectionEnabled",
             "NSAutomaticTextReplacementEnabled", "NSAutomaticCapitalizationEnabled", "NSAutomaticPeriodSubstitutionEnabled", "NSAutomaticTextCompletionEnabled"]
    for k in prefs:
        subprocess.run(f"defaults write com.apple.TextEdit {k} -bool false", shell=True)
    subprocess.run("defaults write com.apple.TextEdit RichText -int 0", shell=True)
    subprocess.run("open -a TextEdit", shell=True)
    for _ in range(24):
        time.sleep(0.5)
        if front() == "TextEdit":
            break
    ready = front() == "TextEdit"
    check("TextEdit queda delante (necesario para teclear sin riesgo)", ready, front())
    if ready:
        time.sleep(1)
        osa('tell application "TextEdit" to make new document')
        time.sleep(1.2)
        def doc():
            return osa('tell application "TextEdit" to get text of document 1')
        def clear():
            osa('tell application "TextEdit" to set text of document 1 to ""'); time.sleep(0.2)
        def typed(text):
            clear()
            if front() != "TextEdit":
                return None
            api("/api/test", {"type": "text", "text": text}); time.sleep(0.4)
            return doc()
        samples = ["Hola mundo 123", "Hola, ¿qué tal? Ñandú ñ ç", "@#{}[]\\|~ <> - _ ; : . , ' \" ! $ % & / ( ) = ? * +",
                   "á é í ó ú ü Á É", "ABC def GHI", "línea 1\nlínea 2\tfin"]
        for text in samples:
            got = typed(text)
            check(f"escribir por hardware: {text[:30]!r}", got == text, f"salio {got!r}")
        clear()
        if front() == "TextEdit":
            api("/api/test", {"type": "text", "text": "abc"})
            r1 = api("/api/test", {"type": "hotkey", "keys": "cmd+a"})
            api("/api/test", {"type": "text", "text": "z"}); time.sleep(0.4)
            got = doc()
            check("atajo con modificador (cmd+a selecciona y 'z' reemplaza)", r1["ok"] and got == "z", got)
            clear()
            api("/api/test", {"type": "text", "text": "hola"})
            api("/api/test", {"type": "hotkey", "keys": "cmd+shift+left"}); time.sleep(0.2)
            api("/api/test", {"type": "hotkey", "keys": "delete"}); time.sleep(0.3)
            check("combinaciones con varios modificadores y teclas especiales", doc() == "", doc())
        osa('tell application "TextEdit" to close every document saving no')
        osa('tell application "TextEdit" to quit saving no')
    for k in prefs:
        subprocess.run(f"defaults delete com.apple.TextEdit {k} 2>/dev/null", shell=True)
    subprocess.run("defaults delete com.apple.TextEdit RichText 2>/dev/null", shell=True)

    v0 = int(osa("output volume of (get volume settings)"))
    api("/api/test", {"type": "media", "cmd": "voldown"}); time.sleep(0.6)
    v1 = int(osa("output volume of (get volume settings)"))
    api("/api/test", {"type": "media", "cmd": "volup"}); time.sleep(0.6)
    v2 = int(osa("output volume of (get volume settings)"))
    check("teclas multimedia por hardware cambian el volumen", v1 < v0 and v2 > v1, (v0, v1, v2))

api("/api/config", base_cfg)
print()
failed = [n for n, ok in results if not ok]
print(f"{len(results) - len(failed)}/{len(results)} pruebas correctas")
sys.exit(1 if failed else 0)
