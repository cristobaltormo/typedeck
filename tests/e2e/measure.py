#!/usr/bin/env python3
import json, os, pty, re, statistics, subprocess, sys, tempfile, threading, time, tty, urllib.request

BIN = sys.argv[1]
IDLE = float(sys.argv[2]) if len(sys.argv) > 2 else 120
home = tempfile.mkdtemp(prefix="typedeck-measure-")
master, slave = pty.openpty(); tty.setraw(master)
link = os.path.join(home, "ttyACM-fake"); os.symlink(os.ttyname(slave), link)
keys, lock = [], threading.Lock()
def say(s): os.write(master, (s + "\n").encode())
def firmware():
    buf = b""
    while True:
        try: d = os.read(master, 256)
        except OSError: return
        buf += d
        while b"\n" in buf:
            line, buf = buf.split(b"\n", 1); l = line.decode(errors="ignore").strip()
            if l == "WHO": say("TYPEDECK-FW 15")
            elif l == "INFO": say('vid=258A pid=0016 mfr="BY Tech" prod="Usb Gaming Keyboard" power=500'); say("END")
            elif l.startswith("KEY "):
                with lock: keys.append(time.perf_counter())
                say("OK")
            elif l.startswith(("MASK ", "CONS ", "KEYS ", "LAYER ")): say("OK")
threading.Thread(target=firmware, daemon=True).start()

env = {**os.environ, "HOME": home, "XDG_CONFIG_HOME": os.path.join(home, "cfg"), "TYPEDECK_PORT": link}
for k in ("DISPLAY", "WAYLAND_DISPLAY"): env.pop(k, None)
t0 = time.perf_counter()
proc = subprocess.Popen([BIN], env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
portfile = os.path.join(home, "cfg", "typedeck", "port")
while True:
    try:
        PORT = int(open(portfile).read()); urllib.request.urlopen(f"http://127.0.0.1:{PORT}/", timeout=1).read(); break
    except Exception: time.sleep(0.002)
startup = time.perf_counter() - t0
B = f"http://127.0.0.1:{PORT}"
TOKEN = re.search(r'name="token" content="([^"]+)"', urllib.request.urlopen(B + "/").read().decode()).group(1)
def api(p, b=None):
    r = urllib.request.Request(B + p, data=None if b is None else json.dumps(b).encode(), method="POST" if b is not None else "GET", headers={"X-Token": TOKEN})
    return json.loads(urllib.request.urlopen(r, timeout=10).read())
while not api("/api/status")["connected"]: time.sleep(0.05)
cfg = api("/api/config"); cfg["settings"]["typing_layout"] = "us"; cfg["layers"][0]["keys"] = {"04": {"tap": {"type": "hotkey", "keys": "ctrl+b"}}}
api("/api/config", cfg); time.sleep(1)

lat = []
for i in range(60):
    with lock: n = len(keys)
    s = time.perf_counter(); say("D 04 00"); say("U 04")
    while True:
        with lock:
            if len(keys) > n: lat.append((keys[n] - s) * 1000); break
        if time.perf_counter() - s > 2: break
        time.sleep(0.0002)
    time.sleep(0.08)

def stat():
    f = open(f"/proc/{proc.pid}/stat").read().rsplit(")", 1)[1].split()
    return int(f[11]) + int(f[12])
def status(key): return int(re.search(rf"{key}:\s+(\d+)", open(f"/proc/{proc.pid}/status").read()).group(1))
time.sleep(5)
rss_start = status("VmRSS"); c0 = stat(); w0 = time.time()
time.sleep(IDLE)
c1 = stat(); w1 = time.time()
tick = os.sysconf("SC_CLK_TCK")
out = {
    "binary": BIN, "size_bytes": os.path.getsize(BIN), "startup_ms": round(startup * 1000, 1),
    "rss_kb_idle": status("VmRSS"), "rss_kb_after_warmup": rss_start, "rss_hwm_kb": status("VmHWM"), "threads": status("Threads"),
    "idle_cpu_ms_per_min": round((c1 - c0) * 1000 / tick / ((w1 - w0) / 60), 2), "idle_seconds": round(w1 - w0),
    "macro_latency_ms_median": round(statistics.median(lat), 3) if lat else None, "macro_latency_ms_p95": round(sorted(lat)[int(len(lat) * 0.95) - 1], 3) if lat else None, "samples": len(lat),
}
proc.terminate(); proc.wait(5)
print(json.dumps(out))
