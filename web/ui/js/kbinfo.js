import { h, ic, clear } from "./dom.js";
import { api } from "./api.js";
import { t } from "./i18n.js";
import { toast } from "./toast.js";
import { select } from "./controls.js";
import { keyboard } from "./keyboard.js";
import { layoutList, legendLang } from "./layouts.js";
import { openModal, modalOpen } from "./modal.js";
import { hex } from "./util.js";
import { state, subscribe, layoutId, setLayout, setKeyboard } from "./store.js";
import { isSoftware, offerCard, hookDownCard, useBoard, osText } from "./capture.js";

const REPORT_KIND = { keyboard: "kind.keyboard", nkro: "kind.nkro", consumer: "kind.consumer", system: "kind.system", mouse: "kind.mouse", vendor: "kind.vendor", other: "kind.other" };
const FORWARDED = new Set(["keyboard", "consumer"]);
const HINT_KEYS = { 0x64: "< >", 0x32: "Ç", 0x31: "\\" };

export const layoutName = (l) => (l.percent ? `${l.percent}% ${l.standard ? l.standard.toUpperCase() : ""}`.trim() : t("layout.numpad"));
export const layoutLabel = (id) => { const l = state.layouts[id]; return l ? layoutName(l) : id; };

const block = (title, ...kids) => h("section", { class: "drawer-section" }, h("h3", {}, title), ...kids);
const miniKb = (id, opts = {}) => h("div", { class: "mini center" }, keyboard(id, legendLang(), () => ({}), { maxU: 22, minU: 10, labels: false, titles: false, ...opts }).el);

export function openKeyboardDrawer() {
  if (modalOpen()) return;
  const body = h("div", { class: "drawer-body" });
  const panel = h("div", { class: "drawer" },
    h("header", { class: "drawer-head" }, h("h2", { class: "grow" }, t("kb.title")), h("button", { type: "button", class: "tool", "aria-label": t("common.close"), onclick: () => modal.close() }, ic("x", 16))), body);
  let learning = false, wizardHost = h("div"), learnData = { seen: {}, suggestions: [], ambiguous: false, hint_keys: [] }, pullTimer;

  async function refresh() {
    try { setKeyboard(await api("/api/keyboard")); } catch {}
    draw();
  }

  const kv = (pairs) => { const dl = h("dl", { class: "kv" }); for (const [k, v] of pairs) if (v !== undefined && v !== "" && v !== null) dl.append(h("dt", {}, k), h("dd", {}, v)); return dl; };
  const applyLayout = async (id) => { try { await setLayout(id); toast(t("kb.layout_saved")); draw(); } catch (e) { toast(e.message, "bad"); } };

  function identityBlock(info, id) {
    const conf = { verified: t("kb.conf_verified"), name: t("kb.conf_name"), vendor: t("kb.conf_vendor"), unknown: t("kb.conf_unknown") }[id?.confidence || "unknown"];
    return block(t("kb.identity"),
      h("div", {}, h("p", { class: "device-name" }, id?.display || t("info.unnamed")), h("p", { class: "dim small" }, conf)),
      kv([[t("kb.mfr"), info.mfr], [t("kb.prod"), info.prod], [t("kb.vendor"), id?.vendor && (id.vendor_is_chip ? `${id.vendor} (${t("kb.chip")})` : id.vendor)], [t("kb.vidpid"), `${info.vid}:${info.pid}`],
        [t("kb.version"), info.bcd && `${info.bcd.slice(0, 2)}.${info.bcd.slice(2)}`], [t("kb.usb"), info.usb && `USB ${parseInt(info.usb.slice(0, 2), 16)}.${parseInt(info.usb.slice(2), 16) >> 4}`],
        [t("kb.power"), info.power_ma && `${info.power_ma} mA`], [t("kb.ifaces"), info.ifaces], [t("kb.serial"), info.serial]]),
      ...(id?.notes || []).map((n) => h("p", { class: "help" }, t("kb.note_" + n))));
  }

  function reports(info) {
    const list = (info.reports || []).filter((x) => x.dir === "input");
    if (!list.length) return h("p", { class: "dim" }, t("kb.no_reports"));
    return h("ul", { class: "rowlist" }, list.map((r) => {
      const fw = FORWARDED.has(r.kind);
      return h("li", { class: "report" }, h("span", { class: "id" }, r.id ? `ID ${r.id}` : t("kb.report_main")),
        h("span", { class: "grow" }, t(REPORT_KIND[r.kind] || "kind.other"), r.detail ? h("span", { class: "faint" }, ` · ${r.detail.replace(/^interface \d+: /, "")}`) : null),
        h("span", { class: fw ? "ok" : "faint" }, fw ? t("kb.forwarded") : t("kb.not_forwarded")));
    }));
  }

  function layoutBlock() {
    const ch = state.keyboard.layout;
    const current = layoutId();
    return block(t("kb.layout"),
      h("p", {}, h("b", { class: "strong" }, layoutLabel(current)), ch ? h("span", { class: "dim" }, ` · ${t("kb.src_" + ch.source)}`) : null, ch && !ch.confident ? h("span", { class: "warn-text" }, ` · ${t("kb.unconfirmed")}`) : null),
      select("", current, layoutList().map((l) => [l.id, layoutName(l)]), applyLayout, t("kb.layout_help"), t("kb.layout")),
      miniKb(current));
  }

  const detectBlock = () => block(t("kb.detect"), h("p", { class: "help" }, t("kb.detect_help")), (wizardHost = h("div", { class: "stack tight" })));

  function draw() {
    drawnSig = sig();
    clear(body);
    const info = state.keyboard.info, st = state.status;
    if (isSoftware() && !st.connected) { body.append(hookDownCard()); return; }
    if (isSoftware()) {
      body.append(block(t("kb.identity"), h("p", { class: "device-name" }, t("capture.sys_keyboard")), h("p", { class: "help" }, t("capture.kb_help"))), layoutBlock(), detectBlock(),
        block(t("capture.limits_title"), h("p", { class: "help" }, osText("capture.c3")), h("div", {}, h("button", { type: "button", class: "btn sm", onclick: async () => { if (await useBoard()) draw(); } }, t("capture.use_board")))));
    } else if (!st.connected) {
      body.append(h("p", { class: "dim" }, t(st.board_error === "outdated_firmware" ? "kb.fw_old" : "kb.no_board")));
      if (st.board_error !== "outdated_firmware") body.append(offerCard());
      return;
    } else if (!info?.present) {
      body.append(h("p", { class: "dim" }, t("kb.no_keyboard")));
      return;
    } else {
      body.append(identityBlock(info, state.keyboard.identity), layoutBlock(), block(t("kb.reports"), h("p", { class: "help" }, t("kb.reports_help")), reports(info)), detectBlock());
    }
    drawWizard();
  }

  function drawWizard() {
    clear(wizardHost);
    const seen = new Set(Object.keys(learnData.seen || {}).map((k) => parseInt(k, 16)));
    const sugs = learnData.suggestions || [];
    const best = sugs[0];
    if (/iso/.test(best?.id || layoutId()) && seen.has(0x31) && !seen.has(0x32)) seen.add(0x32);
    wizardHost.append(h("div", { class: "row wrap" },
      h("button", { type: "button", class: `btn sm ${learning ? "" : "primary"}`, onclick: toggleLearn }, ic(learning ? "check" : "play", 14), learning ? t("kb.detect_stop") : t("kb.detect_start")),
      seen.size ? h("span", { class: "dim small" }, t("kb.detect_seen", { n: seen.size })) : null));
    if (learning) wizardHost.append(h("p", { class: "help" }, t("kb.detect_locked")));
    if (learning || seen.size) {
      wizardHost.append(h("div", { class: "progress", role: "progressbar", "aria-valuemin": "0", "aria-valuemax": "100", "aria-valuenow": String(Math.round((best?.recall || 0) * 100)) }, h("i", { style: { width: `${Math.min(100, (best?.recall || 0) * 100)}%` } })),
        miniKb(best?.id || layoutId(), { heat: (u) => (seen.has(u) ? 1 : undefined), count: () => "" }));
    }
    if (learning && learnData.ambiguous) {
      const keys = (learnData.hint_keys || []).map((u) => HINT_KEYS[u] || hex(u)).join(", ");
      wizardHost.append(h("p", { class: "help warn" }, ic("alert", 14), t("kb.press_these", { keys })));
    }
    if (learning || !best || seen.size < 8) return;
    if (learnData.ambiguous && sugs[1]) {
      const choice = (l) => h("button", { type: "button", class: "btn sm", disabled: l.id === layoutId(), onclick: () => applyLayout(l.id) }, t("kb.use_named", { layout: layoutLabel(l.id) }));
      wizardHost.append(h("p", {}, t("kb.which")), h("div", { class: "row wrap" }, choice(best), choice(sugs[1])));
    } else {
      wizardHost.append(h("p", {}, t("kb.suggest", { layout: layoutLabel(best.id), pct: Math.round(best.score * 100) })),
        h("div", { class: "row wrap" }, h("button", { type: "button", class: "btn sm primary", disabled: best.id === layoutId(), onclick: () => applyLayout(best.id) }, best.id === layoutId() ? t("kb.already") : t("kb.use_layout")),
          sugs[1] ? h("span", { class: "faint small" }, t("kb.alt", { layout: layoutLabel(sugs[1].id) })) : null));
    }
  }

  async function pull() { try { learnData = await api("/api/learn"); } catch {} drawWizard(); }

  async function toggleLearn() {
    try {
      await api("/api/learn", { method: "POST", body: { on: !learning } });
      learning = !learning;
      if (learning) learnData = { seen: {}, suggestions: [] }; else await pull();
    } catch (e) { toast(e.message, "bad"); }
    drawWizard();
    wizardHost.querySelector("button")?.focus();
  }

  let refreshTimer, drawnSig = "";
  const sig = () => { const k = state.keyboard, s = state.status; return JSON.stringify([s.connected, s.capture, s.board_error, k.info?.present, k.info?.vid, k.info?.pid, k.layout, k.identity?.display, layoutId()]); };
  const redraw = () => { if (sig() !== drawnSig) draw(); };
  const off = subscribe((topic) => {
    if (topic === "watch" && learning) { clearTimeout(pullTimer); pullTimer = setTimeout(pull, 250); }
    if (topic === "keyboard" || topic === "status") { clearTimeout(refreshTimer); refreshTimer = setTimeout(redraw, 50); }
  });

  const modal = openModal(panel, { label: t("kb.title"), drawer: true, onClose: () => {
    off(); clearTimeout(pullTimer); clearTimeout(refreshTimer);
    if (learning) api("/api/learn", { method: "POST", body: { on: false } }).catch(() => {});
  } });
  draw();
  refresh();
}
