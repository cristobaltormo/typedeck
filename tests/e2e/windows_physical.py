#!/usr/bin/env python3
import json, os, subprocess, sys, tempfile, time

HOST = os.environ.get("WIN_HOST", "win")
DIR = "C:\\td-phys"
HERE = os.path.dirname(os.path.abspath(__file__))
HELPERS = os.path.join(HERE, "windows_physical")
KEY_ID = "3F"
WORD = "tdwin"
results = []


def check(name, cond, extra=""):
    results.append(bool(cond))
    print(("PASS  " if cond else "FAIL  ") + name + (f"   [{extra}]" if extra and not cond else ""), flush=True)


def ssh(cmd, timeout=90):
    r = subprocess.run(["ssh", "-o", "ConnectTimeout=10", HOST, cmd], capture_output=True, timeout=timeout)
    return r.stdout.decode("utf-8", "replace").replace("\r", "")


def scp(local, remote):
    subprocess.run(["scp", "-q", local, f"{HOST}:{remote.replace(chr(92), '/')}"], check=True)


def api(method, path, body=None):
    args = f"-Path {path} -Method {method}"
    if body is not None:
        with tempfile.NamedTemporaryFile("w", suffix=".json", delete=False) as f:
            json.dump(body, f)
        scp(f.name, DIR + "\\body.json")
        os.unlink(f.name)
        args += f" -BodyFile {DIR}\\body.json"
    out = ssh(f"powershell -NoProfile -ExecutionPolicy Bypass -File {DIR}\\api.ps1 {args}")
    try:
        return json.loads(out)
    except ValueError:
        return {"raw": out}


def interactive(name, cmd):
    ssh(f"schtasks /Create /TN {name} /SC ONCE /ST 23:59 /IT /F /TR {DIR}\\{cmd} & schtasks /Run /TN {name}")


def drop(name):
    ssh(f"schtasks /End /TN {name} & schtasks /Delete /TN {name} /F")


def sink_text():
    return ssh(f"type {DIR}\\testbox.txt 2>nul"), ssh(f"type {DIR}\\testbox.keys 2>nul")


def wait_for_text(word, seconds=40):
    deadline = time.time() + seconds
    while time.time() < deadline:
        text, keys = sink_text()
        if word in text:
            return text, keys
        time.sleep(1)
    return sink_text()


def press_prompt(message):
    print(f"PRESS: {message}", flush=True)
    gate = os.environ.get("GATE")
    if gate:
        while not os.path.exists(gate):
            time.sleep(1)
        os.unlink(gate)


def main():
    ssh(f"mkdir {DIR}")
    for name in ("api.ps1", "run-sink.cmd", "run-lock.cmd", "run-suspend.cmd"):
        scp(os.path.join(HELPERS, name), f"{DIR}\\{name}")
    scp(os.path.join(HERE, "windows_testbox.ps1"), f"{DIR}\\windows_testbox.ps1")

    original = api("GET", "/api/config")
    check("the program answers on the PC", "layers" in original)
    changed = json.loads(json.dumps(original))
    changed["layers"][0]["keys"][KEY_ID] = {"tap": {"type": "text", "text": WORD}}
    check("the test macro is saved", "config" in api("POST", "/api/config", changed))

    interactive("tdphys-sink", "run-sink.cmd")
    time.sleep(5)
    mode = "board" if os.environ.get("BOARD") else "software"
    if mode == "board":
        status = api("GET", "/api/status")
        check("the board is connected with its keyboard", status.get("connected") and status.get("keyboard_present") and status.get("capture") == "board", str(status)[:200])
    else:
        resp = api("POST", "/api/capture", {"mode": "software", "consent": True})
        check("software capture turns on", resp.get("capture") == "software", str(resp)[:160])

    press_prompt("F6 once (40 s), do not click anywhere")
    text, keys = wait_for_text(WORD)
    check("the macro on F6 typed its text into the window", WORD in text, repr(text))
    check("the F6 key itself did not reach the window", "F6" not in keys, keys.strip()[-80:])

    if os.environ.get("LOCK") != "0":
        interactive("tdphys-lock", "run-lock.cmd")
        before, _ = sink_text()
        press_prompt("the PC is locked: unlock it, then press F6 once (90 s)")
        deadline = time.time() + 90
        after = before
        while time.time() < deadline:
            after, _ = sink_text()
            if after.count(WORD) > before.count(WORD):
                break
            time.sleep(2)
        check("after locking and unlocking the macro works again", after.count(WORD) > before.count(WORD), repr(after))
        drop("tdphys-lock")

    if os.environ.get("SUSPEND") == "1":
        before, _ = sink_text()
        interactive("tdphys-suspend", "run-suspend.cmd")
        press_prompt("the PC suspends: wake it, then press F6 once (180 s)")
        text = before
        deadline = time.time() + 180
        while time.time() < deadline:
            text, _ = sink_text()
            if text.count(WORD) > before.count(WORD):
                break
            time.sleep(3)
        check("after suspending and waking the macro works again", text.count(WORD) > before.count(WORD), repr(text))
        drop("tdphys-suspend")

    if mode == "software":
        api("POST", "/api/capture", {"mode": "board"})
    api("POST", "/api/config", original)
    ssh("powershell -NoProfile -Command \"Get-CimInstance Win32_Process | ? { $_.CommandLine -match 'windows_testbox' } | % { Stop-Process -Id $_.ProcessId -Force }\"")
    drop("tdphys-sink")
    after = api("GET", "/api/config")
    check("the original configuration is restored", KEY_ID not in after["layers"][0]["keys"])
    ssh(f"rmdir /S /Q {DIR}")
    failed = results.count(False)
    print(f"{len(results) - failed}/{len(results)} checks passed")
    sys.exit(1 if failed else 0)


main()
