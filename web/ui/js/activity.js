import { h, ic, clear, pageHead, emptyState } from "./dom.js";
import { api } from "./api.js";
import { t, lang } from "./i18n.js";
import { legendLang, keyName } from "./layouts.js";
import { keyboard } from "./keyboard.js";
import { armed } from "./controls.js";
import { toast, guarded } from "./toast.js";
import { state, layoutId } from "./store.js";
import { hex } from "./util.js";

const fmt = (ts) => new Date(ts * 1000).toLocaleTimeString(lang(), { hour: "2-digit", minute: "2-digit", second: "2-digit" });
const KINDS = ["exec", "test", "layer", "board", "keyboard"];

export function activityView(root) {
  const stats = h("div", { class: "stats" });
  const heat = h("div", { class: "mini heat-host" });
  const log = h("div", { class: "rowlist" });

  async function load() {
    try { state.stats = await api("/api/stats"); } catch {}
    if (root.isConnected) draw();
  }

  const stat = (v, label) => h("div", { class: "stat" }, h("b", {}, String(v)), h("span", {}, label));

  function draw() {
    const s = state.stats || { total: 0, keys: {}, days: {} };
    const keys = s.keys || {};
    const today = s.days?.[new Date().toLocaleDateString("sv")] || 0;
    const top = Object.entries(keys).sort((a, b) => b[1] - a[1])[0];
    const errors = state.events.filter((e) => e.kind === "exec" && e.ok === false).length;
    clear(stats).append(stat(s.total, t("activity.total")), stat(today, t("activity.today")), stat(top ? keyName(parseInt(top[0], 16)) : "-", t("activity.top")), stat(errors, t("activity.errors")));
    const max = Math.max(1, ...Object.values(keys));
    clear(heat).append(keyboard(layoutId(), legendLang(), () => ({}), { maxU: 34, minU: 14, labels: false, titles: false, heat: (u) => (keys[hex(u)] || 0) / max, count: (u) => keys[hex(u)] || "" }).el);
  }

  function row(e) {
    const ago = h("time", {}, fmt(e.ts));
    const dot = (ok) => h("span", { class: `res ${ok ? "ok" : "bad"}`, "aria-label": t(ok ? "activity.ok" : "activity.failed") });
    if (e.kind === "exec" || e.kind === "test") {
      return h("div", { class: "li" }, ago, dot(e.ok),
        h("div", { class: "grow" }, h("b", {}, e.kind === "test" ? t("activity.test") : e.label || e.key),
          e.kind === "exec" ? h("span", { class: "faint" }, ` ${e.key} · ${t("gesture." + e.gesture)} · ${e.layer_name}`) : null,
          e.output ? h("div", { class: "faint mono ellipsis" }, e.output) : null), e.ms !== undefined ? h("span", { class: "num" }, `${e.ms} ms`) : null);
    }
    if (e.kind === "layer") return h("div", { class: "li" }, ago, ic("layers", 14), h("div", { class: "grow dim" }, t("activity.layer", { name: e.name }) + (e.auto ? ` (${t("activity.auto")})` : "")));
    if (e.kind === "board") return h("div", { class: "li" }, ago, dot(e.connected), h("div", { class: "grow dim" }, e.connected ? t(e.name === "software" ? "activity.cap_on" : "activity.board_on") : t(e.name === "software" ? "activity.cap_off" : "activity.board_off")));
    if (e.kind === "keyboard") return h("div", { class: "li" }, ago, dot(e.connected), h("div", { class: "grow dim" }, e.connected ? t("activity.kbd_on") : t("activity.kbd_off")));
    return null;
  }

  function drawLog() {
    clear(log);
    const rows = state.events.filter((e) => KINDS.includes(e.kind)).slice(-80).reverse().map(row).filter(Boolean);
    if (!rows.length) log.append(emptyState("activity", t("activity.empty")));
    else log.append(...rows);
  }

  root.append(h("div", { class: "page" }, pageHead(t("activity.sub")), stats,
    h("section", { class: "section" }, h("div", { class: "section-head" }, h("h2", {}, t("activity.usage")),
      armed("activity.reset", guarded(async () => { await api("/api/stats/reset", { method: "POST", body: {} }); toast(t("activity.reset_done")); load(); }), { small: true })), heat),
    h("section", { class: "section" }, h("h2", {}, t("activity.log")), log)));
  draw(); drawLog(); load();
  return { on(topic, ev) { if (topic === "event" && KINDS.includes(ev.kind)) { drawLog(); if (ev.kind === "exec") load(); } } };
}
