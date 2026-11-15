import { h, ic, pageHead } from "./dom.js";
import { norm, hex } from "./util.js";
import { t } from "./i18n.js";
import { legendLang, layout, keyName } from "./layouts.js";
import { keyboard } from "./keyboard.js";
import { state, describe, layoutId } from "./store.js";

const GESTURES = ["tap", "hold", "double"];

function rows(keys, L) {
  const order = new Map(L.keys.map((k, i) => [k.usage, i]));
  const ids = Object.keys(keys).sort((a, b) => (order.get(parseInt(a, 16)) ?? 1e9) - (order.get(parseInt(b, 16)) ?? 1e9));
  const out = [];
  for (const id of ids) {
    const kd = keys[id], name = keyName(parseInt(id, 16));
    for (const g of GESTURES) {
      const a = kd[g];
      if (a) out.push(h("tr", { dataset: { q: norm([name, t("gesture." + g), kd.label, describe(a), t("type." + a.type)].join(" ")) } },
        h("td", { class: "k" }, h("kbd", {}, name)), h("td", { class: "dim" }, t("gesture." + g)), h("td", {}, kd.label || describe(a)), h("td", { class: "faint" }, t("type." + a.type))));
    }
  }
  return out;
}

export function sheetView(root) {
  const L = layout(layoutId(), legendLang());
  const blocks = [];
  const add = (name, keys, color, icon, isGlobal) => {
    const get = (u) => {
      const own = keys[hex(u)], inh = isGlobal ? undefined : state.cfg.global[hex(u)];
      return { kd: own || inh, inherited: !own && !!inh };
    };
    const r = rows(keys, L);
    blocks.push(h("section", { class: "sheet-block" }, h("h2", {}, ic(icon || "layers", 16), name),
      h("div", { class: "mini sheet-kb" }, keyboard(layoutId(), legendLang(), get, { maxU: 32, minU: 14, labels: true, titles: false, color: color || state.cfg.settings.accent }).el),
      r.length ? h("table", { class: "macro-table" }, h("thead", {}, h("tr", {}, h("th", {}, t("sheet.key")), h("th", {}, t("sheet.gesture")), h("th", {}, t("sheet.does")), h("th", {}, t("sheet.kind")))), h("tbody", {}, r))
        : h("p", { class: "faint" }, t("sheet.empty_layer"))));
  };
  state.cfg.layers.forEach((l, i) => add(`${i + 1}. ${l.name}`, l.keys, l.color, l.icon, false));
  if (Object.keys(state.cfg.global || {}).length) add(t("strip.global"), state.cfg.global, state.cfg.settings.accent, "globe", true);
  const search = h("input", { type: "search", placeholder: t("sheet.search"), "aria-label": t("sheet.search"), spellcheck: false });
  const none = h("p", { class: "empty", hidden: true }, t("sheet.none"));
  search.addEventListener("input", () => {
    const q = norm(search.value.trim());
    let any = false;
    for (const block of blocks) {
      let shown = 0;
      for (const tr of block.querySelectorAll("tbody tr")) { const hit = !q || tr.dataset.q.includes(q); tr.hidden = !hit; if (hit) shown++; }
      block.hidden = !!q && shown === 0;
      any ||= !block.hidden;
    }
    none.hidden = any;
  });
  root.append(h("div", { class: "page wide" },
    pageHead(t("sheet.sub"), h("label", { class: "searchbox no-print" }, ic("search", 16), search),
      h("button", { type: "button", class: "btn no-print", onclick: () => window.print() }, ic("printer", 14), t("sheet.print"))), blocks, none));
  return {};
}
