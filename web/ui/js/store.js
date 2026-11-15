import { api, DRY } from "./api.js";
import { setLang, t } from "./i18n.js";
import { moveLayer as moveLayerIn, removeLayer as removeLayerIn, insertLayer as insertLayerIn, blankLayer, MAX_LAYERS } from "./layerops.js";

export const state = {
  cfg: null,
  appList: [], appSet: new Set(),
  backups: [],
  status: { connected: false, port: null, layer: 0, front: [], uptime: 0 },
  events: [],
  view: "keys", scope: 0, selKey: "04", panel: "key",
  keyboard: { connected: false, info: null, prefs: null, identity: null, layout: null },
  layouts: {}, system: null, packs: { categories: [], packs: [] },
  clip: null, follow: false, down: new Set(), live: false, obsInfo: null,
  save: "idle", saveError: "", lastId: 0,
};

const subs = new Set();
export function subscribe(fn) { subs.add(fn); return () => subs.delete(fn); }
export function notify(topic, detail) { for (const fn of [...subs]) fn(topic, detail); }

export function setKeyboard(kb) {
  state.keyboard = { connected: !!kb.connected, info: kb.info || null, prefs: kb.prefs || null, identity: kb.identity || null, layout: kb.layout || null };
}

const undoStack = [], redoStack = [];
let lastTag = null, lastTime = 0;
export const canUndo = () => undoStack.length > 0;
export const canRedo = () => redoStack.length > 0;

function snapshot() {
  undoStack.push(JSON.stringify(state.cfg));
  if (undoStack.length > 100) undoStack.shift();
  redoStack.length = 0;
}

export function edit(tag, fn) {
  const now = Date.now();
  if (!(tag && tag === lastTag && now - lastTime < 900)) snapshot();
  lastTag = tag; lastTime = now;
  fn(state.cfg);
  afterChange("edit");
}

function afterChange(kind) {
  clampSelection();
  scheduleSave();
  applySettings();
  notify("cfg", kind);
}

export function undo() { stepHistory(undoStack, redoStack, "undo"); }
export function redo() { stepHistory(redoStack, undoStack, "redo"); }
function stepHistory(from, to, kind) {
  if (!from.length) return;
  to.push(JSON.stringify(state.cfg));
  state.cfg = JSON.parse(from.pop());
  lastTag = null;
  afterChange(kind);
}

export function replaceConfig(cfg) {
  snapshot();
  state.cfg = cfg; lastTag = null;
  afterChange("replace");
}

function clampSelection() {
  const n = state.cfg.layers.length;
  if (state.scope !== "global" && state.scope >= n) state.scope = n - 1;
}

let saveTimer = null, dirty = 0, inflight = null;

function scheduleSave() {
  dirty++;
  setSave("saving");
  clearTimeout(saveTimer);
  saveTimer = setTimeout(flush, 320);
}

async function flush() {
  clearTimeout(saveTimer);
  if (inflight) await inflight.catch(() => {});
  if (!dirty) return;
  const sent = dirty;
  inflight = api("/api/config", { method: "POST", body: state.cfg });
  try {
    await inflight;
    if (dirty === sent) { dirty = 0; setSave("saved"); }
  } catch (e) {
    state.saveError = e.message;
    setSave("error");
  } finally {
    inflight = null;
  }
}

function setSave(s) {
  if (state.save === s && s !== "error") return;
  state.save = s;
  notify("save");
}

export const saveNow = () => flush();
export const savePending = () => dirty > 0;

window.addEventListener("pagehide", () => {
  if (!dirty) return;
  clearTimeout(saveTimer);
  api("/api/config", { method: "POST", body: state.cfg, keepalive: true }).catch(() => {});
});

export const settings = () => state.cfg.settings;
export const layerCount = () => state.cfg.layers.length;
export const keysOf = (scope = state.scope) => scope === "global" ? (state.cfg.global ||= {}) : (state.cfg.layers[scope].keys ||= {});
export const keyDef = (key, scope = state.scope) => keysOf(scope)[key];

export function ensureKey(key = state.selKey, scope = state.scope) { return (keysOf(scope)[key] ||= {}); }
export function pruneKey(key = state.selKey, scope = state.scope) {
  const kd = keysOf(scope)[key];
  if (kd && !kd.tap && !kd.hold && !kd.double && !kd.label && !kd.icon && !kd.color) delete keysOf(scope)[key];
}

export const canAddLayer = () => layerCount() < MAX_LAYERS;

export function addLayer(layer = blankLayer(t("layer.default_name", { n: layerCount() + 1 })), at = layerCount()) {
  if (!canAddLayer()) return false;
  edit("", (cfg) => { insertLayerIn(cfg, at, layer); });
  state.scope = at;
  return true;
}
export function moveLayer(from, to) { edit("", (cfg) => moveLayerIn(cfg, from, to)); state.scope = to; }
export function removeLayer(i) { edit("", (cfg) => removeLayerIn(cfg, i)); state.scope = Math.max(0, i - 1); }

export function newAction(type) {
  const base = { type };
  switch (type) {
    case "app": return { ...base, app: "", mode: "open" };
    case "url": return { ...base, url: "" };
    case "shell": return { ...base, cmd: "", show_output: false };
    case "ssh": return { ...base, host: "", cmd: "", show_output: true };
    case "http": return { ...base, method: "GET", url: "", body: "" };
    case "hotkey": return { ...base, keys: "" };
    case "text": return { ...base, text: "" };
    case "sequence": return { ...base, steps: [] };
    case "media": return { ...base, cmd: "playpause" };
    case "system": return { ...base, cmd: "screensaver" };
    case "timer": return { ...base, minutes: 25, label: "" };
    case "layer": return { ...base, to: "next" };
    case "hud": return { ...base, text: "" };
    case "wait": return { ...base, ms: 500 };
    case "if": return { ...base, cond: "app", target: "", steps: [], else: [] };
    case "obs": return { ...base, cmd: "record", target: "" };
    case "history": return { ...base, cmd: "toggle" };
  }
  return base;
}

export const firstAction = (kd) => kd && (kd.tap || kd.hold || kd.double);

export function describe(a) {
  if (!a) return "";
  switch (a.type) {
    case "app": return a.app || "";
    case "url": case "http": return (a.url || "").replace(/^https?:\/\//, "").slice(0, 40);
    case "shell": return (a.cmd || "").slice(0, 40);
    case "ssh": return `${a.host || ""}: ${a.cmd || ""}`.slice(0, 40);
    case "hotkey": return a.keys || "";
    case "text": case "hud": return (a.text || "").slice(0, 30);
    case "media": case "system": return t(a.type + "." + a.cmd);
    case "obs": return a.target ? `${t("obs." + a.cmd)}: ${a.target}` : t("obs." + a.cmd);
    case "timer": return `${a.minutes} min`;
    case "layer": return typeof a.to === "number" ? state.cfg.layers[a.to]?.name || String(a.to + 1) : t("layer." + a.to);
    case "history": return t("hist.act_" + (a.cmd || "toggle"));
    case "sequence": return t("seq.count", { n: (a.steps || []).length });
    case "if": return `${a.not ? t("cond.not") + " " : ""}${t("cond." + a.cond)}${a.target ? " " + a.target : ""}`.slice(0, 40);
    case "wait": return `${a.ms} ms`;
    default: return t("type." + a.type);
  }
}

export function applySettings() {
  const s = state.cfg.settings, root = document.documentElement;
  root.dataset.theme = s.theme || "auto";
  root.dataset.density = s.density || "comfortable";
  root.style.setProperty("--accent", s.accent || "#2563eb");
  root.lang = s.language;
  setLang(s.language);
}

export const DEFAULT_LAYOUT = "full-iso";
export function kbId() {
  const i = state.keyboard.info;
  return i && i.vid ? `${i.vid}:${i.pid}` : "";
}
export function layoutId() {
  const id = kbId();
  const saved = (id && state.cfg.keyboards?.[id]?.layout) || state.cfg.keyboards?.default?.layout;
  return saved || state.keyboard.layout?.id || DEFAULT_LAYOUT;
}
export async function setLayout(layout) {
  const r = await api("/api/keyboard/layout", { method: "POST", body: { layout } });
  state.cfg.keyboards = r.config?.keyboards || { ...(state.cfg.keyboards || {}), [kbId() || "default"]: { layout } };
  notify("cfg", "replace");
}

export function adoptRemote(cfg) {
  if (!cfg || DRY || savePending() || JSON.stringify(cfg) === JSON.stringify(state.cfg)) return false;
  state.cfg = cfg;
  clampSelection();
  applySettings();
  notify("cfg", "remote");
  return true;
}
