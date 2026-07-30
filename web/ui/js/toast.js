import { h, ic } from "./dom.js";

let host;
export function toast(text, kind = "info", ms = 2600) {
  if (!host) { host = h("div", { class: "toasts", role: "status", "aria-live": "polite" }); document.body.append(host); }
  const el = h("div", { class: `toast ${kind === "bad" ? "bad" : ""}` }, kind === "bad" ? ic("alert", 16) : ic("check", 16), text);
  host.append(el);
  setTimeout(() => el.remove(), ms);
}
