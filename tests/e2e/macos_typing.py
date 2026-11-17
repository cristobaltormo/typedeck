#!/usr/bin/env python3
import sys, time

from macos_common import Checks, api, build_sink, sink, ssh, start_sink, stop_sink

TEXTS = [
    "Hola mundo 123",
    "Hola, ¿qué tal? Ñandú ñ ç",
    "@#{}[]\\|~ <> - _ ; : . , ' \" !",
    "á é í ó ú ü Á É",
    "ABC def GHI",
    "€ º ª",
    "` ^ ´ ¨ ~n ^a",
    "Línea con ~/tmp y C:\\Users\\x",
]
check = Checks()


def hotkey(keys):
    return api("POST", "/api/test", {"type": "hotkey", "keys": keys})


def clear():
    hotkey("cmd+a")
    hotkey("backspace")
    time.sleep(0.4)


def type_text(text):
    clear()
    api("POST", "/api/test", {"type": "text", "text": text})
    deadline = time.time() + 5
    got = ""
    while time.time() < deadline:
        time.sleep(0.4)
        got, _, _ = sink()
        if got == text:
            break
    return got


def in_front():
    _, _, front = sink()
    return front


def run(mode):
    for text in TEXTS:
        if not in_front():
            check(f"{mode}: the test window is still in front", False)
            return
        got = type_text(text)
        check(f"typing through the {mode}: {text!r}", got == text, repr(got))


def main():
    status = api("GET", "/api/status")
    check("the board is connected", status.get("connected") and status.get("capture") == "board", status)
    print("layout:", api("GET", "/api/setup").get("host", {}).get("typing_layout"), flush=True)
    clipboard = ssh("pbpaste")
    try:
        build_sink()
        start_sink()
        front = in_front()
        check("the test window is in front (needed to type safely)", front)
        if not front:
            sys.exit(check.finish())

        run("board")
        type_text("x")
        hotkey("cmd+a")
        hotkey("cmd+x")
        time.sleep(0.6)
        text, keys, _ = sink()
        cut = ssh("pbpaste")
        check("cmd+x through the board cuts the selection", cut == "x" and text == "", (cut, text))
        check("the window got Command with the letter, not Option", "7+cmd" in keys, ",".join(keys[-4:]))

        resp = api("POST", "/api/capture", {"mode": "software", "consent": True})
        check("software capture turns on", resp.get("capture") == "software", str(resp)[:160])
        if resp.get("capture") == "software":
            time.sleep(1)
            run("software path")
    finally:
        api("POST", "/api/capture", {"mode": "board"})
        stop_sink()
        ssh("pbcopy", stdin=clipboard)
    check("back to the board", api("GET", "/api/status").get("capture") == "board")
    sys.exit(check.finish())


main()
