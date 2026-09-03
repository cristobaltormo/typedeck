import { h, ic, clear } from "./dom.js";
import { api } from "./api.js";
import { t, lang } from "./i18n.js";
import { keyboard, hex } from "./keyboard.js";
import { legendLang } from "./layouts.js";
import { toast } from "./toast.js";
import { state, edit, layerCount, layoutId } from "./store.js";

const pick = (v) => (v && typeof v === "object" ? v[lang()] || v.es : v);
const clone = (o) => JSON.parse(JSON.stringify(o));
const norm = (s) => (s || "").toLowerCase().normalize("NFD").replace(/[̀-ͯ]/g, "");

export function galleryView(root) {
  const { categories, packs } = state.packs;
  let cat = "all", query = "";
  const chips = h("div", { class: "cats", role: "tablist" });
  const search = h("input", { type: "search", placeholder: t("gallery.search"), "aria-label": t("gallery.search"), spellcheck: false });
  const list = h("div", { class: "rowlist" });

  const matches = (p) => (cat === "all" || p.category === cat) && (!query || norm([pick(p.name), pick(p.description), ...(p.apps || []), ...(p.tags || [])].join(" ")).includes(norm(query)));

  function drawChips() {
    clear(chips);
    const mk = (id, label, n, icon) => chips.append(h("button", { type: "button", role: "tab", class: "cat", "aria-selected": String(cat === id), onclick: () => { cat = id; drawChips(); drawList(); } }, icon ? ic(icon, 15) : null, label, h("span", { class: "n" }, String(n))));
    mk("all", t("gallery.all"), packs.length);
    for (const c of categories) { const n = packs.filter((p) => p.category === c.id).length; if (n) mk(c.id, pick(c.name), n, c.icon); }
  }

  async function resolved(p) {
    return api("/api/packs/resolve", { method: "POST", body: { id: p.id, layout: layoutId(), lang: lang() } });
  }

  function preview(p, r) {
    const get = (u) => ({ kd: r.layer.keys[hex(u)] || r.global[hex(u)] });
    const known = state.layouts[layoutId()];
    return h("div", { class: "mini" }, keyboard(layoutId(), legendLang(), get, { u: known && known.width > 12 ? 11 : 22, labels: false, titles: false, color: p.color }).el);
  }

  function drawList() {
    clear(list);
    const shown = packs.filter(matches);
    if (!shown.length) { list.append(h("div", { class: "empty-state" }, h("p", {}, t("gallery.none")))); return; }
    for (const p of shown) {
      const row = h("div", { class: "pack" });
      const prev = h("div", { class: "preview" });
      const apps = (p.apps || []).map((a) => h("span", { class: "tag" }, a));
      const notes = pick(p.notes);
      row.append(prev, h("div", {}, h("h2", { class: "row" }, ic(p.icon, 17), pick(p.name)), h("p", {}, pick(p.description)),
        h("div", { class: "row wrap", style: { gap: "6px", marginTop: "8px" } }, apps, h("span", { class: "faint", style: { fontSize: "12.5px" } }, t("gallery.macros", { n: p.macros.length }))),
        notes ? h("details", { class: "notes" }, h("summary", {}, t("gallery.how")), h("p", {}, notes)) : null),
        h("div", { class: "row" },
          h("button", { type: "button", class: "btn primary", disabled: layerCount() >= 9, onclick: async () => add(p, "layer") }, ic("plus", 15), t("gallery.add")),
          h("button", { type: "button", class: "btn", onclick: async () => add(p, "merge") }, t("gallery.merge"))));
      list.append(row);
      resolved(p).then((r) => { clear(prev).append(preview(p, r)); }).catch(() => {});
    }
  }

  async function add(p, mode) {
    let r;
    try { r = await resolved(p); } catch (e) { return toast(e.message, "bad"); }
    if (mode === "merge" && state.scope === "global") return toast(t("gallery.pick_layer"), "bad");
    edit("", (cfg) => {
      if (mode === "layer") cfg.layers.push(clone(r.layer)); else Object.assign(cfg.layers[state.scope].keys, clone(r.layer.keys));
      for (const [k, kd] of Object.entries(r.global || {})) if (!cfg.global[k]) cfg.global[k] = clone(kd);
    });
    if (mode === "layer") { state.scope = layerCount() - 1; state.panel = "key"; }
    toast(mode === "layer" ? t("gallery.added", { name: r.layer.name }) : t("gallery.merged"));
    if (r.dropped) toast(t("gallery.dropped", { n: r.dropped }), "bad", 4200);
    location.hash = "#/keys";
  }

  search.addEventListener("input", () => { query = search.value; drawList(); });
  root.append(h("div", { class: "page", style: { maxWidth: "1100px" } }, h("header", {}, h("h1", {}, t("gallery.title")), h("p", {}, t("gallery.sub"))),
    h("div", { class: "gallery-tools" }, h("div", { class: "searchbox" }, ic("search", 16), search), chips), list));
  drawChips(); drawList();
  return {};
}
