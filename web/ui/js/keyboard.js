import { h, ic } from "./dom.js";
import { layout, keyName } from "./layouts.js";
import { state, describe, firstAction } from "./store.js";
import { t } from "./i18n.js";

export const TYPE_ICON = { app: "grid", url: "globe", shell: "terminal", ssh: "server", http: "cloud", hotkey: "command",
  text: "type", sequence: "list", media: "play", system: "power", timer: "clock", layer: "layers", hud: "hud", wait: "wait", obs: "camera", if: "sliders", history: "history" };

function missingApp(kd) {
  if (!state.appSet.size) return false;
  return ["tap", "hold", "double"].map((g) => kd[g]).some((a) => a && a.type === "app" && a.app && !state.appSet.has(a.app.toLowerCase()));
}

function capContent(k, kd, o) {
  const lg = k.legend;
  const first = firstAction(kd);
  const native = lg.icon ? ic(lg.icon, 14) : null;
  if (!first && !kd?.label && !kd?.icon) {
    if (native) return [h("span", { class: "lg c" }, native)];
    return [lg.shift && lg.shift !== lg.main ? h("span", { class: "lg s" }, lg.shift) : null,
      h("span", { class: "lg m" + (lg.main.length > 4 ? " sm" : lg.main.length > 2 ? " md" : "") }, lg.main)];
  }
  const kids = [];
  const icon = kd.icon || TYPE_ICON[first?.type] || "grid";
  const label = kd.label || describe(first);
  kids.push(h("span", { class: "lg tl" }, lg.icon ? native : lg.main.slice(0, 4)));
  const text = String(label), f0 = o.u * 0.27 * 0.72, charW = 0.6 * f0, avail = k.w * o.u * 0.78, usable = k.h * o.u * 0.62;
  const word = Math.max(1, ...text.split(/\s+/).map((x) => x.length));
  const scale = Math.min(1, avail / (word * charW));
  const lines = text.length * charW * scale > avail ? 2 : 1;
  const labelH = lines * f0 * scale * 1.12;
  const full = Math.max(14, Math.min(26, o.u * 0.5));
  const iconPx = [full, Math.max(12, o.u * 0.34), 0].find((px) => px + (px ? 3 : 0) + labelH <= usable);
  const showLabel = o.labels !== false && !!text && scale >= 0.62 && iconPx !== undefined;
  kids.push(h("span", { class: "mc" }, showLabel && iconPx === 0 ? null : ic(icon, showLabel ? iconPx : full),
    showLabel && h("span", { class: "ml", style: scale < 1 ? { fontSize: `${(0.72 * scale).toFixed(3)}em` } : {} }, label)));
  const pips = [kd.hold && "hold", kd.double && "double"].filter(Boolean);
  if (pips.length) kids.push(h("span", { class: "pips", title: pips.map((p) => t("gesture." + p)).join(", ") }, pips.map(() => h("i"))));
  if (missingApp(kd)) kids.push(h("span", { class: "warn", title: t("warn.app_missing") }, ic("alert", 12)));
  return kids;
}

function cropRegion(L, getKd) {
  const used = L.keys.filter((k) => firstAction(getKd(k.usage)?.kd));
  if (!used.length) return { x0: 0, y0: 0, w: L.width, h: L.height };
  const x0 = Math.max(0, Math.floor(Math.min(...used.map((k) => k.x)) - 0.25)), y0 = Math.max(0, Math.floor(Math.min(...used.map((k) => k.y)) - 0.25));
  const x1 = Math.min(L.width, Math.ceil(Math.max(...used.map((k) => k.x + k.w)) + 0.25)), y1 = Math.min(L.height, Math.ceil(Math.max(...used.map((k) => k.y + k.h)) + 0.25));
  return { x0, y0, w: Math.max(2, x1 - x0), h: Math.max(1, y1 - y0) };
}

function dragHandler(el, k, onMove) {
  return (e) => {
    if (e.button !== 0) return;
    const sx = e.clientX, sy = e.clientY;
    let ghost = null, over = null;
    const target = (ev) => {
      const c = document.elementFromPoint(ev.clientX, ev.clientY)?.closest?.(".cap");
      return c && c !== el && c.closest(".kb") === el.closest(".kb") && !c.classList.contains("mod") ? c : null;
    };
    const end = () => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", up);
      window.removeEventListener("pointercancel", end);
      window.removeEventListener("keydown", esc, true);
      over?.classList.remove("drop");
      ghost?.remove();
      el.classList.remove("dragging");
      document.body.classList.remove("dragging-key");
    };
    const esc = (ev) => { if (ev.key === "Escape") { ev.preventDefault(); ev.stopPropagation(); end(); } };
    const move = (ev) => {
      if (!ghost) {
        if (Math.hypot(ev.clientX - sx, ev.clientY - sy) < 6) return;
        ghost = el.cloneNode(true);
        ghost.classList.add("ghost");
        ghost.classList.remove("sel");
        Object.assign(ghost.style, { position: "fixed", left: "0", top: "0", pointerEvents: "none", width: el.offsetWidth + "px", height: el.offsetHeight + "px", zIndex: 60 });
        document.body.append(ghost);
        el.classList.add("dragging");
        document.body.classList.add("dragging-key");
      }
      ghost.style.transform = `translate(${ev.clientX - el.offsetWidth / 2}px, ${ev.clientY - el.offsetHeight / 2}px)`;
      const next = target(ev);
      if (over !== next) { over?.classList.remove("drop"); next?.classList.add("drop"); over = next; }
    };
    const up = (ev) => {
      const dragged = !!ghost, dest = dragged ? target(ev) : null;
      end();
      if (!dragged) return;
      const swallow = (c) => { c.stopImmediatePropagation(); c.preventDefault(); };
      window.addEventListener("click", swallow, { capture: true, once: true });
      setTimeout(() => window.removeEventListener("click", swallow, true), 0);
      if (dest) onMove(k.usage, parseInt(dest.dataset.u, 10), ev.altKey);
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
    window.addEventListener("pointercancel", end);
    window.addEventListener("keydown", esc, true);
  };
}

export function keyboard(layoutId, lang, getKd, opts = {}) {
  const L = layout(layoutId, lang);
  const plate = h("div", { class: "kb-plate", style: opts.color ? { "--lc": opts.color } : {} });
  const wrap = h("div", { class: "kb", "data-layout": layoutId }, plate);
  const caps = new Map();
  let u = opts.u || 40, selected = opts.selected;
  const pad = opts.crop ? 8 : 12;
  const region = opts.crop ? cropRegion(L, getKd) : { x0: 0, y0: 0, w: L.width, h: L.height };
  if (opts.crop && opts.width) u = Math.max(14, Math.min(opts.maxU || 44, Math.floor((opts.width - pad * 2) / region.w)));
  const inRegion = (k) => k.x + k.w > region.x0 && k.x < region.x0 + region.w && k.y + k.h > region.y0 && k.y < region.y0 + region.h;
  const selectable = (k) => opts.onSelect && !k.mod;

  for (const k of L.keys) {
    if (opts.crop && !inRegion(k)) continue;
    const { kd, inherited } = getKd(k.usage) || {};
    const mapped = !!firstAction(kd);
    const heat = opts.heat?.(k.usage);
    const cls = ["cap", k.mod && "mod", (mapped || kd?.label || kd?.icon) && "mapped", inherited && "inherited", selected === k.usage && "sel",
      k.legend.shift && !mapped && "dual", kd?.color && "colored", heat !== undefined && "heat"].filter(Boolean).join(" ");
    const el = h(selectable(k) ? "button" : "div", {
      class: cls, style: { "--c": kd?.color, "--heat": heat }, dataset: { u: String(k.usage) },
      title: opts.titles === false ? undefined : titleFor(k, kd),
      type: selectable(k) ? "button" : undefined,
      onclick: selectable(k) ? () => opts.onSelect(k.usage) : undefined,
    }, heat !== undefined ? [h("span", { class: "lg tl" }, k.legend.icon ? ic(k.legend.icon, 12) : k.legend.main), h("span", { class: "count" }, opts.count?.(k.usage) || "")] : capContent(k, kd, { u, labels: opts.labels }));
    if (selectable(k)) { el.setAttribute("aria-pressed", String(selected === k.usage)); el.tabIndex = selected === k.usage ? 0 : -1; el.setAttribute("aria-label", titleFor(k, kd)); }
    if (opts.onMove && selectable(k) && mapped && !inherited) {
      el.classList.add("movable");
      el.addEventListener("pointerdown", dragHandler(el, k, opts.onMove));
    }
    caps.set(k.usage, { el, k });
    plate.append(el);
  }
  if (opts.onSelect && caps.get(selected)?.el.tagName !== "BUTTON") {
    const first = [...caps.values()].find((c) => c.el.tagName === "BUTTON");
    if (first) first.el.tabIndex = 0;
  }

  function layoutCaps() {
    const gap = Math.max(2, Math.round(u * 0.08));
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
    if (opts.u || !wrap.parentElement) return;
    const availW = wrap.parentElement.clientWidth - pad * 2 - 2;
    let next = Math.floor(availW / region.w);
    const availH = opts.fitHeight ? opts.fitHeight() : 0;
    if (availH > 0) next = Math.min(next, Math.floor((availH - pad * 2) / region.h));
    next = Math.max(opts.minU || 20, Math.min(opts.maxU || 58, next));
    if (next !== u) { u = next; layoutCaps(); opts.onResize?.(); }
  }

  function select(usage) {
    const prev = caps.get(selected), next = caps.get(usage);
    if (prev) { prev.el.classList.remove("sel"); prev.el.setAttribute("aria-pressed", "false"); prev.el.tabIndex = -1; }
    selected = usage;
    if (next) { next.el.classList.add("sel"); next.el.setAttribute("aria-pressed", "true"); next.el.tabIndex = 0; }
  }

  layoutCaps();
  let ro;
  if (!opts.u && typeof ResizeObserver !== "undefined") {
    ro = new ResizeObserver(() => fit());
    queueMicrotask(() => { if (!wrap.parentElement) return; ro.observe(wrap.parentElement); if (opts.observe) ro.observe(opts.observe); fit(); });
  }
  return { el: wrap, caps, layout: L, select, destroy: () => ro?.disconnect() };
}

function titleFor(k, kd) {
  const name = keyName(k.usage);
  const first = firstAction(kd);
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
