import { h, ic, clear, pageHead, emptyState } from "./dom.js";
import { api } from "./api.js";
import { t, lang } from "./i18n.js";
import { keyName } from "./layouts.js";
import { toggle, select, armed } from "./controls.js";
import { toast, guarded } from "./toast.js";
import { state, edit } from "./store.js";
import { norm } from "./util.js";

const clock = (ms) => new Date(ms).toLocaleTimeString(lang(), { hour: "2-digit", minute: "2-digit", second: "2-digit" });
const day = (ms) => new Date(ms).toLocaleDateString(lang(), { weekday: "short", day: "numeric", month: "short" });
const label = (u, fallback) => (u === undefined ? fallback : keyName(u)) || fallback;
const keycap = (u, k) => h("kbd", {}, label(u, k));
const enabled = () => !!state.cfg.settings.key_history;

export function historyView(root) {
  let data = null, query = "", limit = 50, timer = null, quick = null;
  const head = h("div", { class: "card hist-switch" });
  const body = h("div");
  const box = h("textarea", { class: "transcript", readonly: true, spellcheck: false, rows: 8, "aria-label": t("hist.box_title"), placeholder: t("hist.box_ph") });
  const livePill = h("span", { class: "live-pill" }, t("hist.live"));
  const copyBtn = h("button", { type: "button", class: "btn sm", onclick: async () => { try { await navigator.clipboard.writeText(box.value); toast(t("hist.copied")); } catch { box.select(); } } }, ic("copy", 14), t("hist.copy"));
  const boxSection = h("section", { class: "section" },
    h("div", { class: "section-head" }, h("h2", {}, t("hist.box_title"), livePill), copyBtn),
    box, h("p", { class: "help" }, t("hist.box_help")));

  function showText(text) {
    if (box.value === text) return;
    const atEnd = box.scrollTop + box.clientHeight >= box.scrollHeight - 24;
    box.value = text;
    if (atEnd) box.scrollTop = box.scrollHeight;
  }

  function drawSwitch() {
    livePill.hidden = !enabled();
    clear(head).append(
      h("div", { class: "grow" }, h("b", {}, t("hist.switch")), h("p", { class: "dim" }, t(enabled() ? "hist.on_text" : "hist.off_text"))),
      select("", String(state.cfg.settings.key_history_days || 30), [1, 7, 30, 90, 365].map((d) => [String(d), t("hist.days", { n: d })]), (v) => edit("key_history_days", (cfg) => { cfg.settings.key_history_days = Number(v); }), null, t("hist.keep_help")),
      toggle("", enabled(), (v) => {
        edit("key_history", (cfg) => { cfg.settings.key_history = v; });
        setTimeout(load, 900);
        drawSwitch();
      }, null, t("hist.switch")));
  }

  const stat = (value, text) => h("div", { class: "stat" }, h("b", {}, value), h("span", {}, text));

  function drawBody() {
    clear(body);
    const entries = data?.entries || [];
    if (!data || (!entries.length && !data.count)) {
      body.append(emptyState("history", t(enabled() ? "hist.empty_on" : "hist.empty_off")));
      return;
    }
    const s = data.summary;
    const top = s.top?.[0];
    body.append(h("div", { class: "stats" },
      stat(String(s.total), t("hist.total")), stat(String(s.today), t("hist.today")),
      stat(top ? label(top.u, top.k) : "-", t("hist.top_key")), stat(`${s.avg_ms} ms`, t("hist.avg"))));

    if (s.top?.length) {
      const max = s.top[0].n;
      body.append(h("section", { class: "section" }, h("h2", {}, t("hist.top")),
        h("div", { class: "toplist" }, s.top.map((k) => h("div", { class: "toprow" }, keycap(k.u, k.k),
          h("div", { class: "bar" }, h("i", { style: { width: `${Math.max(3, (k.n / max) * 100)}%` } })), h("span", { class: "n" }, String(k.n)))))));
    }

    const search = h("input", { type: "search", placeholder: t("hist.filter"), "aria-label": t("hist.filter"), value: query, spellcheck: false });
    search.addEventListener("input", () => { query = search.value; drawList(); });
    const list = h("div", { class: "rowlist" });
    function drawList() {
      clear(list);
      const rows = entries.filter((e) => !query || norm(label(e.u, e.k) + " " + e.k).includes(norm(query)));
      if (!rows.length) { list.append(emptyState(null, t("hist.no_match"))); return; }
      let lastDay = "";
      for (const e of rows) {
        const d = day(e.t);
        if (d !== lastDay) { list.append(h("div", { class: "daysep" }, d)); lastDay = d; }
        list.append(h("div", { class: "li" }, h("time", {}, clock(e.t)), keycap(e.u, e.k),
          h("div", { class: "bar grow" }, h("i", { style: { width: `${Math.min(100, Math.max(2, e.d / 4))}%` } })), h("span", { class: "num" }, `${e.d} ms`)));
      }
    }
    body.append(h("section", { class: "section" },
      h("div", { class: "section-head" }, h("h2", {}, t("hist.latest")),
        h("label", { class: "searchbox sm" }, ic("search", 14), search),
        select("", String(limit), ["25", "50", "100", "200"].map((n) => [n, n]), (v) => { limit = Number(v); load(); }, null, t("hist.limit"))),
      list));
    drawList();

    body.append(h("section", { class: "section danger-zone" }, h("h2", {}, t("hist.delete")),
      h("div", { class: "section-head" },
        h("p", { class: "dim grow" }, t("hist.delete_text", { n: s.total })),
        armed("hist.clear", guarded(async () => { await api("/api/history/clear", { method: "POST", body: {} }); toast(t("hist.cleared")); load(); }), { small: true, icon: "trash" }))));
  }

  const signature = (d) => `${d.enabled}|${d.count}|${d.entries[0]?.t}|${limit}`;

  async function load() {
    let next;
    try { next = await api(`/api/history?limit=${limit}`); } catch { return; }
    if (!root.isConnected) return;
    showText(next.text || "");
    if (data && signature(next) === signature(data)) return;
    data = next;
    const focus = document.activeElement;
    const refocus = body.contains(focus) && focus.matches("input[type=search], select") ? (focus.type === "search" ? "input[type=search]" : "select") : null;
    drawBody();
    if (refocus) { const el = body.querySelector(refocus); el?.focus(); if (el?.type === "search") el.setSelectionRange(query.length, query.length); }
  }

  root.append(h("div", { class: "page" },
    pageHead(t("hist.sub")),
    head, h("p", { class: "privacy" }, ic("shield", 14), t("hist.privacy")), boxSection, body));
  drawSwitch(); load();
  timer = setInterval(() => { if (!document.hidden && enabled()) load(); }, 1500);
  return {
    unmount() { clearInterval(timer); clearTimeout(quick); },
    on(topic, detail) {
      if (topic === "cfg") drawSwitch();
      if (topic === "key" && detail && !detail.down && enabled()) { clearTimeout(quick); quick = setTimeout(load, 120); }
    },
  };
}
