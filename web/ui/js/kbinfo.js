import { h, ic, clear } from "./dom.js";
import { api } from "./api.js";
import { t } from "./i18n.js";
import { toast } from "./toast.js";
import { select } from "./controls.js";
import { keyboard, hex } from "./keyboard.js";
import { layoutList, legendLang } from "./layouts.js";
import { state, subscribe, layoutId, setLayout } from "./store.js";

const REPORT_KIND = { keyboard: "kind.keyboard", nkro: "kind.nkro", consumer: "kind.consumer", system: "kind.system", mouse: "kind.mouse", vendor: "kind.vendor", other: "kind.other" };
const FORWARDED = new Set(["keyboard", "consumer"]);

export const layoutName = (l) => (l.percent ? `${l.percent}% ${l.standard ? l.standard.toUpperCase() : ""}`.trim() : t("layout.numpad"));
export const layoutLabel = (id) => { const l = state.layouts[id]; return l ? layoutName(l) : id; };

export function openKeyboardDrawer() {
  if (document.querySelector(".drawer-scrim")) return;
  const body = h("div", { class: "body" });
  const drawer = h("div", { class: "drawer", role: "dialog", "aria-modal": "true", "aria-label": t("kb.title") },
    h("header", {}, h("h2", { class: "grow" }, t("kb.title")), h("button", { type: "button", class: "btn sm icon ghost", "aria-label": t("common.close"), onclick: close }, ic("x", 16))), body);
  const scrim = h("div", { class: "drawer-scrim", onclick: (e) => { if (e.target === scrim) close(); } }, drawer);
  let learning = false, wizardHost, off, learnData = { seen: {}, suggestions: [], ambiguous: false, hint_keys: [] };

  async function refresh() {
    try {
      const k = await api("/api/keyboard");
      Object.assign(state.keyboard, { connected: k.connected, info: k.info || null, prefs: k.prefs || null, identity: k.identity || null, layout: k.layout || null });
    } catch {  }
    draw();
  }

  const kv = (pairs) => { const dl = h("dl", { class: "kv" }); for (const [k, v] of pairs) if (v !== undefined && v !== "" && v !== null) dl.append(h("dt", {}, k), h("dd", {}, v)); return dl; };

  function identityBlock(info, id) {
    const conf = { verified: t("kb.conf_verified"), name: t("kb.conf_name"), vendor: t("kb.conf_vendor"), unknown: t("kb.conf_unknown") }[id?.confidence || "unknown"];
    const notes = (id?.notes || []).map((n) => h("p", { class: "help" }, t("kb.note_" + n)));
    return h("section", { class: "stack", style: { gap: "10px" } }, h("h3", {}, t("kb.identity")),
      h("div", {}, h("div", { class: "strong", style: { fontSize: "18px", fontWeight: 600 } }, id?.display || t("info.unnamed")), h("p", { class: "dim", style: { fontSize: "13px" } }, conf)),
      kv([[t("kb.mfr"), info.mfr], [t("kb.prod"), info.prod], [t("kb.vendor"), id?.vendor && (id.vendor_is_chip ? `${id.vendor} (${t("kb.chip")})` : id.vendor)], [t("kb.vidpid"), `${info.vid}:${info.pid}`],
        [t("kb.version"), info.bcd && `${info.bcd.slice(0, 2)}.${info.bcd.slice(2)}`], [t("kb.usb"), info.usb && `USB ${parseInt(info.usb.slice(0, 2), 16)}.${(parseInt(info.usb.slice(2), 16) >> 4)}`],
        [t("kb.power"), info.power_ma && `${info.power_ma} mA`], [t("kb.ifaces"), info.ifaces], [t("kb.serial"), info.serial]]), ...notes);
  }

  function reports(info) {
    const list = (info.reports || []).filter((x) => x.dir === "input");
    if (!list.length) return h("p", { class: "dim" }, t("kb.no_reports"));
    return h("div", {}, list.map((r) => {
      const fw = FORWARDED.has(r.kind);
      return h("div", { class: "report" }, h("span", { class: "id" }, r.id ? `ID ${r.id}` : t("kb.report_main")),
        h("span", { class: "grow" }, t(REPORT_KIND[r.kind] || "kind.other"), r.detail ? h("span", { class: "faint" }, ` - ${r.detail.replace(/^interface \d+: /, "")}`) : null),
        h("span", { class: fw ? "ok" : "faint" }, fw ? t("kb.forwarded") : t("kb.not_forwarded")));
    }));
  }

  function layoutBlock() {
    const ch = state.keyboard.layout;
    const current = layoutId();
    const sourceText = ch ? t("kb.src_" + ch.source) : "";
    return h("section", { class: "stack", style: { gap: "12px" } }, h("h3", {}, t("kb.layout")),
      h("p", {}, h("b", { class: "strong" }, layoutLabel(current)), h("span", { class: "dim" }, `  ${sourceText}`), ch && !ch.confident ? h("span", { class: "faint" }, ` - ${t("kb.unconfirmed")}`) : null),
      select("", current, layoutList().map((l) => [l.id, layoutName(l)]), async (v) => { try { await setLayout(v); draw(); } catch (e) { toast(e.message, "bad"); } }, t("kb.layout_help")),
      h("div", { class: "mini", style: { display: "flex", justifyContent: "center" } }, keyboard(current, legendLang(), () => ({}), { u: 18, labels: false, titles: false }).el));
  }

  function draw() {
    clear(body);
    const info = state.keyboard.info, st = state.status;
    if (!st.connected) { body.append(h("p", { class: "dim" }, t(st.board_error === "outdated_firmware" ? "kb.fw_old" : "kb.no_board"))); return; }
    if (!info?.present) { body.append(h("p", { class: "dim" }, t("kb.no_keyboard"))); return; }
    body.append(identityBlock(info, state.keyboard.identity), layoutBlock(),
      h("section", { class: "stack", style: { gap: "6px" } }, h("h3", {}, t("kb.reports")), h("p", { class: "help", style: { marginTop: 0 } }, t("kb.reports_help")), reports(info)),
      h("section", { class: "stack", style: { gap: "10px" } }, h("h3", {}, t("kb.detect")), h("p", { class: "dim", style: { fontSize: "13px" } }, t("kb.detect_help")), (wizardHost = h("div"))));
    drawWizard();
  }

  function drawWizard() {
    clear(wizardHost);
    const seen = new Set(Object.keys(learnData.seen).map((k) => parseInt(k, 16)));
    const sugs = learnData.suggestions || [];
    const best = sugs[0];
    wizardHost.append(h("div", { class: "row wrap" },
      h("button", { type: "button", class: `btn ${learning ? "" : "primary"}`, onclick: toggleLearn }, ic(learning ? "check" : "play", 15), learning ? t("kb.detect_stop") : t("kb.detect_start")),
      seen.size ? h("span", { class: "dim" }, t("kb.detect_seen", { n: seen.size })) : null));
    if (learning || seen.size) {
      const target = best?.id || layoutId();
      wizardHost.append(h("div", { class: "progress", style: { margin: "12px 0" } }, h("i", { style: { width: best ? Math.min(100, best.recall * 100) + "%" : "0%" } })),
        h("div", { class: "mini", style: { display: "flex", justifyContent: "center" } }, keyboard(target, legendLang(), () => ({}), { u: 18, labels: false, titles: false, heat: (u) => (seen.has(u) ? 1 : undefined), count: () => "" }).el));
    }
    if (learning && learnData.ambiguous) {
      const keys = (learnData.hint_keys || []).map((u) => ({ 0x64: "< >", 0x32: "Ç", 0x31: "\\" })[u] || hex(u)).join(", ");
      wizardHost.append(h("p", { class: "help warn", style: { marginTop: "10px" } }, ic("alert", 15), t("kb.press_these", { keys })));
    }
    if (!learning && best && seen.size >= 8) {
      wizardHost.append(h("div", { class: "stack", style: { gap: "8px", marginTop: "12px" } },
        h("p", {}, t("kb.suggest", { layout: layoutLabel(best.id), pct: Math.round(best.score * 100) })),
        h("div", { class: "row wrap" }, h("button", { type: "button", class: "btn primary", disabled: best.id === layoutId(), onclick: async () => { try { await setLayout(best.id); toast(t("kb.layout_saved")); draw(); } catch (e) { toast(e.message, "bad"); } } }, best.id === layoutId() ? t("kb.already") : t("kb.use_layout")),
          sugs[1] ? h("span", { class: "faint", style: { fontSize: "12.5px" } }, t("kb.alt", { layout: layoutLabel(sugs[1].id) })) : null)));
    }
  }

  async function pull() { try { learnData = await api("/api/learn"); } catch {  } drawWizard(); }

  async function toggleLearn() {
    try {
      if (!learning) { await api("/api/learn", { method: "POST", body: { on: true } }); learning = true; learnData = { seen: {}, suggestions: [] }; }
      else { await api("/api/learn", { method: "POST", body: { on: false } }); learning = false; await pull(); }
    } catch (e) { toast(e.message, "bad"); }
    drawWizard();
  }

  let pullTimer;
  off = subscribe((topic) => {
    if (topic === "watch" && learning) { clearTimeout(pullTimer); pullTimer = setTimeout(pull, 250); }
    if (topic === "keyboard" || topic === "status") refresh();
  });

  function close() {
    off?.(); clearTimeout(pullTimer);
    if (learning) api("/api/learn", { method: "POST", body: { on: false } }).catch(() => {});
    scrim.remove();
    document.removeEventListener("keydown", onKey, true);
  }
  const onKey = (e) => { if (e.key === "Escape") { e.preventDefault(); close(); } };
  document.addEventListener("keydown", onKey, true);
  document.body.append(scrim);
  draw();
  refresh();
}
