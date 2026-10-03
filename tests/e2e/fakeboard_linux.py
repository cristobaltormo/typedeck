#!/usr/bin/env python3
"""End-to-end test on Linux without hardware: a fake board on a pseudo terminal speaks the serial protocol, and the test
checks that Typedeck finds it, runs actions, types through it and answers on the API.
Usage: tests/e2e/fakeboard_linux.py /path/to/typedeck   (uses a temporary HOME, never touches the real configuration)"""
import json, os, pty, re, subprocess, sys, tempfile, threading, time, tty, urllib.request

BIN = sys.argv[1]
home = tempfile.mkdtemp(prefix="typedeck-e2e-")
results = []
def check(name, cond, extra=""):
    results.append(bool(cond)); print(("PASS  " if cond else "FAIL  ") + name + (f"   [{extra}]" if extra and not cond else ""), flush=True)

master, slave = pty.openpty(); tty.setraw(master)
link = os.path.join(home, "ttyACM-fake"); os.symlink(os.ttyname(slave), link)
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
            elif l == "STATS": say("n=1 avg_us=120 max_us=150 hid_ready=1 state=0x90 capture=1")
            elif l.startswith(("MASK ", "KEY ", "CONS ", "WATCH ", "LAYER ", "LEDS ", "DBG ", "L ")): say("OK")
threading.Thread(target=firmware, daemon=True).start()
def sent(prefix): 
    with lock: return [l for l in got if l.startswith(prefix)]

env = {**os.environ, "HOME": home, "XDG_CONFIG_HOME": os.path.join(home, "cfg"), "TYPEDECK_PORT": link}
for k in ("DISPLAY", "WAYLAND_DISPLAY"): env.pop(k, None)
proc = subprocess.Popen([BIN], env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
def port():
    for _ in range(100):
        try: return int(open(os.path.join(home, "cfg", "typedeck", "port")).read())
        except Exception: time.sleep(0.1)
    sys.exit("no arranca")
PORT = port(); B = f"http://127.0.0.1:{PORT}"
time.sleep(0.5)
TOKEN = re.search(r'name="token" content="([^"]+)"', urllib.request.urlopen(B + "/").read().decode()).group(1)
def api(p, b=None):
    r = urllib.request.Request(B + p, data=None if b is None else json.dumps(b).encode(), method="POST" if b is not None else "GET", headers={"X-Token": TOKEN, "Content-Type": "application/json"})
    return json.loads(urllib.request.urlopen(r, timeout=20).read())
def wait(cond, secs=6):
    t = time.time()
    while time.time() - t < secs:
        if cond(): return True
        time.sleep(0.1)
    return False

try:
    check("the fake board is found through TYPEDECK_PORT", wait(lambda: api("/api/status")["connected"]))
    wait(lambda: api("/api/setup")["keyboard"], 15)
    st = api("/api/setup")
    check("the system identifies as linux", st["host"]["os"] == "linux", st["host"])
    check("the keyboard is recognized", st["keyboard"]["present"] and "258A" in json.dumps(st["keyboard"]))
    check("there are no compatibility errors", not [c for c in st["checks"] if c["level"] == "error"], st["checks"])

    marker = os.path.join(home, "marker")
    cfg = api("/api/config")
    cfg["settings"]["typing_layout"] = "es-pc"; cfg["settings"]["obs"]["port"] = 1
    cfg["layers"][0]["keys"] = {
        "04": {"tap": {"type": "shell", "cmd": f"echo hola > '{marker}'"}},
        "05": {"tap": {"type": "hotkey", "keys": "ctrl+shift+4"}},
        "06": {"tap": {"type": "text", "text": "Añ@~€\\{"}},
        "07": {"tap": {"type": "media", "cmd": "playpause"}, "double": {"type": "media", "cmd": "next"}},
        "08": {"tap": {"type": "obs", "cmd": "stream"}},
        "09": {"tap": {"type": "system", "cmd": "caffeinate"}},
    }
    check("the configuration is accepted", api("/api/config", cfg).get("ok"))
    check("the board receives the mask of captured keys", wait(lambda: any(l.startswith("MASK ") and l != "MASK " + "0" * 64 for l in sent("MASK "))))
    time.sleep(0.5)

    say("D 04 00"); say("U 04")
    check("a captured key runs its shell command", wait(lambda: os.path.exists(marker)))

    n = len(sent("KEY "))
    say("D 05 00"); say("U 05")
    check("a shortcut goes out through the board with its modifiers (ctrl+shift+4 = 03 21)", wait(lambda: "KEY 03 21" in sent("KEY ")), sent("KEY "))

    n = len(sent("KEY "))
    say("D 06 00"); say("U 06")
    wait(lambda: len(sent("KEY ")) >= n + 10)
    typed = sent("KEY ")[n:]
    # the sample text is Añ@~€\{ -> A=shift+04, ñ=00 33, @=40 1F (AltGr+2), ~=40 21, €=40 22, \=40 35, {=40 34
    want = ["KEY 02 04", "KEY 00 33", "KEY 40 1F", "KEY 40 21", "KEY 40 22", "KEY 40 35", "KEY 40 34"]
    check("text is typed with the Spanish (PC) layout: Alt Gr for @ ~ € \\ {", typed[:7] == want, typed[:8])

    say("D 07 00"); say("U 07")
    check("the media key goes out through the board", wait(lambda: "CONS CD" in sent("CONS ")), sent("CONS "))

    ev0 = api("/api/status")["last_id"]
    say("D 08 00"); say("U 08")
    ok = wait(lambda: any(e["kind"] == "exec" and e["key"] == "08" for e in api(f"/api/events?since={ev0}&wait=1")["events"]) or True, 3)
    evs = api(f"/api/events?since=0")["events"]
    ex = [e for e in evs if e["kind"] == "exec" and e["key"] == "08"]
    check("an OBS action without OBS fails with a clear message", ex and ex[-1]["ok"] is False and "WebSocket" in ex[-1].get("output", ""), ex[-1:] if ex else evs[-3:])

    r = api("/api/test", {"type": "system", "cmd": "darkmode"})
    check("a system command without a desktop answers with an error, without hanging", r["ok"] is False and r["ms"] < 5000, r)
    r = api("/api/test", {"type": "app", "app": "NoExisteEstaApp", "mode": "open"})
    check("opening a missing app gives a clear error", r["ok"] is False and "NoExisteEstaApp" in r["output"], r)
    r = api("/api/test", {"type": "url", "url": "https://example.com"})
    check("opening a link without xdg-open or a desktop does not hang", r["ms"] < 12000, r)
    apps = api("/api/apps")
    check("the application list is a list", isinstance(apps, list))

    api("/api/board", {"cmd": "leds", "arg": 1})
    proc.terminate(); proc.wait(5)
    check("it shuts down cleanly on SIGTERM", proc.returncode in (0, -15, None), proc.returncode)
finally:
    if proc.poll() is None: proc.kill()
    out = proc.stdout.read().decode(errors="ignore") if proc.stdout else ""
    if not all(results): print("--- program log ---\n" + out[-1500:])
print(f"\n{sum(results)}/{len(results)} checks passed"); sys.exit(0 if all(results) else 1)
