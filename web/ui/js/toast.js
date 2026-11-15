import { h, ic } from "./dom.js";

let host;
export function toast(text, kind = "info", ms = 2600) {
  if (!host?.isConnected) { host = h("div", { class: "toasts", role: "status", "aria-live": "polite" }); document.body.append(host); }
  const bad = kind === "bad";
  const el = h("div", { class: `toast ${bad ? "bad" : ""}` }, ic(bad ? "alert" : "check", 16), h("span", {}, text));
  host.append(el);
  while (host.children.length > 3) host.firstElementChild.remove();
  setTimeout(() => el.remove(), bad ? Math.max(ms, 4000) : ms);
}

export const guarded = (fn) => async (...args) => {
  try { return await fn(...args); } catch (e) { toast(e.message, "bad"); }
};
