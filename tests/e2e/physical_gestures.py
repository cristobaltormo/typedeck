"""Physical gesture test with a real key (ScrollLock by default, usage 47).
Binds harmless wait actions to tap, hold and double, listens for 45 s and restores the configuration.
Run on the Mac with the service running. Usage: python3 physical_gestures.py [usage_hex]"""
import json, re, sys, time, urllib.request
B = "http://127.0.0.1:7788"
KEY = (sys.argv[1] if len(sys.argv) > 1 else "47").upper()
T = re.search(r'name="token" content="([^"]+)"', urllib.request.urlopen(B + "/").read().decode()).group(1)
def api(p, b=None):
    r = urllib.request.Request(B + p, data=None if b is None else json.dumps(b).encode(), method="POST" if b is not None else "GET", headers={"X-Token": T, "Content-Type": "application/json"})
    return json.loads(urllib.request.urlopen(r, timeout=20).read())
orig = api("/api/config"); trial = json.loads(json.dumps(orig))
trial["layers"][0]["keys"][KEY] = {"tap": {"type": "wait", "ms": 5}, "hold": {"type": "wait", "ms": 6}, "double": {"type": "wait", "ms": 7}}
try:
    assert api("/api/config", trial).get("ok")
    time.sleep(0.8)
    since = api("/api/status")["last_id"]; t0 = time.time(); out = []
    print("READY: press the key now", flush=True)
    while time.time() - t0 < 45:
        r = api(f"/api/events?since={since}&wait=2"); since = r["last_id"]
        for e in r["events"]:
            if e["kind"] in ("down", "up", "exec") and e.get("key") == KEY:
                out.append((round(time.time() - t0, 2), e["kind"], e.get("gesture", "")))
finally:
    api("/api/config", orig)
print("config restaurada")
for o in out: print(o)
ex = [o[2] for o in out if o[1] == "exec"]
print("RESUMEN gestos ejecutados:", ex)
