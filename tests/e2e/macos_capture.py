#!/usr/bin/env python3
import json, os, subprocess, sys, time

HOST = os.environ.get("MAC_HOST", "mac")
HOME = subprocess.run(["ssh", HOST, "echo $HOME"], capture_output=True, text=True).stdout.strip()
SINK_BIN = os.environ.get("SINK_BIN", "/tmp/typedeck-sink")
SINK_OUT = "/tmp/typedeck-sink.txt"
LABEL = "typedeck.test.sink"
KEY_ID = "3F"
KEY_CODE = "97"
WORD = "tdmac"
results = []


def check(name, cond, extra=""):
    results.append(bool(cond))
    print(("PASS  " if cond else "FAIL  ") + name + (f"   [{extra}]" if extra and not cond else ""), flush=True)


def ssh(cmd):
    return subprocess.run(["ssh", HOST, cmd], capture_output=True, text=True, timeout=60).stdout


def api(method, path, body=None):
    data = f" -H 'Content-Type: application/json' -d '{json.dumps(body)}'" if body is not None else ""
    script = (
        "B=http://127.0.0.1:7788; T=$(curl -s $B/ | sed -n 's/.*name=\"token\" content=\"\\([^\"]*\\)\".*/\\1/p'); "
        f"curl -s -X {method} -H \"X-Token: $T\"{data} $B{path}"
    )
    out = ssh(script)
    try:
        return json.loads(out)
    except ValueError:
        return {"raw": out}


def build_sink():
    here = os.path.dirname(os.path.abspath(__file__))
    subprocess.run(["scp", "-q", os.path.join(here, "macos_capture", "sink.swift"), f"{HOST}:/tmp/typedeck-sink.swift"], check=True)
    ssh(f"swiftc -O /tmp/typedeck-sink.swift -o {SINK_BIN} && rm /tmp/typedeck-sink.swift")


def start_sink():
    plist = f"""<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict><key>Label</key><string>{LABEL}</string>
<key>ProgramArguments</key><array><string>{SINK_BIN}</string><string>{SINK_OUT}</string></array>
<key>RunAtLoad</key><true/></dict></plist>"""
    ssh(f"rm -f {SINK_OUT}; cat > /tmp/{LABEL}.plist <<'EOF'\n{plist}\nEOF\n"
        f"launchctl bootout gui/$(id -u)/{LABEL} 2>/dev/null; launchctl bootstrap gui/$(id -u) /tmp/{LABEL}.plist")


def stop_sink():
    ssh(f"launchctl bootout gui/$(id -u)/{LABEL} 2>/dev/null; rm -f /tmp/{LABEL}.plist {SINK_OUT}")


def sink():
    out = ssh(f"cat {SINK_OUT} 2>/dev/null")
    text = out.split("\nKEYS:")[0].removeprefix("TEXT:")
    keys = out.split("\nKEYS:")[1].split("\n")[0].split(",") if "\nKEYS:" in out else []
    front = "\nFRONT:1" in out
    return text, [k for k in keys if k], front


def main():
    original = api("GET", "/api/config")
    check("the program answers on the Mac", "layers" in original)
    status = api("GET", "/api/status")
    check("the board is connected before the test", status.get("capture") == "board", status.get("capture"))

    changed = json.loads(json.dumps(original))
    changed["layers"][0]["keys"][KEY_ID] = {"tap": {"type": "text", "text": WORD}}
    saved = api("POST", "/api/config", changed)
    check("the test macro is saved", "config" in saved, str(saved)[:120])

    build_sink()
    start_sink()
    time.sleep(2)
    text, keys, front = sink()
    check("the test window is in front", front)

    resp = api("POST", "/api/capture", {"mode": "software", "consent": True})
    check("software capture turns on with the permissions granted", resp.get("capture") == "software", str(resp)[:160])

    print("PRESS F6 on the keyboard now (40 s)", flush=True)
    deadline = time.time() + 40
    while time.time() < deadline:
        text, keys, front = sink()
        if WORD in text:
            break
        time.sleep(1)
    check("the macro on F6 typed its text into the window", WORD in text, repr(text))
    check("the F6 key itself did not reach the window", KEY_CODE not in keys, ",".join(keys))
    check("the window kept the focus", front)

    resp = api("POST", "/api/capture", {"mode": "board"})
    check("back to the board", resp.get("capture") == "board", str(resp)[:160])
    api("POST", "/api/config", original)
    stop_sink()
    ssh(f"rm -f {SINK_BIN}")
    after = api("GET", "/api/config")
    check("the original configuration is restored", KEY_ID not in after["layers"][0]["keys"])

    failed = results.count(False)
    print(f"{len(results) - failed}/{len(results)} checks passed")
    sys.exit(1 if failed else 0)


main()
