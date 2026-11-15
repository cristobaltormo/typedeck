import { h, ic } from "./dom.js";
import { api } from "./api.js";
import { t, tOS } from "./i18n.js";
import { toast } from "./toast.js";
import { openModal } from "./modal.js";
import { state, notify, setKeyboard } from "./store.js";

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
  setKeyboard(kb);
  notify("status"); notify("keyboard");
}

export async function useBoard() {
  try { await applyMode("board", false); toast(t("capture.off_toast")); return true; } catch (e) { toast(e.message, "bad"); return false; }
}

let consentOpen = false;
export function useWithoutBoard() {
  if (consentOpen) return Promise.resolve(false);
  consentOpen = true;
  return new Promise((resolve) => {
    let ok = false;
    const enable = async (e) => {
      const btn = e.currentTarget;
      btn.disabled = true;
      try { await applyMode("software", true); toast(t("capture.on_toast")); ok = true; modal.close(); }
      catch (err) { toast(t("capture.failed", { error: err.message }), "bad", 5000); btn.disabled = false; }
    };
    const panel = h("div", { class: "dialog consent" },
      h("header", { class: "dialog-head" }, h("h2", {}, t("capture.consent_title"))),
      h("p", {}, osText("capture.consent_lead")),
      h("ul", { class: "consent-list" }, ["capture.c1", "capture.c2", "capture.c3", "capture.c4", "capture.c5"].map((k) => h("li", {}, osText(k)))),
      h("footer", { class: "dialog-foot" },
        h("button", { type: "button", class: "btn", onclick: () => modal.close() }, t("common.cancel")),
        h("button", { type: "button", class: "btn primary", onclick: enable }, t("capture.enable"))));
    const modal = openModal(panel, { label: t("capture.consent_title"), onClose: () => { consentOpen = false; resolve(ok); } });
  });
}

export function offerCard() {
  return h("div", { class: "banner", role: "status" }, ic("alert", 16), h("p", {}, t("capture.missing")),
    h("button", { type: "button", class: "btn sm primary", onclick: useWithoutBoard }, t("capture.use_without")));
}

export function hookDownCard() {
  return h("div", { class: "banner", role: "status" }, ic("alert", 16), h("p", {}, osText("capture.hook_down")),
    h("button", { type: "button", class: "btn sm", onclick: useBoard }, t("capture.use_board")));
}
