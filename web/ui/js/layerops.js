export const MAX_LAYERS = 9;
const GESTURES = ["tap", "hold", "double"];

function walk(a, fn) {
  if (!a || typeof a !== "object") return;
  fn(a);
  for (const s of a.steps || []) walk(s, fn);
  for (const s of a.else || []) walk(s, fn);
}

export function forEachAction(cfg, fn) {
  const keysets = [cfg.global || {}, ...cfg.layers.map((l) => l.keys || {})];
  for (const keys of keysets) for (const kd of Object.values(keys)) for (const g of GESTURES) walk(kd?.[g], fn);
}

function remap(cfg, map, fallback) {
  const to = (i) => (map[i] === undefined ? i : map[i] < 0 ? fallback : map[i]);
  forEachAction(cfg, (a) => {
    if (a.type === "layer" && typeof a.to === "number") a.to = to(a.to);
    if (a.type === "if" && a.cond === "layer" && /^\d+$/.test(String(a.target ?? "").trim())) a.target = String(to(Number(a.target) - 1) + 1);
  });
}

export function moveLayer(cfg, from, to) {
  if (from === to || to < 0 || to >= cfg.layers.length) return;
  const order = cfg.layers.map((_, i) => i);
  order.splice(to, 0, ...order.splice(from, 1));
  const map = [];
  order.forEach((old, now) => { map[old] = now; });
  cfg.layers = order.map((i) => cfg.layers[i]);
  remap(cfg, map, 0);
}

export function removeLayer(cfg, index) {
  if (cfg.layers.length <= 1) return;
  const map = cfg.layers.map((_, i) => (i < index ? i : i === index ? -1 : i - 1));
  cfg.layers.splice(index, 1);
  remap(cfg, map, Math.max(0, index - 1));
}

export function insertLayer(cfg, at, layer) {
  if (cfg.layers.length >= MAX_LAYERS) return false;
  const map = cfg.layers.map((_, i) => (i < at ? i : i + 1));
  remap(cfg, map, 0);
  cfg.layers.splice(at, 0, layer);
  return true;
}

export const blankLayer = (name) => ({ name, color: "", icon: "", auto_apps: [], keys: {} });

export function looksLikeConfig(c) {
  return !!c && typeof c === "object" && !Array.isArray(c) && !c.typedeck_macros && Array.isArray(c.layers) && c.layers.length > 0 &&
    c.layers.every((l) => l && typeof l === "object" && typeof (l.keys ?? {}) === "object") && !!c.settings && typeof c.settings === "object";
}
