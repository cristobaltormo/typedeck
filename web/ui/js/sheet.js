import { h, ic } from "./dom.js";
import { t, lang } from "./i18n.js";
import { legendLang } from "./layouts.js";
import { keyboard, hex } from "./keyboard.js";
import { layout } from "./layouts.js";
import { state, describe, layoutId } from "./store.js";

function rows(keys, L) {
  const out = [];
  for (const [id, kd] of Object.entries(keys)) {
    const k = L.keys.find((x) => x.usage === parseInt(id, 16));
    const name = k ? (k.legend.icon ? t("key.arrow") : k.legend.main) : id;
    for (const g of ["tap", "hold", "double"]) {
      const a = kd[g];
      if (a) out.push(h("tr", {}, h("td", { class: "k" }, name), h("td", { class: "dim" }, t("gesture." + g)), h("td", {}, kd.label || describe(a)), h("td", { class: "faint" }, t("type." + a.type))));
    }
  }
  return out;
}

export function sheetView(root) {
  const L = layout(layoutId(), legendLang());
  const blocks = [];
  const add = (name, keys, color, icon, global) => {
    const get = (u) => ({ kd: keys[hex(u)] || (!global ? state.cfg.global[hex(u)] : undefined), inherited: !keys[hex(u)] && !global && !!state.cfg.global[hex(u)] });
    const r = rows(keys, L);
    blocks.push(h("section", { class: "sheet-block" }, h("h2", {}, ic(icon || "layers", 17), name),
      h("div", { class: "mini", style: { display: "flex", marginBottom: "12px" } }, keyboard(layoutId(), legendLang(), get, { u: 30, labels: true, titles: false, color: color || state.cfg.settings.accent }).el),
      r.length ? h("table", { class: "macro-table" }, h("thead", {}, h("tr", {}, h("th", {}, t("sheet.key")), h("th", {}, t("sheet.gesture")), h("th", {}, t("sheet.does")), h("th", {}, t("sheet.kind")))), h("tbody", {}, r)) : h("p", { class: "faint" }, t("sheet.empty_layer"))));
  };
  state.cfg.layers.forEach((l, i) => add(`${i + 1}. ${l.name}`, l.keys, l.color, l.icon, false));
  if (Object.keys(state.cfg.global).length) add(t("strip.global"), state.cfg.global, state.cfg.settings.accent, "globe", true);
  root.append(h("div", { class: "page", style: { maxWidth: "1180px" } },
    h("header", { class: "row wrap" }, h("div", { class: "grow" }, h("h1", {}, t("sheet.title")), h("p", {}, t("sheet.sub"))),
      h("button", { type: "button", class: "btn no-print", onclick: () => window.print() }, ic("printer", 16), t("sheet.print"))), blocks));
  return {};
}
