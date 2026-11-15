import { h } from "./dom.js";

const FOCUSABLE = 'a[href], button:not([disabled]), input:not([disabled]):not([type="hidden"]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';
const stack = [];

export const modalOpen = () => stack.length > 0;

export function openModal(panel, { label, drawer = false, onClose, focus } = {}) {
  const before = document.activeElement;
  panel.setAttribute("role", "dialog");
  panel.setAttribute("aria-modal", "true");
  if (label) panel.setAttribute("aria-label", label);
  panel.tabIndex = -1;
  const scrim = h("div", { class: drawer ? "scrim drawer-scrim" : "scrim" }, panel);
  let downOnScrim = false;
  scrim.addEventListener("pointerdown", (e) => { downOnScrim = e.target === scrim; });
  scrim.addEventListener("click", (e) => { if (downOnScrim && e.target === scrim) close(); });
  let closed = false;
  const self = { el: scrim, panel, close };

  function onKey(e) {
    if (stack.at(-1) !== self) return;
    if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); close(); return; }
    if (e.key !== "Tab") return;
    const items = [...panel.querySelectorAll(FOCUSABLE)].filter((el) => el.offsetParent !== null || el === document.activeElement);
    if (!items.length) { e.preventDefault(); panel.focus(); return; }
    const first = items[0], last = items.at(-1);
    if (e.shiftKey && (document.activeElement === first || !panel.contains(document.activeElement))) { e.preventDefault(); last.focus(); }
    else if (!e.shiftKey && (document.activeElement === last || !panel.contains(document.activeElement))) { e.preventDefault(); first.focus(); }
  }

  function close() {
    if (closed) return;
    closed = true;
    stack.splice(stack.indexOf(self), 1);
    document.removeEventListener("keydown", onKey, true);
    scrim.remove();
    onClose?.();
    if (before?.isConnected && !modalOpen()) before.focus({ preventScroll: true });
  }

  document.addEventListener("keydown", onKey, true);
  document.body.append(scrim);
  stack.push(self);
  (focus || panel.querySelector("[autofocus]") || panel).focus({ preventScroll: true });
  return self;
}
