import { h, ic, clear } from "./dom.js";
import { norm } from "./text.js";
import { t } from "./i18n.js";
import { state, notify, edit, undo, redo, describe, layoutId } from "./store.js";
import { exportConfig } from "./settings.js";
import { openKeyboardDrawer } from "./kbinfo.js";
import { layout, legendLang } from "./layouts.js";

export function openPalette() {
  if (document.querySelector(".scrim")) return;
  const cmds = [];
  const add = (label, icon, run, hint = "") => cmds.push({ label, icon, run, hint, key: label.toLowerCase() });
  for (const id of ["keys", "gallery", "activity", "sheet", "compat", "diag", "settings"]) add(t("pal.go", { name: t("nav." + id) }), "chevron", () => { location.hash = `#/${id}`; });
  state.cfg.layers.forEach((l, i) => add(t("pal.layer", { name: l.name }), "layers", () => { state.scope = i; state.panel = "key"; location.hash = "#/keys"; notify("cfg", "replace"); }, `${i + 1}`));
  add(t("pal.keyboard"), "keyboard", () => openKeyboardDrawer());
  add(t("tool.undo"), "undo", undo, "Cmd Z"); add(t("tool.redo"), "redo", redo, "Cmd Shift Z");
  add(t("strip.add"), "plus", () => { edit("", (c) => { if (c.layers.length < 9) c.layers.push({ name: t("layer.default_name", { n: c.layers.length + 1 }), color: "", icon: "", auto_apps: [], keys: {} }); }); state.scope = state.cfg.layers.length - 1; location.hash = "#/keys"; notify("cfg", "replace"); });
  add(t("pal.theme"), "sun", () => edit("", (c) => { c.settings.theme = c.settings.theme === "dark" ? "light" : "dark"; }));
  add(t("io.export"), "download", exportConfig);
  const L = layout(layoutId(), legendLang());
  const keyName = (id) => { const k = L.keys.find((x) => x.usage === parseInt(id, 16)); return k ? (k.legend.icon ? t("key.arrow") : k.legend.main) : id; };
  const addKeys = (keys, scope, where) => {
    for (const [id, kd] of Object.entries(keys)) {
      const a = kd.tap || kd.hold || kd.double;
      if (!a) continue;
      add(t("pal.key", { key: keyName(id), what: kd.label || describe(a) || t("type." + a.type), where }), "keyboard", () => { state.scope = scope; state.selKey = id.toUpperCase(); state.panel = "key"; location.hash = "#/keys"; notify("cfg", "replace"); });
    }
  };
  state.cfg.layers.forEach((l, i) => addKeys(l.keys, i, l.name));
  addKeys(state.cfg.global, "global", t("strip.global"));

  const input = h("input", { type: "search", placeholder: t("pal.placeholder"), "aria-label": t("pal.placeholder"), autocomplete: "off" });
  const list = h("ul", { role: "listbox" });
  const scrim = h("div", { class: "scrim", onclick: (e) => { if (e.target === scrim) close(); } }, h("div", { class: "palette", role: "dialog", "aria-modal": "true" }, input, list));
  let shown = [], sel = 0;
  const close = () => { scrim.remove(); document.removeEventListener("keydown", onKey, true); };
  const run = (c) => { close(); c.run(); };
  function draw() {
    const q = input.value.trim().toLowerCase();
    shown = cmds.filter((c) => !q || norm(c.key).includes(norm(q))).slice(0, 40);
    sel = Math.min(sel, Math.max(0, shown.length - 1));
    clear(list);
    if (!shown.length) { list.append(h("div", { class: "none" }, t("pal.none"))); return; }
    shown.forEach((c, i) => list.append(h("li", { role: "option", "aria-selected": String(i === sel), onclick: () => run(c), onmousemove: () => { if (sel !== i) { sel = i; draw(); } } }, ic(c.icon, 17), c.label, c.hint && h("small", {}, c.hint))));
  }
  function onKey(e) {
    if (e.key === "Escape") { e.preventDefault(); close(); }
    else if (e.key === "ArrowDown") { e.preventDefault(); sel = Math.min(sel + 1, shown.length - 1); draw(); list.children[sel]?.scrollIntoView({ block: "nearest" }); }
    else if (e.key === "ArrowUp") { e.preventDefault(); sel = Math.max(sel - 1, 0); draw(); list.children[sel]?.scrollIntoView({ block: "nearest" }); }
    else if (e.key === "Enter" && shown[sel]) { e.preventDefault(); run(shown[sel]); }
  }
  input.addEventListener("input", () => { sel = 0; draw(); });
  document.addEventListener("keydown", onKey, true);
  document.body.append(scrim); draw(); input.focus();
}
