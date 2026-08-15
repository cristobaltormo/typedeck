import { h, ic, clear } from "./dom.js";
import { api } from "./api.js";
import { t, lang } from "./i18n.js";
import { toast } from "./toast.js";
import { state } from "./store.js";

const fmtTime = (ts) => (ts ? new Date(ts * 1000).toLocaleTimeString(lang()) : "-");
const fmtUp = (s) => { const m = Math.floor(s / 60); return m < 60 ? `${m} min` : `${Math.floor(m / 60)} h ${m % 60} min`; };

export function diagView(root) {
  const info = h("dl", { class: "kv" });
  const sys = h("dl", { class: "kv" });
  const out = h("div", { class: "console", style: { minHeight: "56px" } }, t("diag.out_idle"));
  const raw = h("div", { class: "rowlist" });
  let stats = null, system = null;

  function drawInfo() {
    const s = state.status, k = state.keyboard.info;
    clear(info).append(
      h("dt", {}, t("diag.board")), h("dd", {}, h("span", { class: `dot ${s.connected ? "on" : "bad"}`, style: { display: "inline-block", marginRight: "8px" } }), s.connected ? t("diag.connected") : t("diag.disconnected")),
      h("dt", {}, t("diag.port")), h("dd", { class: "mono" }, s.port || "-"),
      h("dt", {}, t("diag.firmware")), h("dd", {}, s.firmware ? `Typedeck FW ${s.firmware}` : "-"),
      h("dt", {}, t("diag.since")), h("dd", {}, fmtTime(s.connected_since)),
      h("dt", {}, t("diag.keyboard")), h("dd", {}, k?.present ? [k.mfr, k.prod].filter(Boolean).join(" ") + ` (${k.vid}:${k.pid})` : t("diag.kbd_none")),
      h("dt", {}, t("diag.latency")), h("dd", {}, stats && stats.n !== "0" ? `${stats.media_us} us ${t("diag.avg")}, ${stats.max_us} us ${t("diag.max")} (${stats.n})` : t("diag.no_samples")),
      h("dt", {}, t("diag.capture")), h("dd", {}, stats ? (stats.captura === "1" ? t("diag.capture_on") : t("diag.capture_off")) : t("diag.unknown")),
      h("dt", {}, t("diag.front")), h("dd", {}, (s.front || []).join(", ") || "-"),
      h("dt", {}, t("diag.daemon")), h("dd", {}, `Typedeck ${s.version || ""}, ${t("diag.up")} ${fmtUp(s.uptime || 0)}`));
  }

  function drawSys() {
    clear(sys);
    if (!system) { sys.append(h("dt", {}, ""), h("dd", { class: "dim" }, t("diag.unknown"))); return; }
    const remap = system.mod_remap.length ? system.mod_remap.join(", ") + " " + t("diag.compensated") : t("diag.none");
    sys.append(h("dt", {}, t("diag.mac_layout")), h("dd", {}, t("layout.sys." + system.typing_layout)));
    sys.append(
      h("dt", {}, t("diag.mac_kbtype")), h("dd", {}, system.keyboard_type ? system.keyboard_type.toUpperCase() + (system.keys_swapped ? " " + t("diag.compensated") : "") : t("diag.unknown")),
      h("dt", {}, t("diag.mac_remap")), h("dd", {}, remap));
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

  function drawRaw() {
    clear(raw);
    const evs = state.events.filter((e) => e.kind === "down" || e.kind === "up").slice(-24).reverse();
    if (!evs.length) raw.append(h("div", { class: "empty-state" }, h("p", {}, t("diag.raw_empty"))));
    for (const e of evs) raw.append(h("div", { class: "li" }, h("time", {}, fmtTime(e.ts)), h("span", { class: "mono" }, e.kind === "down" ? "D" : "U"), h("span", { class: "grow mono" }, e.key)));
  }

  root.append(h("div", { class: "page" }, h("header", {}, h("h1", {}, t("diag.title")), h("p", {}, t("diag.sub"))),
    h("section", { class: "section" }, h("h2", {}, t("diag.status")), h("div", { style: { paddingTop: "14px" } }, info)),
    h("section", { class: "section" }, h("h2", {}, t("diag.mac")), h("p", { class: "sub" }, t("diag.mac_sub")), h("div", { style: { paddingTop: "14px" } }, sys)),
    h("section", { class: "section" }, h("h2", {}, t("diag.tests")),
      h("div", { class: "stack", style: { paddingTop: "14px", gap: "12px" } },
        h("div", { class: "row wrap" }, btn("Ping", () => cmd("ping"), "pulse"), btn(t("diag.stats"), () => cmd("stats"), "activity"), btn(t("diag.leds"), () => cmd("leds", 7), "bolt"),
          btn(t("diag.layer_blink"), () => cmd("layer", state.status.layer), "layers"), btn(t("diag.hud_test"), () => api("/api/hud", { method: "POST", body: { title: "Typedeck", subtitle: t("diag.hud_sub") } }), "hud")), out)),
    h("section", { class: "section" }, h("h2", {}, t("diag.raw")), raw),
    h("section", { class: "section" }, h("h2", {}, t("diag.help")), h("div", { class: "stack", style: { paddingTop: "12px", gap: "8px" } }, ["diag.h1", "diag.h2", "diag.h3"].map((k) => h("p", { class: "dim" }, t(k)))))));
  drawInfo(); drawSys(); drawRaw(); cmd("stats");
  api("/api/system").then((s) => { system = s; drawSys(); }).catch(() => {});
  return { on(topic, ev) { if (topic === "status" || topic === "keyboard") drawInfo(); if (topic === "event" && (ev.kind === "down" || ev.kind === "up")) drawRaw(); } };
}
