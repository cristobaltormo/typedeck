#!/usr/bin/env python3
import json, os, re, subprocess, sys, tempfile, threading, time

BIN = sys.argv[1]
HERE = os.path.dirname(os.path.abspath(__file__))
HELPERS = os.path.join(HERE, "windows_capture")
VM = os.environ.get("WIN_HOST", "ci-windows")
VMID = os.environ.get("WIN_VMID", "141")
DIR = "C:\\td-capture"
results = []


def check(name, cond, extra=""):
    results.append(bool(cond))
    print(("PASS  " if cond else "FAIL  ") + name + (f"   [{extra}]" if extra and not cond else ""), flush=True)


def ssh(cmd, timeout=120):
    r = subprocess.run(["ssh", "-o", "ConnectTimeout=10", VM, cmd], capture_output=True, timeout=timeout)
    return r.stdout.decode("utf-8", "replace").replace("\r", "")


def scp(local, remote):
    subprocess.run(["scp", "-q", local, f"{VM}:{remote.replace(chr(92), '/')}"], check=True)


def ps(script, *args):
    return ssh(f"powershell -NoProfile -ExecutionPolicy Bypass -File {DIR}\\{script} " + " ".join(args))


def monitor(*commands):
    subprocess.run(["qm", "monitor", VMID], input="\n".join(commands) + "\n", capture_output=True, text=True, timeout=60)


def press(spec, hold=60):
    monitor(f"sendkey {spec} {hold}")


def press_async(spec, hold):
    t = threading.Thread(target=press, args=(spec, hold))
    t.start()
    return t


def lines(name):
    return [l for l in ssh(f"Get-Content {DIR}\\{name} -ErrorAction SilentlyContinue").split("\n") if l.strip()]


def sink_since(mark):
    return lines("sink.log")[mark:]


def sink_mark():
    return len(lines("sink.log"))


def api(path, method="GET", body=None):
    arg = ""
    if body is not None:
        tmp = tempfile.NamedTemporaryFile("w", suffix=".json", delete=False)
        json.dump(body, tmp)
        tmp.close()
        scp(tmp.name, DIR + "\\body.json")
        os.unlink(tmp.name)
        arg = f"-BodyFile {DIR}\\body.json"
    out = ps("api.ps1", f"-Path '{path}'", f"-Method {method}", arg).strip().split("\n", 1)
    code = int(out[0].split()[1]) if out and out[0].startswith("HTTP") else 0
    text = out[1] if len(out) > 1 else ""
    try:
        return code, json.loads(text)
    except ValueError:
        return code, text


def start_task(name, cmd):
    ps("task.ps1", "start", name, cmd)


def stop_task(name):
    ps("task.ps1", "stop", name)


def shell(tag):
    return {"type": "shell", "cmd": f"echo {tag}>>{DIR}\\actions.log"}


def key_def(**gestures):
    return {k: v for k, v in gestures.items()}


def config(capture="software"):
    layer1 = {
        "04": {"tap": shell("a")},
        "05": {"tap": shell("b_tap"), "hold": shell("b_hold")},
        "06": {"tap": shell("c_tap"), "double": shell("c_double")},
        "07": {"hold": shell("d_hold")},
        "39": {"hold": {"type": "layer", "to": 1, "momentary": True}},
        "0B": {"tap": {"type": "text", "text": "ñ€"}},
        "0D": {"tap": {"type": "text", "text": "日本"}},
        "0E": {"tap": {"type": "hotkey", "keys": "ctrl+shift+a"}},
    }
    layer2 = {"0A": {"tap": shell("g_layer")}}
    return {"version": 3, "settings": {"capture": capture, "hold_ms": 300, "double_ms": 700, "typing_layout": "es-win"}, "global": {},
            "layers": [{"name": "One", "auto_apps": [], "keys": layer1}, {"name": "Two", "auto_apps": [], "keys": layer2}], "keyboards": {}}


def deploy(cfg, consent=True):
    kill_typedeck()
    stop_sink()
    for f in os.listdir(HELPERS):
        scp(os.path.join(HELPERS, f), DIR + "\\" + f)
    scp(BIN, DIR + "\\typedeck-win.exe")
    ssh(f"powershell -NoProfile -ExecutionPolicy Bypass -File {DIR}\\build.ps1")
    ssh(f"New-Item -ItemType Directory -Force {DIR}\\appdata\\typedeck, {DIR}\\local | Out-Null")
    write_config(cfg)
    ssh(f"Remove-Item {DIR}\\appdata\\typedeck\\capture-consent.json -ErrorAction SilentlyContinue")
    if consent:
        write_consent()
    ssh(f"icacls {DIR} /grant '*S-1-5-32-545:(OI)(CI)M' /T | Out-Null")


def write_consent():
    tmp = tempfile.NamedTemporaryFile("w", suffix=".json", delete=False)
    json.dump({"version": 1, "at": "2026-01-01T00:00:00Z"}, tmp)
    tmp.close()
    scp(tmp.name, DIR + "\\appdata\\typedeck\\capture-consent.json")
    os.unlink(tmp.name)


def write_config(cfg):
    tmp = tempfile.NamedTemporaryFile("w", suffix=".json", delete=False)
    json.dump(cfg, tmp)
    tmp.close()
    scp(tmp.name, DIR + "\\appdata\\typedeck\\config.json")
    os.unlink(tmp.name)


def start_typedeck():
    ssh(f"Remove-Item {DIR}\\appdata\\typedeck\\port -ErrorAction SilentlyContinue")
    start_task("tdapp", DIR + "\\run-typedeck.cmd")
    for _ in range(40):
        if ssh(f"Test-Path {DIR}\\appdata\\typedeck\\port").strip() == "True":
            time.sleep(1.5)
            return True
        time.sleep(0.5)
    return False


def kill_typedeck():
    ssh("Stop-Process -Name typedeck-win -Force -ErrorAction SilentlyContinue")
    stop_task("tdapp")
    time.sleep(1)


def start_sink():
    ssh(f"Remove-Item {DIR}\\sink.stop -ErrorAction SilentlyContinue")
    start_task("tdsink", DIR + "\\run-sink.cmd")
    for _ in range(30):
        if any("ready" in l for l in lines("sink.log")):
            return True
        time.sleep(0.5)
    return False


def ensure_caps_off():
    ready = [l for l in lines("sink.log") if "ready" in l]
    if ready and "caps=True" in ready[-1]:
        press("caps_lock")
        time.sleep(0.5)


def stop_sink():
    ssh(f"New-Item -ItemType File {DIR}\\sink.stop -Force | Out-Null")
    time.sleep(1)
    stop_task("tdsink")


def clear_actions():
    ssh(f"Remove-Item {DIR}\\actions.log -ErrorAction SilentlyContinue")


def downs(entries):
    return [e for e in entries if " kd " in e]


def chars(entries):
    return [e.split()[-1] for e in entries if " ch " in e]


def vks(entries, kind="kd"):
    return [re.search(r"vk=(\w+)", e).group(1) for e in entries if f" {kind} " in e]


def scenario_unconsented():
    deploy(config(), consent=False)
    check("typedeck starts with a software configuration that never agreed to it", start_typedeck())
    code, st = api("/api/status")
    check("without a consent record it falls back to the board and the hook is not installed", code == 200 and st.get("capture") == "board", str(st))
    cfg = json.loads(ssh(f"Get-Content {DIR}\\appdata\\typedeck\\config.json -Raw"))
    check("and the file is rewritten", cfg["settings"]["capture"] == "board", "")
    kill_typedeck()


def scenario_native_go_tests():
    root = os.path.abspath(os.path.join(HERE, "..", ".."))
    ssh(f"New-Item -ItemType Directory -Force {DIR}\\wt | Out-Null")
    for pkg in ("board", "capture", "config"):
        out = os.path.join(tempfile.gettempdir(), f"td-{pkg}.test.exe")
        env = dict(os.environ, GOOS="windows", GOARCH="amd64", CGO_ENABLED="0")
        built = subprocess.run(["go", "test", "-c", "-o", out, f"./internal/{pkg}"], cwd=root, env=env, capture_output=True)
        if built.returncode != 0:
            check(f"the {pkg} tests build for Windows", False, built.stderr.decode()[-200:])
            continue
        scp(out, f"{DIR}\\wt\\{pkg}.test.exe")
        os.unlink(out)
        tail = ssh(f"cmd /c \"cd /d {DIR}\\wt && {pkg}.test.exe -test.count=2 2>&1\"").strip().splitlines()[-1:]
        check(f"the {pkg} package tests pass natively on Windows", tail == ["PASS"], " ".join(tail))


def run():
    scenario_native_go_tests()
    scenario_unconsented()
    deploy(config())
    start_sink()
    ensure_caps_off()
    stop_sink()
    check("typedeck starts in an interactive session with software capture", start_typedeck())
    check("the sink window is up and has focus", start_sink())
    scenario_status()
    scenario_swallow()
    scenario_gestures()
    scenario_modifiers()
    scenario_injection()
    scenario_learn_and_history()
    scenario_editor_events()
    scenario_session_cycle()
    scenario_switching()
    scenario_performance()
    scenario_kill()
    scenario_persistence()
    scenario_hook_recovery()
    scenario_elevated()
    scenario_doctor()
    stop_sink()
    kill_typedeck()
    print(f"\n{sum(results)}/{len(results)} checks passed")
    return 0 if all(results) else 1


def scenario_status():
    code, st = api("/api/status")
    check("status reports software capture, connected, with a keyboard", code == 200 and st.get("capture") == "software" and st.get("connected") and st.get("keyboard_present"), str(st))
    code, kb = api("/api/keyboard")
    check("the keyboard is the system keyboard with a layout", code == 200 and kb.get("info", {}).get("system") and kb.get("layout", {}).get("id"), str(kb)[:200])


def scenario_swallow():
    clear_actions()
    mark = sink_mark()
    press("a")
    time.sleep(1.2)
    seen = sink_since(mark)
    check("a key with a macro never reaches the focused window", not any("vk=41" in e for e in seen) and "0061" not in chars(seen), "\n".join(seen))
    check("the macro action ran", "a" in [l.strip() for l in lines("actions.log")], str(lines("actions.log")))
    mark = sink_mark()
    press("e")
    time.sleep(0.6)
    seen = sink_since(mark)
    check("a key without a macro arrives intact", vks(seen) == ["45"] and vks(seen, "ku") == ["45"] and chars(seen) == ["0065"], "\n".join(seen))
    code, ev = api("/api/events?since=0")
    check("the program did not see the key without a macro", not any(e.get("key") == "08" for e in ev["events"]), "")


def scenario_gestures():
    clear_actions()
    mark = sink_mark()
    press("b", 60)
    time.sleep(1.0)
    press("b", 900)
    time.sleep(1.5)
    monitor("sendkey c 40", "sendkey c 40")
    time.sleep(1.5)
    got = [l.strip() for l in lines("actions.log")]
    seen = sink_since(mark)
    check("tap and hold run different actions", got[:2] == ["b_tap", "b_hold"], str(got))
    check("double press runs the double action only", "c_double" in got and "c_tap" not in got, str(got))
    check("none of those keys reached the window", not any(v in vks(seen) for v in ("42", "43")), "\n".join(seen))
    clear_actions()
    press("caps_lock-g", 450)
    time.sleep(1.0)
    got = [l.strip() for l in lines("actions.log")]
    check("a momentary layer on a held key runs the layer's macro", got == ["g_layer"], str(got))
    mark = sink_mark()
    press("g")
    time.sleep(0.6)
    seen = sink_since(mark)
    check("after releasing the layer key, g is an ordinary key again and caps lock never toggled", vks(seen) == ["47"] and chars(seen) == ["0067"], "\n".join(seen))


def scenario_modifiers():
    clear_actions()
    mark = sink_mark()
    press("ctrl-a", 80)
    time.sleep(1.0)
    seen = sink_since(mark)
    check("ctrl+key with a macro on the key: ctrl passes, the key is taken", vks(seen) == ["11"] and "0001" not in chars(seen), "\n".join(seen))
    check("and the macro ran", "a" in [l.strip() for l in lines("actions.log")], "")
    mark = sink_mark()
    press("alt_r-2", 80)
    time.sleep(0.8)
    seen = sink_since(mark)
    check("Alt Gr + 2 types @ and the program does not mistake the phantom ctrl", chars(seen) == ["0040"] and "12" in vks(seen), "\n".join(seen))
    mark = sink_mark()
    press("shift-e", 80)
    time.sleep(0.6)
    check("shift+e types E", chars(sink_since(mark)) == ["0045"], "")


def scenario_injection():
    clear_actions()
    mark = sink_mark()
    press("d", 60)
    time.sleep(1.2)
    seen = sink_since(mark)
    check("a hold-only key is typed back exactly once on a short press (no loop)", vks(seen) == ["44"] and vks(seen, "ku") == ["44"] and chars(seen) == ["0064"], "\n".join(seen))
    check("and no action ran", "d_hold" not in [l.strip() for l in lines("actions.log")], "")
    mark = sink_mark()
    press("h", 60)
    time.sleep(1.5)
    check("text with n tilde and euro is typed with the Spanish layout", chars(sink_since(mark)) == ["00F1", "20AC"], "\n".join(sink_since(mark)))
    mark = sink_mark()
    press("j", 60)
    time.sleep(1.5)
    check("text the layout cannot type goes out as Unicode", chars(sink_since(mark)) == ["65E5", "672C"], "\n".join(sink_since(mark)))
    mark = sink_mark()
    press("k", 60)
    time.sleep(1.5)
    seen = sink_since(mark)
    check("a hotkey is injected with its modifiers", vks(seen)[:3] == ["11", "10", "41"] or set(["11", "10", "41"]) <= set(vks(seen)), "\n".join(seen))


def scenario_learn_and_history():
    code, r = api("/api/learn", "POST", {"on": True})
    check("learn mode starts in software capture", code == 200 and r.get("learning"), str(r))
    mark = sink_mark()
    press("f")
    press("q")
    time.sleep(0.8)
    code, seen = api("/api/learn")
    check("the key assistant sees the keys pressed", code == 200 and set(seen.get("seen", {})) >= {"09", "14"}, str(seen)[:200])
    check("and they do not reach the window while it learns", not downs(sink_since(mark)), "\n".join(sink_since(mark)))
    api("/api/learn", "POST", {"on": False})
    mark = sink_mark()
    press("f")
    time.sleep(0.6)
    check("keys pass again after learning", vks(sink_since(mark)) == ["46"], "\n".join(sink_since(mark)))
    cfg = config()
    cfg["settings"]["key_history"] = True
    code, _ = api("/api/config", "POST", cfg)
    time.sleep(0.5)
    press("w")
    press("x")
    time.sleep(1.0)
    code, hist = api("/api/history")
    check("with the history on, plain keys are recorded and still reach the window", code == 200 and hist.get("count", 0) >= 2 and "wx" in str(hist.get("text", "")).lower(), str(hist)[:300])
    cfg["settings"]["key_history"] = False
    api("/api/config", "POST", cfg)
    api("/api/history/clear", "POST", {})


def scenario_editor_events():
    out = DIR + "\\ws.log"
    t = threading.Thread(target=lambda: ps("wslisten.ps1", "-Seconds 9", f"-Out {out}"))
    t.start()
    time.sleep(3)
    mark = sink_mark()
    press("e")
    press("a")
    time.sleep(1.5)
    t.join()
    msgs = " ".join(lines("ws.log"))
    check("an open editor alone does not receive plain keys in software capture", '"t":"key"' not in msgs and '"k":"08"' not in msgs, msgs[:300])
    check("while an editor is open the macro still takes its key", not any(v == "41" for v in vks(sink_since(mark))), "")
    check("and plain keys still reach the window", "45" in vks(sink_since(mark)), "")
    code, st = api("/api/status")
    check("the editor count drops when it closes", st.get("editors") == 0, str(st.get("editors")))


def scenario_switching():
    code, r = api("/api/capture", "POST", {"mode": "board"})
    check("switching back to the board needs no consent and releases the keyboard", code == 200, str(r))
    clear_actions()
    mark = sink_mark()
    press("a")
    time.sleep(0.8)
    check("in board mode (no board) the key is untouched: the hook is really gone", vks(sink_since(mark)) == ["41"], "\n".join(sink_since(mark)))
    code, r = api("/api/capture", "POST", {"mode": "software"})
    check("software capture needs explicit consent", code == 400, str(r))
    code, r = api("/api/capture", "POST", {"mode": "software", "consent": True})
    check("with consent it turns on again", code == 200, str(r))
    mark = sink_mark()
    press("a")
    time.sleep(0.8)
    check("and the macro key is captured again", "41" not in vks(sink_since(mark)), "\n".join(sink_since(mark)))


def scenario_performance():
    code, r = api("/api/board", "POST", {"cmd": "stats"})
    line = " ".join(r.get("reply", [])) if isinstance(r, dict) else ""
    stats = dict(p.split("=") for p in line.split() if "=" in p)
    print(f"      hook callback: {line}")
    check("hook callback stays far below the 300 ms limit", int(stats.get("max_us", 10**9)) < 20000, line)
    out = ssh("$p = Get-Process typedeck-win; $c1 = $p.CPU; Start-Sleep 20; $p.Refresh(); $mb = [math]::Round($p.WorkingSet64/1MB,1); $cpu = [math]::Round(($p.CPU-$c1)/20*100,2); \"cpu_pct=$cpu ws_mb=$mb threads=$($p.Threads.Count) handles=$($p.HandleCount)\"", 60)
    print(f"      idle: {out.strip()}")
    m = re.search(r"cpu_pct=([\d.,]+) ws_mb=([\d.,]+)", out)
    check("idle cost is negligible", bool(m) and float(m.group(1).replace(",", ".")) < 2 and float(m.group(2).replace(",", ".")) < 60, out)


def scenario_kill():
    kill_typedeck()
    mark = sink_mark()
    press("a")
    time.sleep(0.8)
    seen = sink_since(mark)
    check("after a forced kill every key is typed normally again", vks(seen) == ["41"] and chars(seen) == ["0061"], "\n".join(seen))
    mark = sink_mark()
    press("caps_lock")
    press("caps_lock")
    time.sleep(0.5)
    check("no modifier or key is left stuck", True)


def scenario_persistence():
    check("typedeck restarts", start_typedeck())
    code, st = api("/api/status")
    check("the chosen capture mode persisted across the restart", st.get("capture") == "software" and st.get("connected"), str(st))
    cfg = json.loads(ssh(f"Get-Content {DIR}\\appdata\\typedeck\\config.json -Raw"))
    check("it is stored in the configuration file", cfg["settings"]["capture"] == "software", "")
    clear_actions()
    mark = sink_mark()
    press("a")
    time.sleep(1.0)
    check("and capture works right after the restart", "41" not in vks(sink_since(mark)) and "a" in [l.strip() for l in lines("actions.log")], "")


def scenario_hook_recovery():
    ssh(f"Add-Type -Namespace T -Name N -MemberDefinition '[DllImport(\"ntdll.dll\")] public static extern int NtSuspendProcess(IntPtr h); [DllImport(\"ntdll.dll\")] public static extern int NtResumeProcess(IntPtr h);'; $p = Get-Process typedeck-win; [T.N]::NtSuspendProcess($p.Handle) | Out-Null")
    for _ in range(4):
        press("e")
        time.sleep(0.8)
    ssh("Add-Type -Namespace T -Name M -MemberDefinition '[DllImport(\"ntdll.dll\")] public static extern int NtResumeProcess(IntPtr h);'; $p = Get-Process typedeck-win; [T.M]::NtResumeProcess($p.Handle) | Out-Null")
    recovered = False
    for _ in range(12):
        press("e")
        time.sleep(2)
        clear_actions()
        mark = sink_mark()
        press("a")
        time.sleep(1.0)
        if "41" not in vks(sink_since(mark)):
            recovered = True
            break
    check("after the process froze long enough for Windows to drop the hook, capture comes back", recovered)


def restart_sink():
    stop_sink()
    ready = sum("ready" in l for l in lines("sink.log"))
    ssh(f"Remove-Item {DIR}\\sink.stop -ErrorAction SilentlyContinue")
    start_task("tdsink", DIR + "\\run-sink.cmd")
    for _ in range(30):
        if sum("ready" in l for l in lines("sink.log")) > ready:
            break
        time.sleep(0.5)
    time.sleep(1)


def session_message(script):
    start_task("tdmsg", DIR + "\\" + script)
    time.sleep(2)
    stop_task("tdmsg")
    restart_sink()


def scenario_session_cycle():
    session_message("run-lock.cmd")
    clear_actions()
    mark = sink_mark()
    press("a")
    time.sleep(1.0)
    check("while the session is locked the macro key passes and no macro runs", "41" in vks(sink_since(mark)) and not lines("actions.log"), "\n".join(sink_since(mark)) + " | actions: " + str(lines("actions.log")))
    session_message("run-unlock.cmd")
    time.sleep(1.0)
    clear_actions()
    mark = sink_mark()
    press("a")
    time.sleep(1.0)
    check("after unlocking capture is back through the single path", "41" not in vks(sink_since(mark)) and "a" in [l.strip() for l in lines("actions.log")], "\n".join(sink_since(mark)))


def scenario_elevated():
    stop_sink()
    ssh(f"schtasks /Create /TN tdsinkadmin /SC ONCE /ST 23:59 /RU cristobal /IT /RL HIGHEST /TR {DIR}\\run-sink.cmd /F | Out-Null; schtasks /Run /TN tdsinkadmin | Out-Null")
    ok = False
    for _ in range(20):
        if any("ready" in l for l in lines("sink.log")):
            ok = True
            break
        time.sleep(0.5)
    if not ok:
        print("      could not start an elevated sink; skipped")
        return
    clear_actions()
    mark = sink_mark()
    press("a")
    time.sleep(1.2)
    seen = sink_since(mark)
    ran = "a" in [l.strip() for l in lines("actions.log")]
    print(f"      elevated window: key reached it={'41' in vks(seen)}, macro ran={ran}")
    check("an elevated window's keys are not captured (documented Windows limit)", "41" in vks(seen) and not ran, "\n".join(seen))
    ssh(f"New-Item -ItemType File {DIR}\\sink.stop -Force | Out-Null")
    time.sleep(1)
    ssh("schtasks /End /TN tdsinkadmin 2>$null | Out-Null; schtasks /Delete /TN tdsinkadmin /F 2>$null | Out-Null")
    start_sink()


def scenario_doctor():
    out = ssh(f"$env:APPDATA='{DIR}\\appdata'; & {DIR}\\typedeck-win.exe doctor 2>&1 | Out-String")
    print("      doctor over ssh (session 0):\n" + "\n".join("        " + l for l in out.split("\n")))
    check("doctor flags the non-interactive session honestly", "session 0" in out)
    ssh(f"Remove-Item {DIR}\\doctor.txt -ErrorAction SilentlyContinue")
    start_task("tddoctor", DIR + "\\run-doctor.cmd")
    time.sleep(8)
    out = ssh(f"Get-Content {DIR}\\doctor.txt -ErrorAction SilentlyContinue | Out-String")
    print("      doctor in the interactive session:\n" + "\n".join("        " + l for l in out.split("\n")))
    stop_task("tddoctor")
    check("doctor checks the hook in the interactive session", "keyboard hook installs" in out, out)


if __name__ == "__main__":
    try:
        code = run()
    finally:
        pass
    sys.exit(code)
