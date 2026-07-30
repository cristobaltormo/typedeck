import { h, ic } from "./dom.js";
import { ICON_NAMES } from "./icons.js";
import { t } from "./i18n.js";

let uid = 0;
const nid = (p) => `${p}-${++uid}`;

export function field(label, control, help, warn) {
  const id = nid("f");
  if (control.id === "" || !control.id) control.id = id;
  const w = h("div", { class: "f" }, label && h("label", { class: "f", for: control.id }, label), control);
  if (help) w.append(h("p", { class: "help" }, help));
  if (warn) w.append(h("p", { class: "help warn" }, ic("alert", 15), warn));
  return w;
}

export function textInput(label, value, onInput, o = {}) {
  const el = h("input", { type: o.type || "text", value: value ?? "", placeholder: o.placeholder || "", maxLength: o.max, spellcheck: false, autocomplete: "off" });
  if (o.list) el.setAttribute("list", o.list);
  if (o.min !== undefined) el.min = o.min;
  if (o.step !== undefined) el.step = o.step;
  el.addEventListener("input", () => onInput(el.value));
  return field(label, el, o.help, o.warn);
}

export function area(label, value, onInput, o = {}) {
  const el = h("textarea", { value: value ?? "", placeholder: o.placeholder || "", spellcheck: false });
  el.addEventListener("input", () => onInput(el.value));
  return field(label, el, o.help);
}

export function select(label, value, options, onChange, help) {
  const el = h("select", {}, options.map(([v, text]) => h("option", { value: String(v), selected: String(v) === String(value) }, text)));
  el.addEventListener("change", () => onChange(el.value));
  return field(label, el, help);
}

export function toggle(label, checked, onChange, sub) {
  const id = nid("sw");
  const input = h("input", { type: "checkbox", id, checked: !!checked, role: "switch" });
  input.addEventListener("change", () => onChange(input.checked));
  return h("label", { class: "switch", for: id }, h("span", {}, label, sub && h("small", {}, sub)), input);
}

export function seg(options, value, onChange) {
  const box = h("div", { class: "seg", role: "group" });
  for (const [v, text] of options) {
    const b = h("button", { type: "button", "aria-pressed": String(v === value) }, text);
    b.addEventListener("click", () => { for (const c of box.children) c.setAttribute("aria-pressed", "false"); b.setAttribute("aria-pressed", "true"); onChange(v); });
    box.append(b);
  }
  return box;
}

export const COLORS = ["#2563eb", "#0ea5e9", "#0ea5a4", "#16a34a", "#84cc16", "#f59e0b", "#f97316", "#cc4848", "#ec4899", "#8b5cf6", "#64748b"];

export function swatches(value, onChange, { allowNone = true } = {}) {
  const box = h("div", { class: "swatches" });
  const draw = (v) => {
    box.replaceChildren();
    if (allowNone) box.append(h("button", { type: "button", class: "swatch none", title: t("color.none"), "aria-pressed": String(!v), onclick: () => { onChange(null); draw(null); } }, ic("x", 12)));
    for (const c of COLORS) box.append(h("button", { type: "button", class: "swatch", style: { "--c": c }, title: c, "aria-pressed": String(v === c), onclick: () => { onChange(c); draw(c); } }));
    const custom = h("label", { class: "swatch custom", title: t("color.custom"), "aria-pressed": String(!!v && !COLORS.includes(v)) });
    const pick = h("input", { type: "color", value: v || "#2563eb" });
    pick.addEventListener("input", () => onChange(pick.value));
    pick.addEventListener("change", () => draw(pick.value));
    custom.append(pick);
    box.append(custom);
  };
  draw(value);
  return box;
}

export function iconPicker(value, onChange) {
  const box = h("div", { class: "iconfield" });
  let open = false;
  const draw = (v) => {
    box.replaceChildren();
    const toggleBtn = h("button", { type: "button", class: "iconbtn", "aria-expanded": String(open), onclick: () => { open = !open; draw(v); } },
      v ? ic(v, 18) : h("span", { class: "iconnone" }), h("span", { class: "grow" }, v || t("icon.none")), ic(open ? "chevron-down" : "chevron", 14));
    box.append(toggleBtn);
    if (!open) return;
    const grid = h("div", { class: "iconpick" });
    for (const n of ICON_NAMES) grid.append(h("button", { type: "button", title: n, "aria-label": n, "aria-pressed": String(v === n), onclick: () => { onChange(v === n ? null : n); draw(v === n ? null : n); } }, ic(n, 18)));
    box.append(grid);
  };
  draw(value);
  return box;
}

export function armed(labelKey, onConfirm, o = {}) {
  let timer;
  const b = h("button", { type: "button", class: `btn ${o.small ? "sm" : ""} danger` }, o.icon ? ic(o.icon, 16) : null, h("span", {}, t(labelKey)));
  b.addEventListener("click", () => {
    if (b.dataset.armed) { clearTimeout(timer); onConfirm(); return; }
    b.dataset.armed = "1"; b.lastChild.textContent = t("common.confirm_delete");
    timer = setTimeout(() => { delete b.dataset.armed; b.lastChild.textContent = t(labelKey); }, 3000);
  });
  return b;
}
