import { h, ic, clear, pageHead } from "./dom.js";
import { api } from "./api.js";
import { t, tOS, lang, pick } from "./i18n.js";
import { state } from "./store.js";

const LEVEL_ICON = { ok: "check", info: "info", warn: "alert", error: "alert" };
const OS_NAME = { darwin: "macOS", windows: "Windows", linux: "Linux" };

export function compatView(root) {
  const mine = h("div");
  const matrix = h("div");
  let seq = 0;

  const check = (c) => h("li", { class: `check ${c.level}` }, ic(LEVEL_ICON[c.level] || "info", 16), h("span", {}, tOS("check." + c.code, state.system?.os, c.args || {})));

  async function drawMine() {
    const mineSeq = ++seq;
    let s;
    try { s = await api("/api/setup"); } catch (e) { clear(mine).append(h("p", { class: "dim" }, e.message)); return; }
    if (mineSeq !== seq) return;
    const rows = [];
    const soft = s.board?.capture === "software";
    if (soft) rows.push([t("compat.capture"), t("capture.software")]);
    if (s.board?.connected && !soft) {
      const b = s.board.sys;
      rows.push([t("compat.board"), b ? `${b.board === "leonardo" ? "Arduino Leonardo" : b.board} (${b.mcu}, ${Math.round(b.f_cpu / 1e6)} MHz)` : t("diag.unknown")],
        [t("compat.shield"), b ? `MAX3421E ${t("compat.rev")} ${parseInt(b.max3421e_rev, 16) & 15}` : "-"], [t("compat.power"), b ? `${(b.vcc_mv / 1000).toFixed(2)} V` : "-"], [t("compat.fw"), `Typedeck FW ${s.board.firmware}`]);
    }
    const k = s.keyboard;
    if (k) {
      const l = k.layout || {};
      rows.push([t("compat.keyboard"), soft ? t("capture.sys_keyboard") : k.identity?.display || t("info.unnamed")],
        [t("compat.format"), l.id ? `${l.percent ? l.percent + "% " : ""}${(l.standard || "").toUpperCase()}`.trim() || t("layout.numpad") : "-"]);
    }
    const hs = s.host || {};
    rows.push([t("compat.mac"), `${OS_NAME[hs.os] || hs.os || ""} ${hs.version || ""} (${hs.arch || ""})`], [t("compat.typing"), t("layout.sys." + (hs.typing_layout || "none"))]);
    const dl = h("dl", { class: "kv" });
    for (const [a, b] of rows) dl.append(h("dt", {}, a), h("dd", {}, b));
    clear(mine).append(dl, h("ul", { class: "checks" }, (s.checks || []).map(check)));
  }

  async function drawMatrix() {
    let c;
    try { c = await api("/api/compat"); } catch { return; }
    clear(matrix);
    for (const g of c.groups) {
      matrix.append(h("section", { class: "section" }, h("h3", {}, pick(g.name)),
        h("ul", { class: "rowlist" }, g.items.map((it) => h("li", { class: "compat-row" },
          h("span", { class: `badge ${it.status}` }, pick(c.status_legend[it.status])),
          h("div", { class: "grow" }, h("span", { class: "strong" }, (lang() === "en" && it.name_en) || it.name), h("p", { class: "dim small" }, pick(it.note))))))));
    }
  }

  root.append(h("div", { class: "page" }, pageHead(t("compat.sub")),
    h("section", { class: "section" }, h("h2", {}, t("compat.yours")), mine),
    h("section", { class: "section" }, h("h2", {}, t("compat.others")), h("p", { class: "sub" }, t("compat.others_sub")), matrix)));
  drawMine(); drawMatrix();
  let timer;
  return { on(topic) { if (topic === "keyboard" || topic === "status") { clearTimeout(timer); timer = setTimeout(drawMine, 300); } }, unmount() { clearTimeout(timer); } };
}
