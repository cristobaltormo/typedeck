import json, os, subprocess, time

HOST = os.environ.get("MAC_HOST", "mac")
SINK_BIN = os.environ.get("SINK_BIN", "/tmp/typedeck-sink")
SINK_OUT = "/tmp/typedeck-sink.txt"
LABEL = "typedeck.test.sink"
HERE = os.path.dirname(os.path.abspath(__file__))

TOKEN = (
    "B=http://127.0.0.1:7788; T=$(curl -s $B/ | sed -n 's/.*name=\"token\" content=\"\\([^\"]*\\)\".*/\\1/p'); "
)

BOARD_KEY = r"""
import glob, os, sys, time
ports = glob.glob("/dev/cu.usbmodem*")
if not ports:
    sys.exit("no board port")
fd = os.open(ports[0], os.O_WRONLY | os.O_NOCTTY | os.O_NONBLOCK)
os.write(fd, ("\nKEY %s %s %d\n" % (sys.argv[1], sys.argv[2], int(sys.argv[3]))).encode())
time.sleep(0.2)
os.close(fd)
"""


class Checks:
    def __init__(self):
        self.results = []

    def __call__(self, name, cond, extra=""):
        self.results.append(bool(cond))
        print(("PASS  " if cond else "FAIL  ") + name + (f"   [{extra}]" if extra and not cond else ""), flush=True)

    def finish(self):
        failed = self.results.count(False)
        print(f"{len(self.results) - failed}/{len(self.results)} checks passed")
        return 1 if failed else 0


def ssh(cmd, stdin=None, timeout=60):
    return subprocess.run(["ssh", HOST, cmd], input=stdin, capture_output=True, text=True, timeout=timeout).stdout


def api(method, path, body=None):
    if body is None:
        out = ssh(TOKEN + f'curl -s -X {method} -H "X-Token: $T" $B{path}')
    else:
        out = ssh(TOKEN + f'curl -s -X {method} -H "X-Token: $T" -H "Content-Type: application/json" --data-binary @- $B{path}',
                  stdin=json.dumps(body))
    try:
        return json.loads(out)
    except ValueError:
        return {"raw": out}


def board_key(usage, mods="00", ms=30):
    ssh(f"/usr/bin/python3 - {mods} {usage} {ms}", stdin=BOARD_KEY)


def build_sink():
    subprocess.run(["scp", "-q", os.path.join(HERE, "macos_capture", "sink.swift"), f"{HOST}:/tmp/typedeck-sink.swift"], check=True)
    ssh(f"swiftc -O /tmp/typedeck-sink.swift -o {SINK_BIN} && rm /tmp/typedeck-sink.swift", timeout=180)


def start_sink():
    plist = f"""<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict><key>Label</key><string>{LABEL}</string>
<key>ProgramArguments</key><array><string>{SINK_BIN}</string><string>{SINK_OUT}</string></array>
<key>RunAtLoad</key><true/></dict></plist>"""
    ssh(f"rm -f {SINK_OUT}; cat > /tmp/{LABEL}.plist <<'EOF'\n{plist}\nEOF\n"
        f"launchctl bootout gui/$(id -u)/{LABEL} 2>/dev/null; launchctl bootstrap gui/$(id -u) /tmp/{LABEL}.plist")
    time.sleep(2)


def stop_sink(remove_bin=True):
    ssh(f"launchctl bootout gui/$(id -u)/{LABEL} 2>/dev/null; rm -f /tmp/{LABEL}.plist {SINK_OUT}"
        + (f" {SINK_BIN}" if remove_bin else ""))


def sink():
    out = ssh(f"cat {SINK_OUT} 2>/dev/null")
    text = out.split("\nKEYS:")[0].removeprefix("TEXT:")
    keys = out.split("\nKEYS:")[1].split("\n")[0].split(",") if "\nKEYS:" in out else []
    front = "\nFRONT:1" in out
    return text, [k for k in keys if k], front
