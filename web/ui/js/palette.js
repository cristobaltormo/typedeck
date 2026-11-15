import { h, ic, clear, MOD } from "./dom.js";
import { norm, usageOf } from "./util.js";
import { t } from "./i18n.js";
import { state, notify, edit, undo, redo, describe, firstAction, addLayer, canAddLayer } from "./store.js";
import { exportConfig } from "./settings.js";
import { openKeyboardDrawer } from "./kbinfo.js";
import { keyName } from "./layouts.js";
import { openModal, modalOpen } from "./modal.js";

const VIEWS = ["keys", "gallery", "activity", "history", "sheet", "compat", "diag", "settings"];

function commands() {
  const cmds = [];
  const add = (label, icon, run, hint = "") => cmds.push({ label, icon, run, hint, key: norm(label) });
  const goKeys = () => { location.hash = "#/keys"; notify("cfg", "replace"); };
  for (const id of VIEWS) add(t("pal.go", { name: t("nav." + id) }), "chevron", () => { location.hash = `#/${id}`; });
  state.cfg.layers.forEach((l, i) => add(t("pal.layer", { name: l.name }), "layers", () => { state.scope = i; state.panel = "key"; goKeys(); }, `${i + 1}`));
  add(t("pal.keyboard"), "keyboard", () => openKeyboardDrawer());
  add(t("tool.undo"), "undo", undo, `${MOD} Z`);
  add(t("tool.redo"), "redo", redo, `${MOD} Shift Z`);
  if (canAddLayer()) add(t("pal.add_layer"), "plus", () => { addLayer(); state.panel = "layer"; goKeys(); });
  add(t("pal.theme"), "sun", () => edit("", (c) => {
    const dark = c.settings.theme === "dark" || (c.settings.theme !== "light" && matchMedia("(prefers-color-scheme: dark)").matches);
    c.settings.theme = dark ? "light" : "dark";
  }));
  add(t("io.export"), "download", exportConfig);
  const addKeys = (keys, scope, where) => {
    for (const [id, kd] of Object.entries(keys)) {
      const a = firstAction(kd);
      if (!a) continue;
      add(t("pal.key", { key: keyName(usageOf(id)), what: kd.label || describe(a) || t("type." + a.type), where }), "keyboard", () => { state.scope = scope; state.selKey = id.toUpperCase(); state.panel = "key"; goKeys(); });
    }
  };
  state.cfg.layers.forEach((l, i) => addKeys(l.keys, i, l.name));
  addKeys(state.cfg.global || {}, "global", t("strip.global"));
  return cmds;
}

export function openPalette() {
  if (modalOpen()) return;
  const cmds = commands();
  const input = h("input", { type: "text", role: "combobox", placeholder: t("pal.placeholder"), "aria-label": t("pal.placeholder"), "aria-expanded": "true", "aria-controls": "pal-list", autocomplete: "off", spellcheck: false });
  const list = h("ul", { id: "pal-list", role: "listbox", "aria-label": t("tool.palette") });
  const panel = h("div", { class: "palette" }, h("div", { class: "palette-input" }, ic("search", 16), input), list);
  let shown = [], sel = 0;
  const modal = openModal(panel, { label: t("tool.palette"), focus: input });
  const run = (c) => { modal.close(); c.run(); };

  function mark() {
    [...list.children].forEach((li, i) => li.setAttribute("aria-selected", String(i === sel)));
    const cur = list.children[sel];
    if (cur?.id) { input.setAttribute("aria-activedescendant", cur.id); cur.scrollIntoView({ block: "nearest" }); } else input.removeAttribute("aria-activedescendant");
  }
  function draw() {
    const q = norm(input.value.trim());
    shown = cmds.filter((c) => !q || c.key.includes(q)).slice(0, 50);
    sel = Math.min(sel, Math.max(0, shown.length - 1));
    clear(list);
    if (!shown.length) { list.append(h("li", { class: "none", role: "presentation" }, t("pal.none"))); input.removeAttribute("aria-activedescendant"); return; }
    shown.forEach((c, i) => list.append(h("li", { id: `pal-${i}`, role: "option", onclick: () => run(c), onpointermove: () => { if (sel !== i) { sel = i; mark(); } } },
      ic(c.icon, 16), h("span", { class: "grow" }, c.label), c.hint ? h("kbd", {}, c.hint) : null)));
    mark();
  }
  input.addEventListener("keydown", (e) => {
    if (e.key === "ArrowDown") { e.preventDefault(); sel = Math.min(sel + 1, shown.length - 1); mark(); }
    else if (e.key === "ArrowUp") { e.preventDefault(); sel = Math.max(sel - 1, 0); mark(); }
    else if (e.key === "Enter" && shown[sel]) { e.preventDefault(); run(shown[sel]); }
  });
  input.addEventListener("input", () => { sel = 0; draw(); });
  draw();
}
