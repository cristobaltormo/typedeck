import { h, ic, clear } from "./dom.js";
import { api } from "./api.js";
import { t, lang } from "./i18n.js";
import { state } from "./store.js";

const pick = (v) => (v && typeof v === "object" ? v[lang()] || v.es : v);
const LEVEL_ICON = { ok: "check", info: "info", warn: "alert", error: "alert" };

export function compatView(root) {
  const mine = h("div");
  const matrix = h("div");

  function check(c) {
    const lvl = c.level;
    const text = t("check." + c.code, c.args || {});
    return h("div", { class: `check ${lvl}` }, ic(LEVEL_ICON[lvl] || "info", 16), h("span", {}, text));
  }

  async function drawMine() {
    clear(mine);
    let s;
    try { s = await api("/api/setup"); state.setup = s; } catch (e) { mine.append(h("p", { class: "dim" }, e.message)); return; }
    const rows = [];
    if (s.board?.connected) {
      const b = s.board.sys;
      rows.push([t("compat.board"), b ? `${b.board === "leonardo" ? "Arduino Leonardo" : b.board} (${b.mcu}, ${Math.round(b.f_cpu / 1e6)} MHz)` : t("diag.unknown")],
        [t("compat.shield"), b ? `MAX3421E ${t("compat.rev")} ${parseInt(b.max3421e_rev, 16) & 15}` : "-"], [t("compat.power"), b ? `${(b.vcc_mv / 1000).toFixed(2)} V` : "-"], [t("compat.fw"), `Typedeck FW ${s.board.firmware}`]);
    }
    const k = s.keyboard;
    if (k) rows.push([t("compat.keyboard"), k.identity.display || t("info.unnamed")], [t("compat.format"), k.layout.id ? `${k.layout.percent || ""}${k.layout.percent ? "% " : ""}${(k.layout.standard || "").toUpperCase()}`.trim() || t("layout.numpad") : "-"]);
    const hs = s.host;
    rows.push([t("compat.mac"), `macOS ${hs.version} (${hs.arch})`], [t("compat.typing"), t("layout.sys." + (hs.typing_layout || "none"))]);
    const dl = h("dl", { class: "kv" });
    for (const [a, b] of rows) dl.append(h("dt", {}, a), h("dd", {}, b));
    mine.append(dl, h("div", { class: "checks" }, s.checks.map(check)));
  }

  async function drawMatrix() {
    let c;
    try { c = await api("/api/compat"); state.compat = c; } catch (e) { return; }
    clear(matrix);
    for (const g of c.groups) {
      matrix.append(h("section", { class: "section" }, h("h2", {}, pick(g.name)),
        h("div", { class: "rowlist" }, g.items.map((it) => h("div", { class: "compat-row" },
          h("span", { class: `badge ${it.status}` }, pick(c.status_legend[it.status])), h("div", { class: "grow" }, h("b", { class: "strong", style: { fontWeight: 400 } }, (lang() === "en" && it.name_en) || it.name), h("p", { class: "dim", style: { fontSize: "13px" } }, pick(it.note))))))));
    }
  }

  root.append(h("div", { class: "page" }, h("header", {}, h("h1", {}, t("compat.title")), h("p", {}, t("compat.sub"))),
    h("section", { class: "section" }, h("h2", {}, t("compat.yours")), h("div", { style: { paddingTop: "14px" } }, mine)),
    h("div", {}, h("h1", { style: { fontSize: "17px", margin: "30px 0 6px" } }, t("compat.others")), h("p", { class: "dim", style: { marginBottom: "18px" } }, t("compat.others_sub"))), matrix));
  drawMine(); drawMatrix();
  return { on(topic) { if (topic === "keyboard" || topic === "status") drawMine(); } };
}
