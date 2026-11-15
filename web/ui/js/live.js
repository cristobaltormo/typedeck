import { api, token } from "./api.js";
import { state, notify } from "./store.js";
import { isTextField } from "./dom.js";

let ws = null, retry = 0, onEvent = () => {}, onKey = () => {}, editing = false, recording = 0, keepAlive = null, reconnectTimer = null;

export const isLive = () => !!ws && ws.readyState === WebSocket.OPEN;

function wanted() { return recording > 0 || (document.hasFocus() && isTextField(document.activeElement)); }

function post(on) {
  const msg = JSON.stringify({ t: "editing", on });
  if (isLive()) ws.send(msg); else api("/api/editing", { method: "POST", body: { on } }).catch(() => {});
}

function refresh() {
  const on = wanted();
  if (on === editing && !on) return;
  editing = on;
  post(on);
  clearInterval(keepAlive);
  keepAlive = on ? setInterval(() => post(true), 3000) : null;
}

export function holdEditing(on) {
  recording = Math.max(0, recording + (on ? 1 : -1));
  refresh();
}

function connect() {
  clearTimeout(reconnectTimer);
  let sock;
  try { sock = new WebSocket(`ws://${location.host}/api/ws`, [`typedeck.${token}`]); } catch { return schedule(); }
  ws = sock;
  sock.onopen = () => { retry = 0; state.live = true; notify("live"); if (editing) post(true); };
  sock.onmessage = (m) => {
    let d; try { d = JSON.parse(m.data); } catch { return; }
    if (d.t === "event") onEvent(d.ev);
    else if (d.t === "key") onKey(d.k, d.d);
    else if (d.t === "hello") onEvent(null, d.last_id);
    else if (d.t === "focus") { window.focus(); flashTitle(); }
  };
  sock.onclose = () => {
    if (ws !== sock) return;
    ws = null;
    if (state.live) { state.live = false; notify("live"); }
    schedule();
  };
  sock.onerror = () => sock.close();
}

function schedule() {
  retry = Math.min(retry + 1, 6);
  if (retry >= 3) api("/api/status").catch((e) => { if (e.message === "token") location.reload(); });
  reconnectTimer = setTimeout(connect, Math.min(1000 * 2 ** (retry - 1), 15000));
}

let flashing = false;
function flashTitle() {
  if (flashing || document.hasFocus()) return;
  flashing = true;
  const base = document.title;
  let n = 0;
  const id = setInterval(() => { document.title = n++ % 2 ? base : `> ${base}`; if (n > 6 || document.hasFocus()) { clearInterval(id); document.title = base; flashing = false; } }, 500);
}

export function startLive(handler, keyHandler) {
  onEvent = handler;
  if (keyHandler) onKey = keyHandler;
  document.addEventListener("focusin", refresh);
  document.addEventListener("focusout", () => setTimeout(refresh, 0));
  window.addEventListener("blur", refresh);
  window.addEventListener("focus", refresh);
  window.addEventListener("pagehide", () => { if (editing) post(false); });
  document.addEventListener("visibilitychange", () => { if (!document.hidden && !ws) { retry = 0; connect(); } });
  connect();
}
