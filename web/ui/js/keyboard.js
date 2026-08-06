import { h, ic } from "./dom.js";
import { layout } from "./layouts.js";
import { state, describe } from "./store.js";
import { t } from "./i18n.js";

export const hex = (u) => u.toString(16).toUpperCase().padStart(2, "0");

export const TYPE_ICON = { app: "grid", url: "globe", shell: "terminal", ssh: "server", http: "cloud", hotkey: "command",
  text: "type", sequence: "list", media: "play", system: "power", timer: "clock", layer: "layers", hud: "hud", wait: "wait" };

function missingApp(kd) {
  if (!state.appSet.size) return false;
  return ["tap", "hold", "double"].map((g) => kd[g]).some((a) => a && a.type === "app" && a.app && !state.appSet.has(a.app.toLowerCase()));
}

function capContent(k, kd, o) {
  const kids = [];
  const lg = k.legend;
  const mapped = kd && (kd.tap || kd.hold || kd.double);
  const native = lg.icon ? ic(lg.icon, 14) : null;
  if (!mapped && !kd?.label && !kd?.icon) {
    if (native) kids.push(h("span", { class: "lg c" }, native));
    else {
      if (lg.shift && lg.shift !== lg.main) kids.push(h("span", { class: "lg s" }, lg.shift));
      kids.push(h("span", { class: "lg m" + (lg.main.length > 4 ? " sm" : lg.main.length > 2 ? " md" : "") }, lg.main));
    }
    return kids;
  }
  const first = kd.tap || kd.hold || kd.double;
  const icon = kd.icon || TYPE_ICON[first?.type] || "grid";
  const label = kd.label || describe(first);
  kids.push(h("span", { class: "lg tl" }, lg.icon ? native : lg.main.length > 4 ? lg.main.slice(0, 4) : lg.main));
  kids.push(h("span", { class: "mc" }, ic(icon, Math.max(14, Math.min(26, o.u * 0.5))), o.labels !== false && h("span", { class: "ml" }, label)));
  const pips = [kd.hold && "hold", kd.double && "double"].filter(Boolean);
  if (pips.length) kids.push(h("span", { class: "pips", title: pips.map((p) => t("gesture." + p)).join(", ") }, pips.map(() => h("i"))));
  if (missingApp(kd)) kids.push(h("span", { class: "warn", title: t("warn.app_missing") }, ic("alert", 13)));
  return kids;
}

export function keyboard(layoutId, lang, getKd, opts = {}) {
  const L = layout(layoutId, lang);
  const plate = h("div", { class: "kb-plate", style: opts.color ? { "--lc": opts.color } : {} });
  const wrap = h("div", { class: "kb", "data-layout": layoutId }, plate);
  const caps = new Map();
  let u = opts.u || 46;
  const pad = 14;

  for (const k of L.keys) {
    const { kd, inherited } = getKd(k.usage) || {};
    const mapped = kd && (kd.tap || kd.hold || kd.double);
    const cls = ["cap", k.mod ? "mod" : "", mapped || kd?.label || kd?.icon ? "mapped" : "", inherited ? "inherited" : "", opts.selected === k.usage ? "sel" : "",
      k.legend.shift && !mapped ? "dual" : "", k.w === 1 && k.h === 1 ? "unit" : ""].filter(Boolean).join(" ");
    const style = {};
    if (kd?.color) { style["--c"] = kd.color; }
    const heat = opts.heat?.(k.usage);
    if (heat !== undefined) style["--heat"] = heat;
    const el = h(opts.onSelect && !k.mod ? "button" : "div", {
      class: cls + (kd?.color ? " colored" : "") + (heat !== undefined ? " heat" : ""), style, dataset: { u: String(k.usage) },
      title: opts.titles === false ? undefined : titleFor(k, kd),
      onclick: opts.onSelect && !k.mod ? () => opts.onSelect(k.usage) : undefined,
    }, heat !== undefined ? [h("span", { class: "lg tl" }, k.legend.icon ? ic(k.legend.icon, 12) : k.legend.main), h("span", { class: "count" }, opts.count?.(k.usage) || "")] : capContent(k, kd, { u, labels: opts.labels }));
    if (opts.onSelect && !k.mod) el.setAttribute("aria-pressed", String(opts.selected === k.usage));
    caps.set(k.usage, { el, k });
    plate.append(el);
  }

  function layoutCaps() {
    const gap = Math.max(3, u * 0.1);
    plate.style.width = L.width * u + pad * 2 + "px";
    plate.style.height = L.height * u + pad * 2 + "px";
    wrap.style.setProperty("--u", u + "px");
    for (const { el, k } of caps.values()) {
      el.style.left = pad + k.x * u + gap / 2 + "px";
      el.style.top = pad + k.y * u + gap / 2 + "px";
      el.style.width = k.w * u - gap + "px";
      el.style.height = k.h * u - gap + "px";
    }
  }

  function fit() {
    if (opts.u) return;
    const availW = (wrap.parentElement?.clientWidth || 900) - pad * 2 - 4;
    let next = Math.floor(availW / L.width);
    const availH = opts.fitHeight ? opts.fitHeight() : 0;
    if (availH > 0) next = Math.min(next, Math.floor((availH - pad * 2 - 6) / L.height));
    next = Math.max(opts.minU || 20, Math.min(opts.maxU || 58, next));
    if (next !== u) { u = next; layoutCaps(); }
  }

  u = opts.u || u;
  layoutCaps();
  let ro;
  if (!opts.u && typeof ResizeObserver !== "undefined") {
    ro = new ResizeObserver(() => fit());
    queueMicrotask(() => { if (wrap.parentElement) { ro.observe(wrap.parentElement); fit(); } });
  }
  return { el: wrap, caps, layout: L, destroy: () => ro?.disconnect() };
}

function titleFor(k, kd) {
  const name = k.legend.icon ? t("key.arrow") : k.legend.main || "";
  if (!kd) return name;
  const first = kd.tap || kd.hold || kd.double;
  return first ? `${name}: ${kd.label || describe(first)}` : name;
}

export function neighbor(L, usage, dx, dy) {
  const cur = L.keys.find((k) => k.usage === usage);
  if (!cur) return usage;
  const cx = cur.x + cur.w / 2, cy = cur.y + cur.h / 2;
  let best = null, bestScore = Infinity;
  for (const k of L.keys) {
    if (k === cur || k.mod) continue;
    const kx = k.x + k.w / 2, ky = k.y + k.h / 2;
    const ddx = kx - cx, ddy = ky - cy;
    const along = ddx * dx + ddy * dy;
    if (along <= 0.2) continue;
    const across = Math.abs(ddx * dy) + Math.abs(ddy * dx);
    const score = along + across * 2.2;
    if (score < bestScore) { bestScore = score; best = k; }
  }
  return best ? best.usage : usage;
}
