import { h, ic } from "./dom.js";
import { api } from "./api.js";
import { t, tOS } from "./i18n.js";
import { toast } from "./toast.js";
import { state, notify } from "./store.js";

const MISSING_AFTER_MS = 8000;
const bootAt = Date.now();
let missing = false;

export const osText = (key, vars) => tOS(key, state.system?.os, vars);

export const isSoftware = () => state.status.capture === "software";

export function boardMissing() {
  const st = state.status;
  return st.capture === "board" && !st.connected && Date.now() - bootAt > MISSING_AFTER_MS;
}

export function watchBoard() {
  setInterval(() => {
    const now = boardMissing();
    if (now !== missing) { missing = now; notify("status"); }
  }, 2000);
}

async function applyMode(mode, consent) {
  const r = await api("/api/capture", { method: "POST", body: { mode, consent } });
  state.cfg.settings.capture = r.capture;
  const [st, kb] = await Promise.all([api("/api/status"), api("/api/keyboard")]);
  Object.assign(state.status, st);
  state.keyboard = { connected: kb.connected, info: kb.info || null, prefs: kb.prefs || null, identity: kb.identity || null, layout: kb.layout || null };
  notify("status"); notify("keyboard");
}

export async function useBoard() {
  try { await applyMode("board", false); toast(t("capture.off_toast")); return true; } catch (e) { toast(e.message, "bad"); return false; }
}

export function useWithoutBoard() {
  return new Promise((resolve) => {
    if (document.querySelector(".capture-consent")) { resolve(false); return; }
    const finish = (ok) => { scrim.remove(); document.removeEventListener("keydown", onKey, true); resolve(ok); };
    const enable = async () => {
      try { await applyMode("software", true); toast(t("capture.on_toast")); finish(true); } catch (e) { toast(t("capture.failed", { error: e.message }), "bad", 5000); }
    };
    const points = ["capture.c1", "capture.c2", "capture.c3", "capture.c4", "capture.c5"].map((k) => h("li", {}, osText(k)));
    const scrim = h("div", { class: "scrim capture-consent", onclick: (e) => { if (e.target === scrim) finish(false); } },
      h("div", { class: "palette consent", role: "dialog", "aria-modal": "true", "aria-label": t("capture.consent_title") },
        h("h2", {}, t("capture.consent_title")), h("p", { class: "dim" }, osText("capture.consent_lead")), h("ul", {}, points),
        h("div", { class: "row", style: { justifyContent: "flex-end", gap: "10px" } },
          h("button", { type: "button", class: "btn", onclick: () => finish(false) }, t("common.cancel")),
          h("button", { type: "button", class: "btn primary", onclick: enable }, t("capture.enable")))));
    const onKey = (e) => { if (e.key === "Escape") { e.preventDefault(); finish(false); } };
    document.addEventListener("keydown", onKey, true);
    document.body.append(scrim);
  });
}

export function offerCard() {
  return h("div", { class: "banner" }, ic("alert", 18), h("p", {}, t("capture.missing")),
    h("button", { type: "button", class: "btn sm primary", onclick: useWithoutBoard }, t("capture.use_without")));
}

export function hookDownCard() {
  return h("div", { class: "banner" }, ic("alert", 18), h("p", {}, osText("capture.hook_down")),
    h("button", { type: "button", class: "btn sm", onclick: useBoard }, t("capture.use_board")));
}
