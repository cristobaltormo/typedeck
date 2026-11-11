#!/usr/bin/env python3
import fcntl, json, os, pwd, re, select, shutil, signal, statistics, struct, subprocess, sys, tempfile, threading, time, urllib.error, urllib.request

BIN = sys.argv[1] if len(sys.argv) > 1 else os.environ["TYPEDECK_BIN"]
ROUNDS = int(os.environ.get("LINUX_CAPTURE_ROUNDS", "300"))
UNPRIV = os.environ.get("LINUX_CAPTURE_UNPRIV", "nobody")
DISPLAY = os.environ.get("LINUX_CAPTURE_DISPLAY", "")
WIDEN_UINPUT = os.environ.get("LINUX_CAPTURE_WIDEN_UINPUT") == "1"
OUT_NAME = "Typedeck virtual keyboard"

EV_SYN, EV_KEY, EV_REL, EV_LED, EV_REP = 0, 1, 2, 0x11, 0x14
UI_SET_EVBIT, UI_SET_KEYBIT, UI_SET_RELBIT, UI_SET_LEDBIT = 0x40045564, 0x40045565, 0x40045566, 0x40045569
UI_DEV_SETUP, UI_DEV_CREATE, UI_DEV_DESTROY = 0x405C5503, 0x5501, 0x5502
EVENT = struct.Struct("@llHHi")
KEY = dict(esc=1, a=30, b=48, c=46, d=32, e=18, f=33, g=34, h=35, j=36, k=37, q=16, r=19, t=20, w=17, x=45, z=44, ctrl=29, shift=42,
           alt=56, ralt=100, caps=58, k2=3, volup=115, mute=113, v=47)
results = []


def check(name, cond, extra=""):
    results.append(bool(cond))
    print(("PASS  " if cond else "FAIL  ") + name + (f"   [{extra}]" if extra and not cond else ""), flush=True)


def eviocgname():
    return 0x80000000 | 256 << 16 | ord("E") << 8 | 0x06


class VirtualDevice:
    def __init__(self, name, keys, rel=False, leds=False, repeat=True):
        self.fd = os.open("/dev/uinput", os.O_RDWR | os.O_NONBLOCK)
        fcntl.ioctl(self.fd, UI_SET_EVBIT, EV_KEY)
        if repeat:
            fcntl.ioctl(self.fd, UI_SET_EVBIT, EV_REP)
        for k in keys:
            fcntl.ioctl(self.fd, UI_SET_KEYBIT, k)
        if rel:
            fcntl.ioctl(self.fd, UI_SET_EVBIT, EV_REL)
            fcntl.ioctl(self.fd, UI_SET_RELBIT, 0)
            fcntl.ioctl(self.fd, UI_SET_RELBIT, 1)
        if leds:
            fcntl.ioctl(self.fd, UI_SET_EVBIT, EV_LED)
            fcntl.ioctl(self.fd, UI_SET_LEDBIT, 1)
        fcntl.ioctl(self.fd, UI_DEV_SETUP, struct.pack("HHHH80sI", 3, 0x1234, 0x5678, 1, name.encode(), 0))
        fcntl.ioctl(self.fd, UI_DEV_CREATE)
        self.name = name
        self.path = wait_node(name)

    def emit(self, typ, code, value):
        os.write(self.fd, EVENT.pack(0, 0, typ, code, value) + EVENT.pack(0, 0, EV_SYN, 0, 0))

    def down(self, code): self.emit(EV_KEY, code, 1)
    def up(self, code): self.emit(EV_KEY, code, 0)

    def tap(self, code, hold=0.03):
        self.down(code)
        time.sleep(hold)
        self.up(code)

    def led_events(self):
        got = []
        while select.select([self.fd], [], [], 0.3)[0]:
            try:
                data = os.read(self.fd, EVENT.size * 16)
            except BlockingIOError:
                continue
            for i in range(0, len(data) - EVENT.size + 1, EVENT.size):
                _, _, typ, code, val = EVENT.unpack_from(data, i)
                if typ == EV_LED:
                    got.append((code, val))
        return got

    def destroy(self):
        fcntl.ioctl(self.fd, UI_DEV_DESTROY)
        os.close(self.fd)


def node_names():
    out = {}
    for n in os.listdir("/dev/input"):
        if n.startswith("event"):
            try:
                fd = os.open("/dev/input/" + n, os.O_RDONLY | os.O_NONBLOCK)
            except OSError:
                continue
            buf = bytearray(256)
            try:
                fcntl.ioctl(fd, eviocgname(), buf)
                out["/dev/input/" + n] = bytes(buf).split(b"\0")[0].decode(errors="replace")
            except OSError:
                pass
            os.close(fd)
    return out


def wait_node(name, secs=8, gone=False):
    end = time.time() + secs
    while time.time() < end:
        paths = [p for p, n in node_names().items() if n == name]
        if bool(paths) != gone:
            return paths[0] if paths else None
        time.sleep(0.05)
    return None


class Reader(threading.Thread):
    def __init__(self, path):
        super().__init__(daemon=True)
        self.path = path
        self.fd = os.open(path, os.O_RDONLY | os.O_NONBLOCK)
        self.events, self.lock, self.running = [], threading.Lock(), True
        self.start()

    def run(self):
        while self.running:
            if not select.select([self.fd], [], [], 0.1)[0]:
                continue
            try:
                data = os.read(self.fd, EVENT.size * 64)
            except OSError:
                return
            now = time.monotonic_ns()
            with self.lock:
                for i in range(0, len(data) - EVENT.size + 1, EVENT.size):
                    _, _, typ, code, val = EVENT.unpack_from(data, i)
                    if typ == EV_KEY:
                        self.events.append((now, code, val))

    def mark(self):
        with self.lock:
            return len(self.events)

    def since(self, mark, repeats=False):
        with self.lock:
            return [(c, v) for _, c, v in self.events[mark:] if repeats or v != 2]

    def close(self):
        self.running = False
        self.join(1)
        os.close(self.fd)


class Typedeck:
    def __init__(self, home, user=None):
        self.home, self.user, self.proc = home, user, None
        self.log = open(os.path.join(home, "typedeck.log"), "ab")

    def start(self):
        env = {**os.environ, "HOME": self.home, "XDG_CONFIG_HOME": os.path.join(self.home, "cfg")}
        for k in ("DISPLAY", "WAYLAND_DISPLAY"):
            env.pop(k, None)
        if DISPLAY:
            env["DISPLAY"] = DISPLAY
        cmd = [BIN]
        if self.user:
            pw = pwd.getpwnam(self.user)
            cmd = ["setpriv", f"--reuid={pw.pw_uid}", f"--regid={pw.pw_gid}", "--clear-groups"] + cmd
        port_file = os.path.join(self.home, "cfg", "typedeck", "port")
        if os.path.exists(port_file):
            os.unlink(port_file)
        self.proc = subprocess.Popen(cmd, env=env, stdout=self.log, stderr=subprocess.STDOUT)
        for _ in range(100):
            try:
                self.base = f"http://127.0.0.1:{int(open(port_file).read())}"
                break
            except (OSError, ValueError):
                time.sleep(0.1)
        else:
            sys.exit("typedeck did not start")
        time.sleep(0.4)
        page = urllib.request.urlopen(self.base + "/").read().decode()
        self.token = re.search(r'name="token" content="([^"]+)"', page).group(1)

    def api(self, path, body=None):
        req = urllib.request.Request(self.base + path, data=None if body is None else json.dumps(body).encode(),
                                     method="GET" if body is None else "POST", headers={"X-Token": self.token, "Content-Type": "application/json"})
        try:
            r = urllib.request.urlopen(req, timeout=20)
            return r.status, json.loads(r.read() or b"{}")
        except urllib.error.HTTPError as e:
            raw = e.read().decode(errors="replace")
            try:
                return e.code, json.loads(raw)
            except ValueError:
                return e.code, {"error": raw}

    def kill(self, sig=signal.SIGKILL):
        self.proc.send_signal(sig)
        self.proc.wait()

    def cpu_ticks(self):
        f = open(f"/proc/{self.proc.pid}/stat").read().rsplit(")", 1)[1].split()
        return int(f[11]) + int(f[12])


class Pump(threading.Thread):
    def __init__(self, kb):
        super().__init__(daemon=True)
        self.kb, self.running = kb, True
        self.start()

    def run(self):
        while self.running:
            self.kb.down(KEY["e"])
            self.kb.up(KEY["e"])
            time.sleep(0.001)

    def stop(self):
        self.running = False
        self.join(1)


def shell(out, tag):
    return {"type": "shell", "cmd": f"echo {tag} >> {out}"}


def config(out, capture="software"):
    keys = {
        "04": {"tap": shell(out, "a")},
        "05": {"tap": shell(out, "b_tap"), "hold": shell(out, "b_hold")},
        "06": {"tap": shell(out, "c_tap"), "double": shell(out, "c_double")},
        "07": {"hold": shell(out, "d_hold")},
        "39": {"hold": {"type": "layer", "to": 1, "momentary": True}},
        "0B": {"tap": {"type": "text", "text": "Hi"}},
        "0D": {"tap": {"type": "text", "text": "日本"}},
        "0E": {"tap": {"type": "hotkey", "keys": "ctrl+shift+a"}},
        "0F": {"tap": {"type": "media", "cmd": "volup"}},
    }
    layer2 = {"0A": {"tap": shell(out, "g_layer")}}
    return {"version": 3, "settings": {"capture": capture, "hold_ms": 300, "double_ms": 700, "typing_layout": "us"}, "global": {},
            "layers": [{"name": "One", "auto_apps": [], "keys": keys}, {"name": "Two", "auto_apps": [], "keys": layer2}], "keyboards": {}}


def write_config(home, cfg):
    d = os.path.join(home, "cfg", "typedeck")
    os.makedirs(d, exist_ok=True)
    json.dump(cfg, open(os.path.join(d, "config.json"), "w"))


def lines(path):
    try:
        return [l.strip() for l in open(path) if l.strip()]
    except OSError:
        return []


def downs(seq):
    return [c for c, v in seq if v == 1]


def main():
    work = tempfile.mkdtemp(prefix="typedeck-capture-")
    os.chmod(work, 0o755)
    home = os.path.join(work, "home")
    os.makedirs(home)
    actions = os.path.join(work, "actions.log")
    write_config(home, config(actions, "board"))
    keyset = list(range(1, 128)) + [183, 184]
    kb = VirtualDevice("Typedeck e2e keyboard", keyset, leds=True)
    plain = Reader(kb.path)
    td = Typedeck(home)
    out = None
    try:
        baseline = measure(kb, plain, 150)
        td.start()
        scenario_status_and_consent(td)
        out = Reader(wait_node(OUT_NAME) or sys.exit("the virtual keyboard did not appear"))
        scenario_swallow(kb, out, td, actions)
        scenario_repeat(kb, out, actions)
        scenario_gestures(kb, out, actions)
        scenario_modifiers(kb, out, actions)
        scenario_injection(kb, out, actions)
        scenario_leds(kb, out)
        scenario_other_devices(out, plain)
        scenario_hotplug(kb, out, actions)
        out = scenario_switching(td, kb, plain, out, actions)
        scenario_performance(td, kb, out, baseline)
        scenario_signals(td, kb, out, plain)
        scenario_permissions(work)
        scenario_doctor(home)
    finally:
        for r in (out, plain):
            if r:
                r.close()
        if td.proc and td.proc.poll() is None:
            td.kill()
        kb.destroy()
        shutil.rmtree(work, ignore_errors=True)
    print(f"\n{sum(results)}/{len(results)} checks passed")
    return 0 if all(results) else 1


def scenario_status_and_consent(td):
    code, r = td.api("/api/capture", {"mode": "software"})
    check("software capture needs explicit consent", code == 400, str(r))
    code, r = td.api("/api/capture", {"mode": "software", "consent": True})
    check("with consent it turns on", code == 200, str(r))
    code, st = td.api("/api/status")
    check("status reports software capture, connected, with a keyboard", st.get("capture") == "software" and st.get("connected") and st.get("keyboard_present"), str(st))
    time.sleep(0.5)


def scenario_swallow(kb, out, td, actions):
    mark = out.mark()
    kb.tap(KEY["a"])
    time.sleep(0.5)
    check("a key with a macro never reaches the receiver", out.since(mark) == [], str(out.since(mark)))
    check("and its action ran", lines(actions) == ["a"], str(lines(actions)))
    mark = out.mark()
    kb.tap(KEY["e"])
    time.sleep(0.2)
    check("a key without a macro arrives intact", out.since(mark) == [(KEY["e"], 1), (KEY["e"], 0)], str(out.since(mark)))
    code, ev = td.api("/api/events?since=0")
    check("the program did not see the key without a macro", not any(e.get("key") == "08" for e in ev["events"]))
    mark = out.mark()
    for k in "wqaxrta":
        kb.tap(KEY[k], 0.01)
    time.sleep(0.4)
    seq = out.since(mark)
    check("the other keys pass in order with the macro keys removed", downs(seq) == [KEY[k] for k in "wqxrt"] and len(seq) == 10, str(seq))


def scenario_repeat(kb, out, actions):
    mark = out.mark()
    kb.down(KEY["e"])
    time.sleep(0.9)
    kb.up(KEY["e"])
    time.sleep(0.2)
    seq = out.since(mark, repeats=True)
    reps = [v for _, v in seq].count(2)
    check("autorepeat is forwarded between the press and the release", seq[0] == (KEY["e"], 1) and seq[-1] == (KEY["e"], 0) and reps >= 5, f"{reps} repeats, {seq[:3]}")
    open(actions, "w").close()
    mark = out.mark()
    kb.down(KEY["a"])
    time.sleep(0.9)
    kb.up(KEY["a"])
    time.sleep(0.5)
    check("holding a macro key swallows its repeats and runs the macro once", out.since(mark, repeats=True) == [] and lines(actions) == ["a"], f"{out.since(mark, True)} {lines(actions)}")


def scenario_gestures(kb, out, actions):
    open(actions, "w").close()
    mark = out.mark()
    kb.tap(KEY["b"], 0.05)
    time.sleep(0.9)
    kb.tap(KEY["b"], 0.7)
    time.sleep(0.8)
    kb.tap(KEY["c"], 0.03)
    time.sleep(0.1)
    kb.tap(KEY["c"], 0.03)
    time.sleep(1.2)
    got = lines(actions)
    check("tap and hold run different actions", got[:2] == ["b_tap", "b_hold"], str(got))
    check("double press runs the double action only", "c_double" in got and "c_tap" not in got, str(got))
    check("none of those keys reached the receiver", out.since(mark) == [], str(out.since(mark)))
    open(actions, "w").close()
    mark = out.mark()
    kb.tap(KEY["d"], 0.05)
    time.sleep(0.8)
    check("a hold-only key is typed back exactly once on a short press", out.since(mark) == [(KEY["d"], 1), (KEY["d"], 0)] and lines(actions) == [], str(out.since(mark)))
    kb.down(KEY["caps"])
    time.sleep(0.6)
    kb.tap(KEY["g"], 0.05)
    time.sleep(0.2)
    kb.up(KEY["caps"])
    time.sleep(0.5)
    check("a momentary layer on a held key runs the layer's macro", lines(actions) == ["g_layer"], str(lines(actions)))
    mark = out.mark()
    kb.tap(KEY["g"], 0.03)
    time.sleep(0.3)
    check("afterwards g is an ordinary key again and caps lock never reached the receiver", out.since(mark) == [(KEY["g"], 1), (KEY["g"], 0)], str(out.since(mark)))


def scenario_modifiers(kb, out, actions):
    open(actions, "w").close()
    mark = out.mark()
    kb.down(KEY["ctrl"])
    kb.tap(KEY["a"], 0.03)
    kb.up(KEY["ctrl"])
    time.sleep(0.5)
    check("ctrl+key with a macro: ctrl passes, the key is taken", out.since(mark) == [(KEY["ctrl"], 1), (KEY["ctrl"], 0)] and lines(actions) == ["a"], str(out.since(mark)))
    mark = out.mark()
    kb.down(KEY["shift"])
    kb.tap(KEY["e"], 0.03)
    kb.up(KEY["shift"])
    kb.down(KEY["ralt"])
    kb.tap(KEY["k2"], 0.03)
    kb.up(KEY["ralt"])
    time.sleep(0.3)
    want = [(KEY["shift"], 1), (KEY["e"], 1), (KEY["e"], 0), (KEY["shift"], 0), (KEY["ralt"], 1), (KEY["k2"], 1), (KEY["k2"], 0), (KEY["ralt"], 0)]
    check("shift+e and Alt Gr+2 pass in the same order", out.since(mark) == want, str(out.since(mark)))


def scenario_injection(kb, out, actions):
    mark = out.mark()
    kb.tap(KEY["h"], 0.03)
    time.sleep(0.8)
    seq = out.since(mark)
    check("typed text goes out through the virtual keyboard (H needs shift)", downs(seq) == [42, 35, 23], str(seq))
    mark = out.mark()
    kb.tap(KEY["k"], 0.03)
    time.sleep(0.8)
    seq = out.since(mark)
    check("a hotkey is injected with its modifiers in order", downs(seq) == [29, 42, 30] and [c for c, v in seq if v == 0] == [30, 42, 29], str(seq))
    mark = out.mark()
    kb.tap(38, 0.03)
    time.sleep(0.6)
    check("a media action sends the media key", (KEY["volup"], 1) in out.since(mark), str(out.since(mark)))
    if DISPLAY and shutil.which("xclip"):
        mark = out.mark()
        kb.tap(36, 0.03)
        time.sleep(1.5)
        check("text the layout cannot type is pasted with ctrl+v through the virtual keyboard", downs(out.since(mark)) == [29, 47], str(out.since(mark)))
    else:
        print("      skipped: clipboard paste (set LINUX_CAPTURE_DISPLAY to an X display and install xclip)")


def scenario_leds(kb, out):
    kb.led_events()
    fd = os.open(out.path, os.O_WRONLY)
    os.write(fd, EVENT.pack(0, 0, EV_LED, 1, 1) + EVENT.pack(0, 0, EV_SYN, 0, 0))
    os.close(fd)
    got = kb.led_events()
    check("a keyboard LED set on the virtual keyboard reaches the real keyboard", (1, 1) in got, str(got))


def scenario_other_devices(out, plain):
    mouse = VirtualDevice("Typedeck e2e mouse", [0x110], rel=True)
    media = VirtualDevice("Typedeck e2e media keys", [KEY["volup"], KEY["mute"]], repeat=False)
    mouse_reader, media_reader = Reader(mouse.path), Reader(media.path)
    time.sleep(1.0)
    mark = out.mark()
    media.tap(KEY["volup"])
    mouse.tap(0x110)
    time.sleep(0.3)
    check("a media-only device and a mouse are left alone (not grabbed)", media_reader.since(0) == [(KEY["volup"], 1), (KEY["volup"], 0)] and mouse_reader.since(0) == [(0x110, 1), (0x110, 0)], f"{media_reader.since(0)} {mouse_reader.since(0)}")
    check("and nothing from them goes through the virtual keyboard", out.since(mark) == [])
    for r in (mouse_reader, media_reader):
        r.close()
    mouse.destroy()
    media.destroy()


def scenario_hotplug(kb, out, actions):
    open(actions, "w").close()
    kb2 = VirtualDevice("Typedeck e2e keyboard 2", list(range(1, 128)))
    time.sleep(1.2)
    mark = out.mark()
    kb2.tap(KEY["a"])
    kb2.tap(KEY["e"])
    time.sleep(0.4)
    check("a keyboard plugged in while running is captured", out.since(mark) == [(KEY["e"], 1), (KEY["e"], 0)] and lines(actions) == ["a"], f"{out.since(mark)} {lines(actions)}")
    mark = out.mark()
    kb2.down(KEY["e"])
    time.sleep(0.2)
    kb2.destroy()
    wait_node("Typedeck e2e keyboard 2", gone=True)
    time.sleep(0.4)
    check("unplugging with a key held releases it on the virtual keyboard", out.since(mark) == [(KEY["e"], 1), (KEY["e"], 0)], str(out.since(mark)))
    mark = out.mark()
    kb.tap(KEY["e"])
    time.sleep(0.2)
    check("the first keyboard keeps working after the second left", len(out.since(mark)) == 2)


def scenario_switching(td, kb, plain, out, actions):
    open(actions, "w").close()
    code, r = td.api("/api/capture", {"mode": "board"})
    check("switching back to the board needs no consent", code == 200, str(r))
    time.sleep(0.5)
    mark = plain.mark()
    kb.tap(KEY["a"])
    time.sleep(0.3)
    check("in board mode the grab is gone: the key reaches ordinary readers", plain.since(mark) == [(KEY["a"], 1), (KEY["a"], 0)] and lines(actions) == [], str(plain.since(mark)))
    check("the virtual keyboard is removed", wait_node(OUT_NAME, 3, gone=True) is None)
    code, r = td.api("/api/capture", {"mode": "software", "consent": True})
    check("it turns on again", code == 200, str(r))
    time.sleep(0.6)
    new_out = wait_node(OUT_NAME)
    out.close()
    out = Reader(new_out)
    mark = out.mark()
    kb.tap(KEY["a"])
    time.sleep(0.3)
    check("and the macro key is taken again", out.since(mark) == [] and lines(actions) == ["a"], str(out.since(mark)))
    return out


def measure(kb, reader, rounds):
    lat = []
    for _ in range(rounds):
        mark = reader.mark()
        t0 = time.monotonic_ns()
        kb.down(KEY["e"])
        for _ in range(500):
            with reader.lock:
                if len(reader.events) > mark:
                    t1 = reader.events[mark][0]
                    break
            time.sleep(0.0002)
        else:
            t1 = t0
        lat.append((t1 - t0) / 1000)
        kb.up(KEY["e"])
        time.sleep(0.015)
    lat.sort()
    return {"median": statistics.median(lat), "p99": lat[int(len(lat) * 0.99) - 1], "max": lat[-1]}


def scenario_performance(td, kb, out, baseline):
    r = measure(kb, out, ROUNDS)
    print(f"      latency, press to receiver (us): direct median={baseline['median']:.0f} p99={baseline['p99']:.0f} max={baseline['max']:.0f}; through typedeck median={r['median']:.0f} p99={r['p99']:.0f} max={r['max']:.0f}")
    check("passing a key through typedeck adds well under 1 ms at the median", r["median"] - baseline["median"] < 1000, str(r))
    code, st = td.api("/api/board", {"cmd": "stats"})
    print(f"      in-process: {' '.join(st.get('reply', [])) if isinstance(st, dict) else st}")
    t0, c0 = time.time(), td.cpu_ticks()
    time.sleep(20)
    ticks = td.cpu_ticks() - c0
    pct = ticks / os.sysconf("SC_CLK_TCK") / (time.time() - t0) * 100
    status = open(f"/proc/{td.proc.pid}/status").read()
    rss = int(re.search(r"VmRSS:\s+(\d+)", status).group(1)) // 1024
    threads = re.search(r"Threads:\s+(\d+)", status).group(1)
    print(f"      idle with capture on: cpu={pct:.2f}% rss={rss} MB threads={threads}")
    check("idle cost is negligible", pct < 1.0 and rss < 80, f"{pct:.2f}% {rss} MB")


def scenario_signals(td, kb, out, plain):
    td.proc.send_signal(signal.SIGTSTP)
    time.sleep(0.3)
    mark = out.mark()
    kb.tap(KEY["e"])
    time.sleep(0.3)
    check("Ctrl+Z in a terminal (SIGTSTP) does not freeze the keyboard", out.since(mark) == [(KEY["e"], 1), (KEY["e"], 0)], str(out.since(mark)))
    td.proc.send_signal(signal.SIGSTOP)
    time.sleep(0.3)
    mark = plain.mark()
    kb.tap(KEY["e"])
    time.sleep(0.3)
    print(f"      frozen (SIGSTOP) process: key reached ordinary readers={plain.since(mark) != []} (a frozen process holds the keyboard until it resumes or dies)")
    td.proc.send_signal(signal.SIGCONT)
    time.sleep(0.3)
    kb.down(KEY["e"])
    time.sleep(0.2)
    kb.up(KEY["e"])
    pump = Pump(kb)
    time.sleep(0.2)
    mark = plain.mark()
    t0 = time.monotonic_ns()
    td.kill(signal.SIGKILL)
    t_dead = time.monotonic_ns()
    time.sleep(0.2)
    pump.stop()
    first = next((t for t, c, v in plain.events[mark:] if v == 1), None)
    check("after kill -9 the keys reach ordinary readers, macro key included", first is not None)
    if first:
        print(f"      kill -9: first key reached a plain reader {(first - t0) / 1e6:.1f} ms after the signal (the process was gone after {(t_dead - t0) / 1e6:.0f} ms)")
    time.sleep(0.2)
    mark = plain.mark()
    kb.tap(KEY["a"], 0.02)
    kb.tap(KEY["e"], 0.02)
    time.sleep(0.2)
    seq = plain.since(mark)
    check("and the macro key is an ordinary key again", seq == [(KEY["a"], 1), (KEY["a"], 0), (KEY["e"], 1), (KEY["e"], 0)], str(seq))
    check("and the virtual keyboard is gone", wait_node(OUT_NAME, 3, gone=True) is None)
    td.start()
    td.api("/api/capture", {"mode": "software", "consent": True})
    time.sleep(0.8)
    td.kill(signal.SIGTERM)
    mark = plain.mark()
    kb.tap(KEY["a"])
    time.sleep(0.3)
    check("after SIGTERM the keyboard is released as well", plain.since(mark) == [(KEY["a"], 1), (KEY["a"], 0)], str(plain.since(mark)))
    td.start()
    code, st = td.api("/api/status")
    check("the capture mode persisted across the restart", st.get("capture") == "software" and st.get("connected"), str(st))
    time.sleep(0.5)


def scenario_permissions(work):
    pu = os.path.join(work, "unpriv")
    os.makedirs(os.path.join(pu, "cfg", "typedeck"))
    try:
        uid = pwd.getpwnam(UNPRIV).pw_uid
    except KeyError:
        print("      skipped: no unprivileged user to test the permission errors")
        return
    subprocess.run(["chown", "-R", str(uid), pu])
    json.dump(config(os.path.join(pu, "a.log"), "board"), open(os.path.join(pu, "cfg", "typedeck", "config.json"), "w"))
    os.chmod(pu, 0o755)
    t = Typedeck(pu, user=UNPRIV)
    t.start()
    before = open("/proc/bus/input/devices").read().count(OUT_NAME)
    code, r = t.api("/api/capture", {"mode": "software", "consent": True})
    msg = json.dumps(r)
    print(f"      without access to /dev/uinput: HTTP {code} {msg[:230]}")
    check("without permission the error names the problem and the fix", code != 200 and ("uinput" in msg or "/dev/input" in msg) and "input group" in msg, msg)
    time.sleep(1)
    check("a failed attempt leaves no virtual keyboard behind", open("/proc/bus/input/devices").read().count(OUT_NAME) == before)
    code, st = t.api("/api/status")
    check("the program stays in board mode and keeps running", st.get("capture") == "board", str(st))
    t.kill()
    if WIDEN_UINPUT:
        mode = os.stat("/dev/uinput").st_mode & 0o777
        os.chmod("/dev/uinput", 0o666)
        try:
            t.start()
            code, r = t.api("/api/capture", {"mode": "software", "consent": True})
            msg = json.dumps(r)
            print(f"      with /dev/uinput but without /dev/input: HTTP {code} {msg[:230]}")
            check("with uinput but no read access to the keyboards the error says so", code != 200 and "/dev/input" in msg, msg)
            t.kill()
        finally:
            os.chmod("/dev/uinput", mode)


def scenario_doctor(home):
    env = {**os.environ, "HOME": home, "XDG_CONFIG_HOME": os.path.join(home, "cfg")}
    r = subprocess.run([BIN, "doctor"], env=env, capture_output=True, text=True, timeout=30)
    print("      doctor:\n" + "\n".join("        " + l for l in (r.stdout + r.stderr).splitlines()))
    check("doctor reports the uinput and keyboard checks", "uinput" in r.stdout and "keyboard" in r.stdout, r.stdout)


if __name__ == "__main__":
    sys.exit(main())
