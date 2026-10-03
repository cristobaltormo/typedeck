#!/usr/bin/env python3
"""Records docs/images/drag.gif: a macro dragged to another key in the editor, with real mouse input in a headless Chrome.
Needs Chromium or Chrome and Pillow. Usage: scripts/drag-gif.py   (the program is started here with a throwaway configuration)"""
import io, os, subprocess, sys, tempfile, time
sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "tests", "e2e"))
from cdp import Browser
from PIL import Image

root = os.path.join(os.path.dirname(__file__), "..")
subprocess.run(["make", "-s", "build"], cwd=root, check=True)
home = tempfile.mkdtemp(prefix="typedeck-gif-")
prog = subprocess.Popen([os.path.join(root, "dist", "typedeck")], env={**os.environ, "HOME": home, "XDG_CONFIG_HOME": home}, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
try:
    for _ in range(50):
        if os.path.exists(os.path.join(home, "typedeck", "port")): break
        time.sleep(0.1)
    port = open(os.path.join(home, "typedeck", "port")).read().strip()
    frames = []
    with Browser(f"http://127.0.0.1:{port}/?scene=default&nolive", (1280, 640)) as b:
        time.sleep(3)
        def rect(u): return b.eval(f"(() => {{ const r = document.querySelector('.cap[data-u=\"{u}\"]').getBoundingClientRect(); return [r.left + r.width / 2, r.top + r.height / 2]; }})()")
        def snap(n=1):
            for _ in range(n): frames.append(Image.open(io.BytesIO(b.png())).convert("RGB"))
        src, dst = rect(0x3B), rect(0x45)
        b.mouse("mouseMoved", *src); snap(6)
        b.mouse("mousePressed", *src); snap(2)
        steps = 12
        for i in range(1, steps + 1):
            x = src[0] + (dst[0] - src[0]) * i / steps; y = src[1] + (dst[1] - src[1]) * i / steps
            b.mouse("mouseMoved", x, y, buttons=1); snap(1)
        snap(4)
        b.mouse("mouseReleased", *dst); time.sleep(0.4); snap(10)
        moved = b.eval("document.querySelector('.cap[data-u=\"69\"]').classList.contains('mapped')")
    if not moved: sys.exit("the drag did not move the macro")
    w = 960
    small = [f.resize((w, int(f.height * w / f.width)), Image.LANCZOS).convert("P", palette=Image.ADAPTIVE, colors=128) for f in frames]
    out = os.path.join(root, "docs", "images", "drag.gif")
    small[0].save(out, save_all=True, append_images=small[1:], duration=[120] * len(small), loop=0, optimize=True)
    print(out, os.path.getsize(out) // 1024, "KB,", len(small), "frames")
finally:
    prog.terminate()
