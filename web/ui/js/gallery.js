import { h, ic, clear, pageHead, emptyState } from "./dom.js";
import { api } from "./api.js";
import { t, lang, pick } from "./i18n.js";
import { keyboard } from "./keyboard.js";
import { legendLang, keyName } from "./layouts.js";
import { toast } from "./toast.js";
import { openModal } from "./modal.js";
import { clone, hex, norm } from "./util.js";
import { state, edit, layoutId, describe, firstAction, canAddLayer } from "./store.js";
import { insertLayer } from "./layerops.js";
import { importPack, exportEverything } from "./share.js";

const cache = new Map();
function resolved(p) {
  const key = `${p.id}|${layoutId()}|${lang()}`;
  if (!cache.has(key)) cache.set(key, api("/api/packs/resolve", { method: "POST", body: { id: p.id, layout: layoutId(), lang: lang() } }).catch((e) => { cache.delete(key); throw e; }));
  return cache.get(key);
}

const macroList = (r) => Object.entries({ ...r.global, ...r.layer.keys }).map(([id, kd]) => ({ u: parseInt(id, 16), kd })).filter((m) => firstAction(m.kd));
const what = (kd) => kd.label || describe(firstAction(kd));

export function galleryView(root) {
  const { categories, packs } = state.packs;
  let cat = "all", query = "";
  const chips = h("div", { class: "cats", role: "group", "aria-label": t("gallery.categories") });
  const search = h("input", { type: "search", placeholder: t("gallery.search"), "aria-label": t("gallery.search"), spellcheck: false });
  const list = h("div", { class: "packs" });

  const matches = (p) => (cat === "all" || p.category === cat) && (!query || norm([pick(p.name), pick(p.description), ...(p.apps || []), ...(p.tags || [])].join(" ")).includes(norm(query)));

  function drawChips() {
    clear(chips);
    const mk = (id, label, n) => chips.append(h("button", { type: "button", class: "cat", "aria-pressed": String(cat === id), onclick: () => { cat = id; drawChips(); drawList(); } }, label, h("span", { class: "n" }, String(n))));
    mk("all", t("gallery.all"), packs.length);
    for (const c of categories) { const n = packs.filter((p) => p.category === c.id).length; if (n) mk(c.id, pick(c.name), n); }
  }

  function preview(p, r) {
    const get = (u) => ({ kd: r.layer.keys[hex(u)] || r.global[hex(u)] });
    const kbd = keyboard(layoutId(), legendLang(), get, { crop: true, width: 340, maxU: 40, labels: false, titles: false, color: p.color });
    const macros = macroList(r);
    const shown = macros.slice(0, 6).map((m) => h("li", { class: "chip" }, h("kbd", {}, keyName(m.u)), h("span", { class: "chip-text" }, what(m.kd))));
    return h("div", { class: "previewbox" },
      h("button", { type: "button", class: "mini zoomable", "aria-label": `${t("gallery.enlarge")}: ${pick(p.name)}`, onclick: () => enlarge(p, r) }, kbd.el),
      h("ul", { class: "chips" }, shown, macros.length > shown.length ? h("li", { class: "chip more" }, `+${macros.length - shown.length}`) : null));
  }

  function enlarge(p, r) {
    const get = (u) => ({ kd: r.layer.keys[hex(u)] || r.global[hex(u)] });
    const rows = macroList(r).map((m) => h("tr", {}, h("td", {}, h("kbd", {}, keyName(m.u))), h("td", {}, what(m.kd)), h("td", { class: "dim" }, t("type." + firstAction(m.kd).type))));
    const panel = h("div", { class: "dialog wide" },
      h("header", { class: "dialog-head" }, ic(p.icon, 18), h("h2", { class: "grow" }, pick(p.name)), h("button", { type: "button", class: "tool", "aria-label": t("common.close"), onclick: () => modal.close() }, ic("x", 16))),
      h("div", { class: "mini big" }, keyboard(layoutId(), legendLang(), get, { crop: true, width: 760, maxU: 64, labels: true, titles: false, color: p.color }).el),
      h("table", { class: "macro-table" }, h("tbody", {}, rows)));
    const modal = openModal(panel, { label: pick(p.name) });
  }

  function drawList() {
    clear(list);
    const shown = packs.filter(matches);
    if (!shown.length) { list.append(emptyState("search", t("gallery.none"))); return; }
    for (const p of shown) {
      const prev = h("div", { class: "preview" }, h("div", { class: "preview-wait" }));
      const notes = pick(p.notes);
      list.append(h("article", { class: "pack" }, prev,
        h("div", { class: "pack-body" },
          h("h2", {}, ic(p.icon, 16), pick(p.name)),
          h("p", { class: "dim" }, pick(p.description)),
          h("div", { class: "meta" }, (p.apps || []).map((a) => h("span", { class: "tag" }, a)), h("span", { class: "faint" }, t("gallery.macros", { n: p.macros.length }))),
          notes ? h("details", { class: "notes" }, h("summary", {}, t("gallery.how")), h("p", {}, notes)) : null),
        h("div", { class: "pack-actions" },
          h("button", { type: "button", class: "btn primary", disabled: !canAddLayer(), title: canAddLayer() ? undefined : t("gallery.full"), onclick: () => add(p, "layer") }, ic("plus", 14), t("gallery.add")),
          h("button", { type: "button", class: "btn", disabled: state.scope === "global", onclick: () => add(p, "merge") }, t("gallery.merge", { name: state.scope === "global" ? "-" : state.cfg.layers[state.scope].name })))));
      resolved(p).then((r) => { if (prev.isConnected) clear(prev).append(preview(p, r)); }).catch(() => { if (prev.isConnected) clear(prev); });
    }
  }

  async function add(p, mode) {
    if (mode === "merge" && state.scope === "global") return toast(t("gallery.pick_layer"), "bad");
    let r;
    try { r = await resolved(p); } catch (e) { return toast(e.message, "bad"); }
    if (mode === "layer" && !canAddLayer()) return toast(t("gallery.full"), "bad");
    edit("", (cfg) => {
      if (mode === "layer") insertLayer(cfg, cfg.layers.length, clone(r.layer));
      else Object.assign(cfg.layers[state.scope].keys, clone(r.layer.keys));
      for (const [k, kd] of Object.entries(r.global || {})) if (!cfg.global[k]) cfg.global[k] = clone(kd);
    });
    if (mode === "layer") state.scope = state.cfg.layers.length - 1;
    state.panel = "key";
    toast(mode === "layer" ? t("gallery.added", { name: r.layer.name }) : t("gallery.merged", { name: state.cfg.layers[state.scope].name }));
    if (r.dropped) toast(t("gallery.dropped", { n: r.dropped }), "bad", 4200);
    location.hash = "#/keys";
  }

  search.addEventListener("input", () => { query = search.value; drawList(); });
  root.append(h("div", { class: "page wide" },
    pageHead(t("gallery.sub"),
      h("button", { type: "button", class: "btn", onclick: importPack }, ic("upload", 14), t("share.import_btn")),
      h("button", { type: "button", class: "btn", onclick: exportEverything }, ic("download", 14), t("share.export_all"))),
    h("div", { class: "gallery-tools" }, h("label", { class: "searchbox" }, ic("search", 16), search), chips), list));
  drawChips(); drawList();
  return {};
}
