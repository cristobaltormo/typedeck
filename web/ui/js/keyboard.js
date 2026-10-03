import { h, ic } from "./dom.js";
import { layout } from "./layouts.js";
import { state, describe } from "./store.js";
import { t } from "./i18n.js";

export const hex = (u) => u.toString(16).toUpperCase().padStart(2, "0");

export const TYPE_ICON = { app: "grid", url: "globe", shell: "terminal", ssh: "server", http: "cloud", hotkey: "command",
  text: "type", sequence: "list", media: "play", system: "power", timer: "clock", layer: "layers", hud: "hud", wait: "wait", obs: "camera", if: "sliders", history: "history" };

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
  const text = String(label), f0 = o.u * 0.27 * 0.72, charW = 0.6 * f0, avail = k.w * o.u * 0.78, usable = k.h * o.u * 0.62;
  const word = Math.max(1, ...text.split(/\s+/).map((x) => x.length));
  const scale = Math.min(1, avail / (word * charW));
  const lines = text.length * charW * scale > avail ? 2 : 1;
  const labelH = lines * f0 * scale * 1.12;
  const full = Math.max(14, Math.min(26, o.u * 0.5));
  const iconPx = [full, Math.max(12, o.u * 0.34), 0].find((px) => px + (px ? 3 : 0) + labelH <= usable);
  const showLabel = o.labels !== false && scale >= 0.62 && iconPx !== undefined;
  kids.push(h("span", { class: "mc" }, showLabel && iconPx === 0 ? null : ic(icon, showLabel ? iconPx : full),
    showLabel && h("span", { class: "ml", style: scale < 1 ? { fontSize: `${(0.72 * scale).toFixed(3)}em` } : {} }, label)));
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
  const pad = opts.crop ? 8 : 14;
  let region = { x0: 0, y0: 0, w: L.width, h: L.height };
  if (opts.crop) {
    const used = L.keys.filter((k) => { const kd = (getKd(k.usage) || {}).kd; return kd && (kd.tap || kd.hold || kd.double); });
    if (used.length) {
      const x0 = Math.max(0, Math.floor(Math.min(...used.map((k) => k.x)) - 0.25)), y0 = Math.max(0, Math.floor(Math.min(...used.map((k) => k.y)) - 0.25));
      const x1 = Math.min(L.width, Math.ceil(Math.max(...used.map((k) => k.x + k.w)) + 0.25)), y1 = Math.min(L.height, Math.ceil(Math.max(...used.map((k) => k.y + k.h)) + 0.25));
      region = { x0, y0, w: Math.max(2, x1 - x0), h: Math.max(1, y1 - y0) };
    }
    if (opts.width) u = Math.max(14, Math.min(opts.maxU || 44, Math.floor((opts.width - pad * 2) / region.w)));
  }
  const inRegion = (k) => k.x + k.w > region.x0 && k.x < region.x0 + region.w && k.y + k.h > region.y0 && k.y < region.y0 + region.h;

  for (const k of L.keys) {
    if (opts.crop && !inRegion(k)) continue;
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
    if (opts.onMove && !k.mod && mapped && !inherited) {
      el.classList.add("movable");
      el.addEventListener("pointerdown", (e) => {
        if (e.button !== 0) return;
        const sx = e.clientX, sy = e.clientY;
        let ghost = null, over = null;
        const target = (ev) => { const c = document.elementFromPoint(ev.clientX, ev.clientY)?.closest?.(".cap"); return c && c !== el && wrap.contains(c) && !c.classList.contains("mod") ? c : null; };
        const move = (ev) => {
          if (!ghost) {
            if (Math.hypot(ev.clientX - sx, ev.clientY - sy) < 6) return;
            ghost = el.cloneNode(true);
            ghost.classList.add("ghost");
            Object.assign(ghost.style, { position: "fixed", pointerEvents: "none", width: el.offsetWidth + "px", height: el.offsetHeight + "px", zIndex: 60 });
            document.body.append(ghost);
            el.classList.add("dragging");
            document.body.classList.add("dragging-key");
          }
          ghost.style.left = ev.clientX - el.offsetWidth / 2 + "px";
          ghost.style.top = ev.clientY - el.offsetHeight / 2 + "px";
          const t = target(ev);
          if (over !== t) { over?.classList.remove("drop"); t?.classList.add("drop"); over = t; }
        };
        const up = (ev) => {
          window.removeEventListener("pointermove", move);
          if (!ghost) return;
          const t = target(ev);
          over?.classList.remove("drop");
          ghost.remove();
          el.classList.remove("dragging");
          document.body.classList.remove("dragging-key");
          const swallow = (c) => { c.stopImmediatePropagation(); c.preventDefault(); };
          window.addEventListener("click", swallow, { capture: true, once: true });
          setTimeout(() => window.removeEventListener("click", swallow, true), 0);
          if (t) opts.onMove(k.usage, parseInt(t.dataset.u, 10), ev.altKey);
        };
        window.addEventListener("pointermove", move);
        window.addEventListener("pointerup", up, { once: true });
      });
    }
    caps.set(k.usage, { el, k });
    plate.append(el);
  }

  function layoutCaps() {
    const gap = Math.max(3, u * 0.1);
    plate.style.width = region.w * u + pad * 2 + "px";
    plate.style.height = region.h * u + pad * 2 + "px";
    wrap.style.setProperty("--u", u + "px");
    for (const { el, k } of caps.values()) {
      el.style.left = pad + (k.x - region.x0) * u + gap / 2 + "px";
      el.style.top = pad + (k.y - region.y0) * u + gap / 2 + "px";
      el.style.width = k.w * u - gap + "px";
      el.style.height = k.h * u - gap + "px";
    }
  }

  function fit() {
    if (opts.u) return;
    const availW = (wrap.parentElement?.clientWidth || 900) - pad * 2 - 4;
    let next = Math.floor(availW / region.w);
    const availH = opts.fitHeight ? opts.fitHeight() : 0;
    if (availH > 0) next = Math.min(next, Math.floor((availH - pad * 2 - 6) / region.h));
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
