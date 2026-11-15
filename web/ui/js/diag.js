import { h, ic, clear, pageHead, emptyState } from "./dom.js";
import { api } from "./api.js";
import { t, lang } from "./i18n.js";
import { toast, guarded } from "./toast.js";
import { armed } from "./controls.js";
import { state } from "./store.js";
import { isSoftware, osText } from "./capture.js";

const fmtTime = (ts) => (ts ? new Date(ts * 1000).toLocaleTimeString(lang()) : "-");
const fmtUp = (s) => { const m = Math.floor(s / 60); return m < 60 ? `${m} min` : `${Math.floor(m / 60)} h ${m % 60} min`; };

export function diagView(root) {
  const info = h("dl", { class: "kv" });
  const sys = h("dl", { class: "kv" });
  const out = h("div", { class: "console", "aria-live": "polite" }, t("diag.out_idle"));
  const raw = h("div", { class: "rowlist" });
  let stats = null, system = null;

  function drawInfo() {
    const s = state.status, k = state.keyboard.info;
    if (isSoftware()) { drawSoftInfo(s); return; }
    clear(info).append(
      h("dt", {}, t("diag.board")), h("dd", {}, h("span", { class: `dot ${s.connected ? "ok" : "bad"}` }), s.connected ? t("diag.connected") : t("diag.disconnected")),
      h("dt", {}, t("diag.port")), h("dd", { class: "mono" }, s.port || "-"),
      h("dt", {}, t("diag.firmware")), h("dd", {}, s.firmware ? `Typedeck FW ${s.firmware}` : "-"),
      h("dt", {}, t("diag.since")), h("dd", {}, fmtTime(s.connected_since)),
      h("dt", {}, t("diag.keyboard")), h("dd", {}, k?.present ? [k.mfr, k.prod].filter(Boolean).join(" ") + ` (${k.vid}:${k.pid})` : t("diag.kbd_none")),
      h("dt", {}, t("diag.latency")), h("dd", {}, stats && stats.n !== "0" ? `${stats.avg_us} us ${t("diag.avg")}, ${stats.max_us} us ${t("diag.max")} (${stats.n})` : t("diag.no_samples")),
      h("dt", {}, t("diag.capture")), h("dd", {}, stats ? (stats.captura === "1" ? t("diag.capture_on") : t("diag.capture_off")) : t("diag.unknown")),
      h("dt", {}, t("diag.front")), h("dd", {}, (s.front || []).join(", ") || "-"),
      h("dt", {}, t("diag.daemon")), h("dd", {}, `Typedeck ${s.version || ""}, ${t("diag.up")} ${fmtUp(s.uptime || 0)}`));
  }

  function drawSoftInfo(s) {
    clear(info).append(
      h("dt", {}, t("diag.hook")), h("dd", {}, h("span", { class: `dot ${s.connected ? "ok" : "bad"}` }), s.connected ? t("diag.hook_on") : t("diag.hook_off")),
      h("dt", {}, t("diag.since")), h("dd", {}, fmtTime(s.connected_since)),
      h("dt", {}, t("diag.keyboard")), h("dd", {}, t("capture.sys_keyboard")),
      h("dt", {}, t("diag.latency_soft")), h("dd", {}, stats && stats.n !== "0" ? `${stats.avg_us} us ${t("diag.avg")}, ${stats.max_us} us ${t("diag.max")} (${stats.n})` : t("diag.no_samples")),
      h("dt", {}, t("diag.capture")), h("dd", {}, stats ? (stats.captura === "1" ? t("diag.capture_on_soft") : t("diag.capture_off")) : t("diag.unknown")),
      h("dt", {}, t("diag.front")), h("dd", {}, (s.front || []).join(", ") || "-"),
      h("dt", {}, t("diag.daemon")), h("dd", {}, `Typedeck ${s.version || ""}, ${t("diag.up")} ${fmtUp(s.uptime || 0)}`));
  }

  function drawSys() {
    clear(sys);
    if (!system) { sys.append(h("dt", {}, ""), h("dd", { class: "dim" }, t("diag.unknown"))); return; }
    const remap = system.mod_remap.length ? system.mod_remap.join(", ") + " " + t("diag.compensated") : t("diag.none");
    sys.append(h("dt", {}, t("diag.mac_layout")), h("dd", {}, t("layout.sys." + system.typing_layout)));
    if (system.keyboard_type || system.mod_remap.length) {
      sys.append(
        h("dt", {}, t("diag.mac_kbtype")), h("dd", {}, system.keyboard_type ? system.keyboard_type.toUpperCase() + (system.keys_swapped ? " " + t("diag.compensated") : "") : t("diag.unknown")),
        h("dt", {}, t("diag.mac_remap")), h("dd", {}, remap));
    }
  }

  async function cmd(name, arg) {
    try {
      const r = await api("/api/board", { method: "POST", body: { cmd: name, arg } });
      const text = (r.reply || []).join("\n") || t("diag.no_reply");
      out.textContent = text;
      if (name === "stats") { stats = Object.fromEntries(text.split(/\s+/).filter((p) => p.includes("=")).map((p) => p.split("="))); drawInfo(); }
    } catch (e) { toast(e.message, "bad"); out.textContent = e.message; }
  }
  const btn = (label, fn, i) => h("button", { type: "button", class: "btn sm", onclick: fn }, i && ic(i, 15), label);

  function testButtons() {
    const common = [btn("Ping", () => cmd("ping"), "pulse"), btn(t("diag.stats"), () => cmd("stats"), "activity")];
    if (isSoftware()) return common;
    return [...common, btn(t("diag.bootlog"), () => cmd("bootlog"), "clock"), armed("diag.reboot", () => cmd("reboot"), { small: true, icon: "power" }), btn(t("diag.leds"), () => cmd("leds", 7), "bolt"),
      btn(t("diag.layer_blink"), () => cmd("layer", state.status.layer), "layers")];
  }

  function drawRaw() {
    clear(raw);
    const evs = state.events.filter((e) => e.kind === "down" || e.kind === "up").slice(-24).reverse();
    if (!evs.length) raw.append(emptyState(null, t("diag.raw_empty")));
    for (const e of evs) raw.append(h("div", { class: "li" }, h("time", {}, fmtTime(e.ts)), h("span", { class: "mono" }, e.kind === "down" ? "D" : "U"), h("span", { class: "grow mono" }, e.key)));
  }

  const sec = (title, ...kids) => h("section", { class: "section" }, h("h2", {}, title), ...kids);
  root.append(h("div", { class: "page" }, pageHead(t(isSoftware() ? "diag.sub_soft" : "diag.sub")),
    sec(t("diag.status"), info),
    sec(t("diag.mac"), h("p", { class: "sub" }, t("diag.mac_sub")), sys),
    sec(t("diag.tests"), h("div", { class: "section-actions" }, testButtons(), btn(t("diag.hud_test"), guarded(() => api("/api/hud", { method: "POST", body: { title: "Typedeck", subtitle: t("diag.hud_sub") } })), "hud")), out),
    sec(t("diag.raw"), raw),
    sec(t("diag.help"), h("div", { class: "stack tight" }, (isSoftware() ? ["capture.c3", "diag.h3"] : ["diag.h1", "diag.h2", "diag.h3"]).map((k) => h("p", { class: "dim" }, osText(k)))))));
  drawInfo(); drawSys(); drawRaw(); cmd("stats");
  api("/api/system").then((s) => { system = s; drawSys(); }).catch(() => {});
  return { on(topic, ev) { if (topic === "status" || topic === "keyboard") drawInfo(); if (topic === "event" && (ev.kind === "down" || ev.kind === "up")) drawRaw(); } };
}
