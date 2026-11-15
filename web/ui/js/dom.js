import { icon } from "./icons.js";

export function h(tag, props = {}, ...kids) {
  const el = document.createElement(tag);
  for (const [k, v] of Object.entries(props || {})) {
    if (v === undefined || v === null || v === false) continue;
    if (k === "class") el.className = v;
    else if (k === "style" && typeof v === "object") {
      for (const [name, val] of Object.entries(v)) {
        if (val === undefined || val === null || val === "") continue;
        if (name.startsWith("--")) el.style.setProperty(name, val); else el.style[name] = val;
      }
    }
    else if (k === "dataset") Object.assign(el.dataset, v);
    else if (k.startsWith("on") && typeof v === "function") el.addEventListener(k.slice(2), v);
    else if (k in el && k !== "list" && !k.includes("-")) el[k] = v;
    else el.setAttribute(k, v === true ? "" : v);
  }
  append(el, kids);
  return el;
}

function append(el, kids) {
  for (const k of kids.flat(Infinity)) {
    if (k === null || k === undefined || k === false) continue;
    el.append(k instanceof Node ? k : document.createTextNode(String(k)));
  }
}

export const ic = (name, size = 16) => {
  const span = document.createElement("span");
  span.className = "ic";
  span.innerHTML = icon(name, size);
  return span;
};

export function clear(el) { el.replaceChildren(); return el; }

const TEXT_INPUTS = new Set(["text", "search", "url", "number", "password", "email", "tel"]);
export const isTextField = (el) => !!el && (el.isContentEditable || el.tagName === "TEXTAREA" || (el.tagName === "INPUT" && TEXT_INPUTS.has(el.type)));
export const isTyping = (el = document.activeElement) => isTextField(el) || el?.tagName === "SELECT";

export const MOD = /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent) ? "Cmd" : "Ctrl";

export const pageHead = (lead, ...actions) => h("header", { class: "page-head" }, h("p", { class: "lead" }, lead), actions.flat().filter(Boolean).length ? h("div", { class: "actions" }, actions) : null);

export const emptyState = (icon, text, action) => h("div", { class: "empty" }, icon ? ic(icon, 20) : null, h("p", {}, text), action || null);
