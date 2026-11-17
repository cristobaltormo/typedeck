#!/usr/bin/env python3
import json, os, sys, time

from macos_common import Checks, api, board_key, build_sink, sink, start_sink, stop_sink

KEY_ID = "3F"
KEY_CODE = "97"
WORD = "tdmac"
PRESS = os.environ.get("PRESS", "")
check = Checks()


def main():
    original = api("GET", "/api/config")
    check("the program answers on the Mac", "layers" in original)
    status = api("GET", "/api/status")
    check("the board is connected before the test", status.get("capture") == "board", status.get("capture"))

    changed = json.loads(json.dumps(original))
    changed["layers"][0]["keys"][KEY_ID] = {"tap": {"type": "text", "text": WORD}}
    try:
        saved = api("POST", "/api/config", changed)
        check("the test macro is saved", "config" in saved, str(saved)[:120])

        build_sink()
        start_sink()
        text, keys, front = sink()
        check("the test window is in front", front)

        resp = api("POST", "/api/capture", {"mode": "software", "consent": True})
        check("software capture turns on with the permissions granted", resp.get("capture") == "software", str(resp)[:160])

        if PRESS == "board":
            print("the board presses F6 through USB, as a keyboard would", flush=True)
            time.sleep(1)
            board_key(KEY_ID)
        else:
            print("PRESS F6 on the keyboard now (40 s)", flush=True)
            gate = os.environ.get("GATE")
            if gate:
                while not os.path.exists(gate):
                    time.sleep(1)
                os.unlink(gate)
        deadline = time.time() + 40
        while time.time() < deadline:
            text, keys, front = sink()
            if WORD in text:
                break
            time.sleep(1)
        check("the macro on F6 typed its text into the window", WORD in text, repr(text))
        check("the F6 key itself did not reach the window", KEY_CODE not in [k.split("+")[0] for k in keys], ",".join(keys))

        resp = api("POST", "/api/capture", {"mode": "board"})
        check("back to the board", resp.get("capture") == "board", str(resp)[:160])
    finally:
        api("POST", "/api/capture", {"mode": "board"})
        api("POST", "/api/config", original)
        stop_sink()
    after = api("GET", "/api/config")
    check("the original configuration is restored", KEY_ID not in after["layers"][0]["keys"])
    sys.exit(check.finish())


main()
