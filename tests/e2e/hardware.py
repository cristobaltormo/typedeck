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
    print(("PASS  " if cond else "FAIL  ") + name + (f"   [{extra}]" if extra and not cond else ""), flush=True)

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
check("the board is connected", st["connected"] and int(st["firmware"]) >= 4, st)
kb = api("/api/keyboard")
info = kb.get("info", {})
check("the keyboard is identified (VID, PID, vendor, product)", info.get("vid") == "258A" and info.get("pid") == "0016" and info.get("prod") and info.get("mfr"), info)
kinds = {r["kind"] for r in info.get("reports", [])}
check("the HID descriptors are parsed (keyboard, media, NKRO, mouse, system)", {"keyboard", "consumer", "nkro", "mouse", "system"} <= kinds, kinds)

setup = api("/api/setup")
sysinfo = (setup.get("board") or {}).get("sys") or {}
check("SYS: microcontroller and board voltage", sysinfo.get("mcu") == "atmega32u4" and 4500 <= sysinfo.get("vcc_mv", 0) <= 5400, sysinfo)
check("SYS: MAX3421E shield chip", sysinfo.get("max3421e_rev") in ("12", "13"), sysinfo)
ident = setup["keyboard"]["identity"]
check("keyboard identity: brand, model and chip vendor", ident["brand"] == "BY Tech" and "Gaming" in ident["model"] and ident["vendor"] == "SINO WEALTH", ident)
lay = setup["keyboard"]["layout"]
check("the layout is a verified full-size one (inferred or chosen by the user)", lay["id"] in ("full-iso", "full-ansi") and lay["confident"], lay)
check("the compatibility checks report no errors", not [c for c in setup["checks"] if c["level"] == "error"], setup["checks"])
check("there are 11 physical layouts", len(api("/api/layouts")) == 11)
packs = api("/api/packs")["packs"]
bad = []
for p in packs:
    r = api("/api/packs/resolve", {"id": p["id"], "layout": "full-iso", "lang": "es"})
    if not r["layer"]["keys"] or r["dropped"] > len(p["macros"]) // 2:
        bad.append(p["id"])
check(f"all {len(packs)} packs are laid out on your keyboard", not bad, bad)
cfg0 = api("/api/config")
r = api("/api/packs/resolve", {"id": "obs-discord", "layout": "full-iso", "lang": "es"})
trial = json.loads(json.dumps(cfg0)); trial["layers"].append(r["layer"])
check("an applied pack produces a configuration the server accepts", api("/api/config", trial).get("ok"))
api("/api/config", cfg0)

cfg = api("/api/config")
base_cfg = json.loads(json.dumps(cfg))
cfg["layers"][0]["keys"] = {"04": {"tap": {"type": "wait", "ms": 10}, "label": "prueba A"},
                            "05": {"tap": {"type": "wait", "ms": 10}, "double": {"type": "wait", "ms": 11}}}
r = api("/api/config", cfg)
check("the configuration is saved and validated", r.get("ok"), r)
time.sleep(0.6)
stats = raw("STATS")[0]
check("the board captures keys while there is a heartbeat", "capture=1" in stats, stats)

i0 = last_id()
raw("SIMQ 0000040000000000")
evs = wait_events(i0, lambda e: any(x["kind"] == "exec" for x in e))
kinds_seen = [(e["kind"], e.get("key")) for e in evs]
check("a captured key arrives as a 'down' event and runs its action", ("down", "04") in kinds_seen and any(e["kind"] == "exec" and e.get("ok") for e in evs), kinds_seen)
raw("SIMQ 0000000000000000")
evs = wait_events(i0, lambda e: any(x["kind"] == "up" for x in e))
check("releasing it sends 'up'", any(e["kind"] == "up" and e["key"] == "04" for e in evs))

api("/api/editing", {"on": True})
time.sleep(0.5)
i_e = last_id()
raw("SIMQ 0000040000000000")
time.sleep(0.4)
raw("SIMQ 0000000000000000")
check("with an editor field focused key A does not run its macro", not any(e["kind"] in ("down", "exec") for e in events_since(i_e)), events_since(i_e))
api("/api/editing", {"on": False})
time.sleep(0.5)
i_e = last_id()
raw("SIMQ 0000040000000000")
evs = wait_events(i_e, lambda e: any(x["kind"] == "exec" for x in e))
check("leaving the field brings capture back", any(e["kind"] == "down" and e["key"] == "04" for e in evs), evs)
raw("SIMQ 0000000000000000")

i1 = last_id()
raw("SIMQ 00000B0000000000")
time.sleep(0.4)
raw("SIMQ 0000000000000000")
evs = events_since(i1)
check("a key without an action is neither captured nor reported", not any(e["kind"] in ("down", "exec") for e in evs), evs)

cfg_h = json.loads(json.dumps(cfg))
cfg_h["settings"]["key_history"] = True
api("/api/config", cfg_h)
time.sleep(0.8)
raw("SIMQ 00000B0000000000")
time.sleep(0.35)
raw("SIMQ 0000000000000000")
raw("SIMQ 0000040000000000")
time.sleep(0.12)
raw("SIMQ 0000000000000000")
time.sleep(0.4)
hist = api("/api/history?limit=10")
check("with the history on, keys without a macro are stored with their duration", hist["enabled"] and any(e["k"] == "H" and e["d"] >= 250 for e in hist["entries"]), hist)
check("and so are the keys with a macro", any(e["k"] == "A" for e in hist["entries"]), hist)
api("/api/history/clear", {})
check("delete everything empties the history", api("/api/history")["count"] <= 3)
cfg_off = json.loads(json.dumps(cfg))
cfg_off["settings"]["key_history"] = False
api("/api/config", cfg_off)
time.sleep(0.5)
before = api("/api/history")["count"]
raw("SIMQ 00000B0000000000"); time.sleep(0.2); raw("SIMQ 0000000000000000"); time.sleep(0.3)
check("with the history off nothing is stored", api("/api/history")["count"] == before)

api("/api/learn", {"on": True})
time.sleep(0.6)
i_l = last_id()
raw("SIMQ 0000040000000000"); time.sleep(0.2); raw("SIMQ 0000000000000000")
raw("SIMQ 0000310000000000"); time.sleep(0.2); raw("SIMQ 0000000000000000")
time.sleep(0.3)
evs = events_since(i_l)
seen = api("/api/learn")["seen"]
check("while detecting keys, every key is noted and none runs its macro", seen.get("04") == 1 and seen.get("31") == 1 and not any(e["kind"] == "exec" for e in evs), (seen, evs))
api("/api/learn", {"on": False})
time.sleep(0.5)

cfg2 = json.loads(json.dumps(cfg))
cfg2["layers"][1]["keys"] = {"06": {"tap": {"type": "wait", "ms": 10}}}
api("/api/config", cfg2)
time.sleep(0.5)
i2 = last_id()
raw("SIMQ 0000060000000000"); time.sleep(0.3); raw("SIMQ 0000000000000000")
check("a key that only exists on another layer passes through", not any(e["kind"] == "down" for e in events_since(i2)))
api("/api/layer", {"index": 1}); time.sleep(0.6)
i3 = last_id()
raw("SIMQ 0000060000000000"); time.sleep(0.4); raw("SIMQ 0000000000000000")
check("switching layers captures the new key (mask synchronized)", any(e["kind"] == "down" and e["key"] == "06" for e in events_since(i3)))
api("/api/layer", {"index": 0}); time.sleep(0.4)

cfg3 = json.loads(json.dumps(cfg))
cfg3["layers"][0]["keys"]["39"] = {"hold": {"type": "layer", "to": 1, "momentary": True}}
cfg3["layers"][1]["keys"] = {"1A": {"tap": {"type": "wait", "ms": 10}}}
api("/api/config", cfg3); time.sleep(0.6)
i5 = last_id()
raw("SIMQ 0000390000000000"); time.sleep(0.08)
raw("SIMQ 00003A1A00000000".replace("3A", "39")); time.sleep(0.15)
raw("SIMQ 0000390000000000"); time.sleep(0.1)
raw("SIMQ 0000000000000000"); time.sleep(0.4)
evs = events_since(i5)
hit = [e for e in evs if e["kind"] == "exec" and e.get("key") == "1A"]
check("holding a layer key and pressing another within 100 ms runs the key of that layer at once", len(hit) == 1 and hit[0].get("layer_name") == cfg3["layers"][1]["name"], hit or evs)
check("releasing the layer key goes back to the first layer", api("/api/status")["layer"] == 0)
cfg3["layers"][0]["keys"]["39"] = {"tap": {"type": "layer", "to": 1, "momentary": True}}
api("/api/config", cfg3); time.sleep(0.6)
i6 = last_id()
raw("SIMQ 0000391A00000000"); time.sleep(0.15)
raw("SIMQ 0000000000000000"); time.sleep(0.4)
hit = [e for e in events_since(i6) if e["kind"] == "exec" and e.get("key") == "1A"]
check("a layer key set as a modifier works with both keys arriving in the same report", len(hit) == 1 and hit[0].get("layer_name") == cfg3["layers"][1]["name"], hit)
if sys.platform == "darwin":
    def caps_on():
        return osa('use framework "Cocoa"\nreturn ((current application\'s NSEvent\'s modifierFlags() as integer) div 65536) mod 2') == "1"
    c0 = caps_on()
    raw("SIMQ 0000390000000000"); time.sleep(0.25); raw("SIMQ 0000000000000000"); time.sleep(0.7)
    c1 = caps_on()
    check("a layer key pressed alone toggles Caps Lock", c1 != c0, (c0, c1))
    raw("SIMQ 0000391A00000000"); time.sleep(0.25); raw("SIMQ 0000000000000000"); time.sleep(0.7)
    check("and used with another key it does not", caps_on() == c1)
    if c1 != c0:
        raw("SIMQ 0000390000000000"); time.sleep(0.25); raw("SIMQ 0000000000000000"); time.sleep(0.7)

if os.environ.get("TYPEDECK_REMOTE"):
    cfg["settings"]["double_ms"] = 800
api("/api/config", cfg); time.sleep(0.5)
i4 = last_id()
for _ in range(2):
    raw("SIMQ 0000050000000000"); time.sleep(0.04); raw("SIMQ 0000000000000000"); time.sleep(0.05)
evs = wait_events(i4, lambda e: any(x["kind"] == "exec" for x in e), 2)
ex = [e for e in evs if e["kind"] == "exec"]
check("double press really detected (through the board)", len(ex) == 1 and ex[0]["gesture"] == "double", ex)

raw("STATS")
for _ in range(30):
    raw("SIMQ 0000040000000000"); raw("SIMQ 0000000000000000")
s = raw("STATS")[0]
m = re.search(r"avg_us=(\d+) max_us=(\d+)", s)
check("firmware internal latency under 2 ms on average", m and int(m.group(1)) < 2000, s)
print("      latencia interna:", s)

REMOTE = bool(os.environ.get("TYPEDECK_REMOTE"))
pid = None if REMOTE else subprocess.run(["pgrep", "-x", "typedeck"], capture_output=True, text=True).stdout.split()[0]
os.kill(int(pid), signal.SIGSTOP) if pid else None
if not REMOTE:
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
    check("without a heartbeat for 5 s the board stops capturing (fail-open)", b"capture=0" in out, out)
    time.sleep(2.5)
    check("when the program returns capture comes back", "capture=1" in raw("STATS")[0])

def front():
    asn = subprocess.run(["lsappinfo", "front"], capture_output=True, text=True).stdout.strip()
    out = subprocess.run(["lsappinfo", "info", "-only", "name", asn], capture_output=True, text=True).stdout
    m = re.match(r'\s*"([^"]+)"', out)
    return m.group(1) if m else ""

if "--typing" in sys.argv and not REMOTE:
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
    check("TextEdit is in front (needed to type safely)", ready, front())
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
            check("shortcut with a modifier (cmd+a selects and 'z' replaces)", r1["ok"] and got == "z", got)
            clear()
            api("/api/test", {"type": "text", "text": "hola"})
            api("/api/test", {"type": "hotkey", "keys": "cmd+shift+left"}); time.sleep(0.2)
            api("/api/test", {"type": "hotkey", "keys": "delete"}); time.sleep(0.3)
            check("combinations with several modifiers and special keys", doc() == "", doc())
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
    check("media keys through the hardware change the volume", v1 < v0 and v2 > v1, (v0, v1, v2))

api("/api/config", base_cfg)
print()
failed = [n for n, ok in results if not ok]
print(f"{len(results) - len(failed)}/{len(results)} tests passed")
sys.exit(1 if failed else 0)
