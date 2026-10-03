"""A tiny Chrome DevTools Protocol client over a raw WebSocket (standard library only), for tests and screen captures that need real
mouse input. Usage: with Browser(url, size) as b: b.eval("..."); b.mouse("mousePressed", x, y); b.png()"""
import base64, json, os, shutil, socket, struct, subprocess, tempfile, time, urllib.request

CHROME = shutil.which("chromium") or shutil.which("google-chrome") or shutil.which("chromium-browser") or shutil.which("chrome")

class Browser:
    def __init__(self, url, size=(1280, 720), port=9333):
        self.url, self.size, self.port, self.n = url, size, port, 0
    def __enter__(self):
        if not CHROME: raise SystemExit("Chrome or Chromium is missing")
        self.dir = tempfile.mkdtemp(prefix="typedeck-cdp-")
        self.proc = subprocess.Popen([CHROME, "--headless=new", "--no-sandbox", "--disable-gpu", "--hide-scrollbars", f"--window-size={self.size[0]},{self.size[1]}",
            f"--remote-debugging-port={self.port}", f"--user-data-dir={self.dir}", self.url], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        ws = None
        for _ in range(60):
            time.sleep(0.25)
            try:
                tabs = json.load(urllib.request.urlopen(f"http://127.0.0.1:{self.port}/json"))
                ws = next(t["webSocketDebuggerUrl"] for t in tabs if t["type"] == "page")
                break
            except Exception: pass
        if not ws: raise SystemExit("could not reach the browser")
        self.sock = socket.create_connection(("127.0.0.1", self.port))
        path = ws.split(str(self.port), 1)[1]
        key = base64.b64encode(os.urandom(16)).decode()
        self.sock.sendall((f"GET {path} HTTP/1.1\r\nHost: 127.0.0.1:{self.port}\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: {key}\r\nSec-WebSocket-Version: 13\r\n\r\n").encode())
        buf = b""
        while b"\r\n\r\n" not in buf: buf += self.sock.recv(4096)
        self.buf = buf.split(b"\r\n\r\n", 1)[1]
        self.call("Page.enable")
        return self
    def __exit__(self, *a):
        try: self.sock.close()
        finally:
            self.proc.terminate(); shutil.rmtree(self.dir, ignore_errors=True)
    def _read(self, n):
        while len(self.buf) < n: self.buf += self.sock.recv(1 << 16)
        out, self.buf = self.buf[:n], self.buf[n:]
        return out
    def _frame(self):
        h = self._read(2); ln = h[1] & 0x7F
        if ln == 126: ln = struct.unpack(">H", self._read(2))[0]
        elif ln == 127: ln = struct.unpack(">Q", self._read(8))[0]
        return h[0] & 0x0F, self._read(ln)
    def call(self, method, **params):
        self.n += 1
        data = json.dumps({"id": self.n, "method": method, "params": params}).encode()
        mask = os.urandom(4); ln = len(data)
        head = bytes([0x81]) + (bytes([0x80 | ln]) if ln < 126 else bytes([0x80 | 126]) + struct.pack(">H", ln) if ln < 65536 else bytes([0x80 | 127]) + struct.pack(">Q", ln))
        self.sock.sendall(head + mask + bytes(b ^ mask[i % 4] for i, b in enumerate(data)))
        while True:
            op, payload = self._frame()
            if op != 1: continue
            msg = json.loads(payload)
            if msg.get("id") == self.n:
                if "error" in msg: raise RuntimeError(msg["error"])
                return msg.get("result", {})
    def eval(self, js):
        r = self.call("Runtime.evaluate", expression=js, returnByValue=True, awaitPromise=True)
        return r.get("result", {}).get("value")
    def mouse(self, kind, x, y, **kw):
        return self.call("Input.dispatchMouseEvent", type=kind, x=x, y=y, button="left", clickCount=1, **kw)
    def png(self):
        return base64.b64decode(self.call("Page.captureScreenshot", format="png")["data"])
