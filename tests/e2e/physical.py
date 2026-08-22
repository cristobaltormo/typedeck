#!/usr/bin/env python3
"""Test with PHYSICAL keys. Run on the Mac with Typedeck running; actions only show popups.
Usage: python3 physical.py [seconds]"""
import json, re, sys, time, urllib.request

BASE = "http://127.0.0.1:7788"
TOKEN = re.search(r'name="token" content="([^"]+)"', urllib.request.urlopen(BASE + "/").read().decode()).group(1)
SECS = int(sys.argv[1]) if len(sys.argv) > 1 else 45

def api(path, body=None):
    req = urllib.request.Request(BASE + path, data=None if body is None else json.dumps(body).encode(),
                                 method="POST" if body is not None else "GET", headers={"X-Token": TOKEN, "Content-Type": "application/json"})
    return json.loads(urllib.request.urlopen(req, timeout=20).read())

hud = lambda text: {"type": "hud", "text": text}
orig = api("/api/config")
cfg = json.loads(json.dumps(orig))
cfg["layers"][0]["keys"] = {
    "48": {"tap": hud("Pausa: pulsada"), "label": "Pausa"},
    "47": {"tap": hud("BloqDesp: corta"), "hold": hud("BloqDesp: LARGA")},
    "46": {"tap": hud("ImpPant: simple"), "double": hud("ImpPant: DOBLE")},
}
cfg["layers"][1]["keys"] = {"14": {"tap": hud("Capa 2: Q")}}
cfg["global"]["39"] = {"tap": hud("Bloq Mayus: corta"), "hold": {"type": "layer", "to": "next", "momentary": True}}
api("/api/config", cfg)
since = api("/api/status")["last_id"]
print(f"Configuracion de prueba puesta. Escuchando {SECS} s...", flush=True)
end = time.time() + SECS
events = []
while time.time() < end:
    r = api(f"/api/events?since={since}&wait=2")
    since = r["last_id"]
    for e in r["events"]:
        events.append(e)
        if e["kind"] in ("down", "exec", "layer"):
            print("  ", e["kind"], e.get("key", ""), e.get("gesture", ""), e.get("label", ""), e.get("name", ""), flush=True)
api("/api/config", orig)
print("Configuracion original restaurada.\n")

execs = [(e["key"], e["gesture"]) for e in events if e["kind"] == "exec"]
layers = [(e["layer"], e.get("auto", False)) for e in events if e["kind"] == "layer"]
checks = [
    ("Pausa (tecla capturada) ejecuta su accion", ("48", "tap") in execs),
    ("BloqDesp: pulsacion corta", ("47", "tap") in execs),
    ("BloqDesp: pulsacion larga", ("47", "hold") in execs),
    ("ImpPant: pulsacion simple", ("46", "tap") in execs),
    ("ImpPant: doble pulsacion", ("46", "double") in execs),
    ("Bloq Mayus corta ejecuta su accion", ("39", "tap") in execs),
    ("Bloq Mayus mantenida activa la capa 2 (momentanea)", (1, True) in layers),
    ("Al soltar Bloq Mayus se vuelve a la capa 1", (0, True) in layers),
    ("Q dentro de la capa momentanea ejecuta su accion", ("14", "tap") in execs),
]
for name, ok in checks:
    print(("PASS  " if ok else "FALLA ") + name)
print(f"\n{sum(ok for _, ok in checks)}/{len(checks)} comprobaciones fisicas correctas")
