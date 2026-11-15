import { h, ic } from "./dom.js";
import { ICON_NAMES } from "./icons.js";
import { t } from "./i18n.js";

let uid = 0;
const nid = (p) => `${p}-${++uid}`;
const LABELLABLE = new Set(["INPUT", "SELECT", "TEXTAREA"]);

export function field(label, control, help, warn) {
  const id = control.id || (control.id = nid("f"));
  const w = h("div", { class: "field" });
  if (label) {
    const lab = h(LABELLABLE.has(control.tagName) ? "label" : "span", { class: "label", id: id + "-l" }, label);
    if (lab.tagName === "LABEL") lab.htmlFor = id; else control.setAttribute("aria-labelledby", lab.id);
    w.append(lab);
  }
  w.append(control);
  if (help) { const p = h("p", { class: "help", id: id + "-h" }, help); control.setAttribute("aria-describedby", p.id); w.append(p); }
  if (warn) w.append(h("p", { class: "help warn" }, ic("alert", 14), warn));
  return w;
}

export function textInput(label, value, onInput, o = {}) {
  const el = h("input", { type: o.type || "text", value: value ?? "", placeholder: o.placeholder || "", maxLength: o.max, spellcheck: false, autocomplete: "off" });
  if (o.list) el.setAttribute("list", o.list);
  if (o.min !== undefined) el.min = o.min;
  if (o.step !== undefined) el.step = o.step;
  if (!label && o.aria) el.setAttribute("aria-label", o.aria);
  el.addEventListener("input", () => onInput(el.value));
  return field(label, el, o.help, o.warn);
}

export function area(label, value, onInput, o = {}) {
  const el = h("textarea", { value: value ?? "", placeholder: o.placeholder || "", spellcheck: false, rows: 3 });
  el.addEventListener("input", () => onInput(el.value));
  return field(label, el, o.help);
}

export function select(label, value, options, onChange, help, aria) {
  const el = h("select", {}, options.map(([v, text]) => h("option", { value: String(v), selected: String(v) === String(value) }, text)));
  if (!label && aria) el.setAttribute("aria-label", aria);
  el.addEventListener("change", () => onChange(el.value));
  return field(label, el, help);
}

export function toggle(label, checked, onChange, sub, aria) {
  const input = h("input", { type: "checkbox", checked: !!checked, role: "switch" });
  if (!label && aria) input.setAttribute("aria-label", aria);
  input.addEventListener("change", () => onChange(input.checked));
  return h("label", { class: "switch" }, label ? h("span", { class: "switch-text" }, label, sub && h("small", {}, sub)) : null, input);
}

export function seg(options, value, onChange, aria) {
  const box = h("div", { class: "seg", role: "group", "aria-label": aria });
  for (const [v, text] of options) {
    const b = h("button", { type: "button", "aria-pressed": String(v === value) }, text);
    b.addEventListener("click", () => {
      if (b.getAttribute("aria-pressed") === "true") return;
      for (const c of box.children) c.setAttribute("aria-pressed", "false");
      b.setAttribute("aria-pressed", "true");
      onChange(v);
    });
    box.append(b);
  }
  return box;
}

export const COLORS = ["#2563eb", "#0ea5e9", "#0d9488", "#16a34a", "#65a30d", "#d97706", "#ea580c", "#dc2626", "#db2777", "#7c3aed", "#64748b"];

export function swatches(value, onChange, { allowNone = true } = {}) {
  const box = h("div", { class: "swatches", role: "group" });
  const draw = (v) => {
    box.replaceChildren();
    if (allowNone) box.append(h("button", { type: "button", class: "swatch none", title: t("color.none"), "aria-label": t("color.none"), "aria-pressed": String(!v), onclick: () => { onChange(null); draw(null); } }, ic("x", 12)));
    for (const c of COLORS) box.append(h("button", { type: "button", class: "swatch", style: { "--c": c }, title: c, "aria-label": c, "aria-pressed": String(v === c), onclick: () => { onChange(c); draw(c); } }));
    const custom = !!v && !COLORS.includes(v);
    const pick = h("input", { type: "color", value: v || "#2563eb", "aria-label": t("color.custom") });
    pick.addEventListener("input", () => onChange(pick.value));
    pick.addEventListener("change", () => draw(pick.value));
    box.append(h("label", { class: "swatch custom", title: t("color.custom"), style: custom ? { "--c": v } : {}, "aria-pressed": String(custom) }, pick));
  };
  draw(value);
  return box;
}

export function iconPicker(value, onChange) {
  const box = h("div", { class: "iconfield" });
  let open = false;
  const draw = (v, focusToggle) => {
    box.replaceChildren();
    const toggleBtn = h("button", { type: "button", class: "iconbtn", "aria-expanded": String(open), onclick: () => { open = !open; draw(v, true); } },
      v ? ic(v, 16) : h("span", { class: "iconnone" }), h("span", { class: "grow" }, v || t("icon.none")), ic(open ? "chevron-down" : "chevron", 14));
    box.append(toggleBtn);
    if (focusToggle) toggleBtn.focus();
    if (!open) return;
    const grid = h("div", { class: "iconpick" });
    for (const n of ICON_NAMES) grid.append(h("button", { type: "button", title: n, "aria-label": n, "aria-pressed": String(v === n), onclick: () => { const next = v === n ? null : n; onChange(next); open = false; draw(next, true); } }, ic(n, 16)));
    box.append(grid);
  };
  draw(value);
  return box;
}

export function armed(labelKey, onConfirm, o = {}) {
  let timer;
  const text = h("span", {}, t(labelKey));
  const b = h("button", { type: "button", class: `btn ${o.small ? "sm" : ""} danger` }, o.icon ? ic(o.icon, 16) : null, text);
  const reset = () => { clearTimeout(timer); delete b.dataset.armed; text.textContent = t(labelKey); };
  b.addEventListener("click", () => {
    if (b.dataset.armed) { reset(); onConfirm(); return; }
    b.dataset.armed = "1"; text.textContent = t("common.confirm_delete");
    timer = setTimeout(reset, 3000);
  });
  b.addEventListener("blur", reset);
  return b;
}

export function appsDatalist(apps) {
  let dl = document.getElementById("apps");
  if (!dl) { dl = h("datalist", { id: "apps" }); document.body.append(dl); }
  dl.replaceChildren(...apps.map((n) => h("option", { value: n })));
}
