import { h, ic, clear } from "./dom.js";
import { api } from "./api.js";
import { t, lang } from "./i18n.js";
import { legendLang, legend } from "./layouts.js";
import { keyboard } from "./keyboard.js";
import { toast } from "./toast.js";
import { state, layoutId } from "./store.js";

const fmt = (ts) => new Date(ts * 1000).toLocaleTimeString(lang(), { hour: "2-digit", minute: "2-digit", second: "2-digit" });
const KINDS = ["exec", "test", "layer", "board", "keyboard"];

export function activityView(root) {
  const stats = h("div", { class: "stats" });
  const heat = h("div", { class: "mini", style: { display: "flex", justifyContent: "center", padding: "10px 0 6px" } });
  const log = h("div", { class: "rowlist" });

  async function load() {
    try { state.stats = await api("/api/stats"); } catch {  }
    draw();
  }

  const stat = (v, label) => h("div", { class: "stat" }, h("b", {}, String(v)), h("span", {}, label));

  function draw() {
    const s = state.stats || { total: 0, keys: {}, days: {} };
    const today = s.days?.[new Date().toLocaleDateString("sv")] || 0;
    const top = Object.entries(s.keys).sort((a, b) => b[1] - a[1])[0];
    const errors = state.events.filter((e) => e.kind === "exec" && e.ok === false).length;
    clear(stats).append(stat(s.total, t("activity.total")), stat(today, t("activity.today")), stat(top ? (legend(parseInt(top[0], 16), legendLang()).main || top[0]) : "-", t("activity.top")), stat(errors, t("activity.errors")));
    const max = Math.max(1, ...Object.values(s.keys));
    clear(heat).append(keyboard(layoutId(), legendLang(), () => ({}), { u: 30, labels: false, titles: false, heat: (u) => (s.keys[u.toString(16).toUpperCase().padStart(2, "0")] || 0) / max,
      count: (u) => s.keys[u.toString(16).toUpperCase().padStart(2, "0")] || "" }).el);
  }

  function row(e) {
    const ago = h("time", {}, fmt(e.ts));
    if (e.kind === "exec" || e.kind === "test") {
      return h("div", { class: "li" }, ago, h("span", { class: `res ${e.ok ? "" : "bad"}` }),
        h("div", { class: "grow" }, h("b", { class: "strong", style: { fontWeight: 600 } }, e.kind === "test" ? t("activity.test") : e.label || e.key), e.kind === "exec" ? h("span", { class: "faint" }, `  ${e.key} ${t("gesture." + e.gesture)}, ${e.layer_name}`) : null,
          e.output ? h("div", { class: "faint mono", style: { overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" } }, e.output) : null), h("span", { class: "ms" }, `${e.ms} ms`));
    }
    if (e.kind === "layer") return h("div", { class: "li" }, ago, ic("layers", 15), h("div", { class: "grow dim" }, t("activity.layer", { name: e.name }) + (e.auto ? ` (${t("activity.auto")})` : "")));
    if (e.kind === "board") return h("div", { class: "li" }, ago, h("span", { class: `res ${e.connected ? "" : "bad"}` }), h("div", { class: "grow dim" }, e.connected ? t("activity.board_on") : t("activity.board_off")));
    if (e.kind === "keyboard") return h("div", { class: "li" }, ago, h("span", { class: `res ${e.connected ? "" : "bad"}` }), h("div", { class: "grow dim" }, e.connected ? t("activity.kbd_on") : t("activity.kbd_off")));
    return null;
  }

  function drawLog() {
    clear(log);
    const rows = state.events.filter((e) => KINDS.includes(e.kind)).slice(-80).reverse().map(row).filter(Boolean);
    if (!rows.length) log.append(h("div", { class: "empty-state" }, ic("activity", 26), h("p", {}, t("activity.empty"))));
    else log.append(...rows);
  }

  root.append(h("div", { class: "page" }, h("header", {}, h("h1", {}, t("activity.title")), h("p", {}, t("activity.sub"))), stats,
    h("section", { class: "section" }, h("div", { class: "row" }, h("h2", { class: "grow", style: { border: 0, padding: 0 } }, t("activity.usage")),
      h("button", { type: "button", class: "btn sm", onclick: async () => { await api("/api/stats/reset", { method: "POST", body: {} }); toast(t("activity.reset_done")); load(); } }, t("activity.reset"))), heat),
    h("section", { class: "section" }, h("h2", {}, t("activity.log")), log)));
  draw(); drawLog(); load();
  return { on(topic, ev) { if (topic === "event" && KINDS.includes(ev.kind)) { drawLog(); if (ev.kind === "exec") load(); } } };
}
