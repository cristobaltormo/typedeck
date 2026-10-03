import { h, ic, clear } from "./dom.js";
import { api } from "./api.js";
import { t, lang } from "./i18n.js";
import { legend, legendLang } from "./layouts.js";
import { toggle, select, armed } from "./controls.js";
import { toast } from "./toast.js";
import { state, edit } from "./store.js";
import { norm } from "./text.js";

const clock = (ms) => new Date(ms).toLocaleTimeString(lang(), { hour: "2-digit", minute: "2-digit", second: "2-digit" });
const day = (ms) => new Date(ms).toLocaleDateString(lang(), { weekday: "short", day: "numeric", month: "short" });

function label(u, fallback) {
  const l = legend(u, legendLang());
  if (l?.icon) return `${t("key.arrow")} ${t("arrow." + l.icon)}`;
  return l?.main || fallback;
}

const keycap = (u, k) => h("span", { class: "keycap" }, label(u, k));

const enabled = () => !!state.cfg.settings.key_history;

export function historyView(root) {
  let data = null, query = "", limit = 50, timer = null;
  const head = h("div", { class: "hist-switch" });
  const body = h("div");
  const box = h("textarea", { class: "transcript", readonly: true, spellcheck: false, rows: 8, "aria-label": t("hist.box_title"), placeholder: t("hist.box_ph") });
  const copyBtn = h("button", { type: "button", class: "btn sm", onclick: async () => { try { await navigator.clipboard.writeText(box.value); toast(t("hist.copied")); } catch { box.select(); } } }, ic("copy", 15), t("hist.copy"));
  const boxSection = h("section", { class: "transcript-wrap" },
    h("div", { class: "row wrap", style: { gap: "10px", marginBottom: "8px" } },
      h("h2", { class: "grow", style: { border: 0, padding: 0, fontSize: "14px" } }, t("hist.box_title"), h("span", { class: "livepill", hidden: !enabled() }, t("hist.live"))), copyBtn),
    box, h("p", { class: "help" }, t("hist.box_help")));

  function showText(text) {
    if (box.value === text) return;
    const atEnd = box.scrollTop + box.clientHeight >= box.scrollHeight - 24;
    box.value = text;
    if (atEnd) box.scrollTop = box.scrollHeight;
  }


  function drawSwitch() {
    clear(head).append(
      h("div", { class: "grow" }, h("b", {}, t("hist.switch")), h("p", { class: "dim" }, t(enabled() ? "hist.on_text" : "hist.off_text"))),
      toggle("", enabled(), (v) => {
        edit("key_history", (cfg) => { cfg.settings.key_history = v; });
        setTimeout(load, 900);
        drawSwitch();
      }));
  }

  function stat(value, text) { return h("div", { class: "stat" }, h("b", {}, value), h("span", {}, text)); }

  function drawBody() {
    clear(body);
    const entries = data?.entries || [];
    if (!data || (!entries.length && !data.count)) {
      body.append(h("div", { class: "empty-state" }, ic("history", 28), h("p", {}, t(enabled() ? "hist.empty_on" : "hist.empty_off"))));
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
          h("div", { class: "bar" }, h("i", { style: { width: `${Math.max(3, (k.n / max) * 100)}%` } })), h("span", { class: "dim n" }, String(k.n)))))));
    }

    const search = h("input", { type: "search", placeholder: t("hist.filter"), "aria-label": t("hist.filter"), value: query, spellcheck: false });
    search.addEventListener("input", () => { query = search.value; drawList(); search.focus(); });
    const list = h("div", { class: "rowlist" });
    function drawList() {
      clear(list);
      const rows = entries.filter((e) => !query || norm(label(e.u, e.k) + " " + e.k).includes(norm(query)));
      if (!rows.length) { list.append(h("div", { class: "empty-state" }, h("p", {}, t("hist.no_match")))); return; }
      let lastDay = "";
      for (const e of rows) {
        const d = day(e.t);
        if (d !== lastDay) { list.append(h("div", { class: "daysep" }, d)); lastDay = d; }
        list.append(h("div", { class: "li" }, h("time", {}, clock(e.t)), keycap(e.u, e.k),
          h("div", { class: "bar grow" }, h("i", { style: { width: `${Math.min(100, Math.max(2, e.d / 4))}%` } })), h("span", { class: "ms" }, `${e.d} ms`)));
      }
    }
    body.append(h("section", { class: "section" },
      h("div", { class: "row wrap", style: { gap: "10px", marginBottom: "6px" } },
        h("h2", { class: "grow", style: { border: 0, padding: 0 } }, t("hist.latest"), enabled() ? h("span", { class: "livepill" }, t("hist.live")) : null),
        h("div", { class: "searchbox" }, ic("search", 16), search),
        select("", String(limit), [["25", "25"], ["50", "50"], ["100", "100"], ["200", "200"]], (v) => { limit = Number(v); load(); })),
      list));
    drawList();

    body.append(h("section", { class: "section danger-zone" }, h("h2", {}, t("hist.delete")),
      h("div", { class: "row wrap", style: { gap: "14px", paddingTop: "12px" } },
        h("p", { class: "dim grow", style: { margin: 0 } }, t("hist.delete_text", { n: s.total })),
        armed("hist.clear", async () => { await api("/api/history/clear", { method: "POST", body: {} }); toast(t("hist.cleared")); load(); }, { icon: "trash" }))));
  }

  const signature = (d) => `${d.enabled}|${d.count}|${d.entries[0]?.t}|${limit}`;
  let quick = null;

  async function load() {
    let next;
    try { next = await api(`/api/history?limit=${limit}`); } catch { return; }
    showText(next.text || "");
    if (data && signature(next) === signature(data)) return;
    data = next;
    const hadFocus = document.activeElement?.type === "search" && body.contains(document.activeElement);
    drawBody();
    if (hadFocus) { const input = body.querySelector('input[type="search"]'); input?.focus(); input?.setSelectionRange(query.length, query.length); }
  }

  root.append(h("div", { class: "page" },
    h("header", {}, h("h1", {}, t("hist.title")), h("p", {}, t("hist.sub"))),
    head, h("p", { class: "privacy" }, ic("shield", 15), t("hist.privacy")), boxSection, body));
  drawSwitch(); load();
  timer = setInterval(() => { if (!document.hidden && enabled()) load(); }, 1500);
  return {
    unmount() { clearInterval(timer); clearTimeout(quick); },
    on(topic, detail) {
      if (topic === "cfg") { drawSwitch(); boxSection.querySelector(".livepill").hidden = !enabled(); }
      if (topic === "key" && detail && !detail.down && enabled()) { clearTimeout(quick); quick = setTimeout(load, 120); }
    },
  };
}
