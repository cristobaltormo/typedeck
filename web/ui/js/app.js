import { h, ic, clear, isTyping, MOD } from "./dom.js";
import { api, TAB } from "./api.js";
import { t } from "./i18n.js";
import { state, subscribe, notify, applySettings, undo, redo, canUndo, canRedo, setKeyboard, adoptRemote } from "./store.js";
import { appsDatalist } from "./controls.js";
import { modalOpen } from "./modal.js";
import { workView } from "./workbench.js";
import { galleryView } from "./gallery.js";
import { activityView } from "./activity.js";
import { historyView } from "./history.js";
import { sheetView } from "./sheet.js";
import { diagView } from "./diag.js";
import { compatView } from "./compat.js";
import { settingsView } from "./settings.js";
import { openPalette } from "./palette.js";
import { openKeyboardDrawer } from "./kbinfo.js";
import { startLive, isLive } from "./live.js";
import { isSoftware, watchBoard, osText } from "./capture.js";

const VIEWS = { keys: workView, gallery: galleryView, activity: activityView, history: historyView, sheet: sheetView, compat: compatView, diag: diagView, settings: settingsView };
const NAV = [["keys", "keyboard"], ["gallery", "grid"], null, ["activity", "activity"], ["history", "history"], ["sheet", "list"], ["compat", "layers"], ["diag", "cpu"]];
const viewId = () => { const id = location.hash.replace(/^#\/?/, ""); return Object.hasOwn(VIEWS, id) ? id : "keys"; };
let current = null, viewHost, navEl, titleEl, deviceEl, saveEl, undoBtn, redoBtn, findLabel;

async function boot() {
  const root = document.getElementById("app");
  try {
    const [cfg, apps, packs, status, kb, layouts, system] = await Promise.all([api("/api/config"), api("/api/apps"), api("/api/packs"), api("/api/status"), api("/api/keyboard"), api("/api/layouts"), api("/api/system").catch(() => null)]);
    state.cfg = cfg; state.appList = apps; state.appSet = new Set(apps.map((a) => a.toLowerCase()));
    state.packs = packs; Object.assign(state.status, status); state.lastId = status.last_id || 0;
    state.layouts = Object.fromEntries(layouts.map((l) => [l.id, l])); state.system = system;
    setKeyboard(kb);
  } catch (e) {
    root.append(h("div", { class: "fatal" }, h("h1", {}, "Typedeck"), h("p", {}, e.message === "token" ? "Reload the page." : e.message)));
    return;
  }
  applySettings();
  appsDatalist(state.appList);
  document.title = state.status.app_name || "Typedeck";
  shell(root);
  window.addEventListener("hashchange", route);
  document.addEventListener("keydown", globalKeys);
  subscribe((topic, detail) => {
    if (topic === "palette") openPalette();
    if (topic === "lang") { shell(root); route(true); return; }
    if (topic === "cfg" || topic === "save") drawTools();
    if (topic === "status" || topic === "keyboard" || topic === "live") drawDevice();
    current?.api?.on?.(topic, detail);
  });
  route();
  loadEvents();
  const q = new URLSearchParams(location.search);
  if (q.has("scene")) { (await import("./scenes.js")).run(q.get("scene")); return; }
  if (q.has("selftest")) { startLive(onLive); (await import("./selftest.js")).run(); return; }
  if (!q.has("nolive")) { startLive(onLive, onPlainKey); eventsLoop(); watchBoard(); }
}

function shell(root) {
  navEl = h("nav", { class: "nav", "aria-label": t("nav.aria") });
  deviceEl = h("div", { class: "device" });
  titleEl = h("h1", { class: "pagetitle" });
  saveEl = h("span", { class: "savestate", role: "status" });
  undoBtn = h("button", { type: "button", class: "tool", "aria-label": t("tool.undo"), title: `${t("tool.undo")} (${MOD} Z)`, onclick: undo }, ic("undo", 16));
  redoBtn = h("button", { type: "button", class: "tool", "aria-label": t("tool.redo"), title: `${t("tool.redo")} (${MOD} Shift Z)`, onclick: redo }, ic("redo", 16));
  findLabel = h("span", { class: "find-label" }, t("tool.palette"));
  const find = h("button", { type: "button", class: "find", onclick: () => openPalette(), "aria-keyshortcuts": MOD === "Cmd" ? "Meta+K" : "Control+K" }, ic("search", 15), findLabel, h("kbd", {}, `${MOD} K`));
  clear(root).append(
    h("a", { class: "skip", href: "#main" }, t("nav.skip")),
    h("aside", { class: "side" }, brand(), navEl, deviceEl),
    h("div", { class: "content" },
      h("header", { class: "top" }, titleEl, h("div", { class: "right" }, saveEl, undoBtn, redoBtn, find)),
      (viewHost = h("main", { id: "main", class: "view", tabIndex: -1 }))));
  drawNav(); drawTools(); drawDevice();
}

function brand() {
  const mark = h("span", { class: "mark", "aria-hidden": "true" });
  mark.innerHTML = `<svg width="22" height="22" viewBox="0 0 32 32"><rect x="2" y="2" width="28" height="28" rx="7" fill="currentColor"/><rect x="8" y="7" width="7" height="7" rx="1.6" fill="var(--side)"/><rect x="17" y="7" width="7" height="7" rx="1.6" fill="var(--side)" opacity=".6"/><rect x="8" y="17" width="16" height="7" rx="1.6" fill="var(--side)" opacity=".85"/></svg>`;
  return h("div", { class: "brand" }, mark, h("span", { class: "brand-name" }, (state.status.app_name || "Typedeck").toLowerCase()));
}

function drawNav() {
  const view = viewId();
  const item = ([id, icon]) => h("a", { class: "navitem", href: `#/${id}`, title: t("nav." + id), "aria-current": view === id ? "page" : undefined }, ic(icon, 18), h("span", { class: "navlabel" }, t("nav." + id)));
  clear(navEl).append(...NAV.map((n) => (n ? item(n) : h("span", { class: "navsep", role: "separator" }))), h("span", { class: "grow" }), item(["settings", "gear"]));
  titleEl.textContent = t("nav." + view);
}

function drawTools() {
  const s = state.save;
  saveEl.textContent = s === "idle" ? "" : t("save." + s);
  saveEl.className = `savestate ${s}`;
  saveEl.title = s === "error" ? state.saveError || "" : "";
  undoBtn.disabled = !canUndo();
  redoBtn.disabled = !canRedo();
}

function drawDevice() {
  const s = state.status, kb = state.keyboard;
  const soft = isSoftware();
  const on = s.connected && kb.info?.present;
  const name = soft ? t("capture.sys_keyboard") : on ? (kb.identity?.display || kb.info?.prod || t("info.unnamed")) : s.connected ? t("board.no_kbd") : t("board.off");
  const sub = soft ? (s.connected ? t("capture.soft_state") : osText("setup.soft_off")) : s.connected ? t("board.on") : t("info.no_board");
  const tone = !s.connected ? "bad" : on || soft ? "ok" : "warn";
  clear(deviceEl).append(h("button", { type: "button", class: "status", onclick: () => openKeyboardDrawer(), title: `${name}. ${sub}`, "aria-label": `${t("info.details")}: ${name}, ${sub}` },
    h("span", { class: `dot ${tone}` }),
    h("span", { class: "who" }, h("b", {}, name), h("small", {}, sub, state.live ? null : ` · ${t("live.off")}`))));
}

function route(force) {
  const id = viewId();
  if (!force && current?.id === id) return;
  current?.api?.unmount?.();
  state.view = id;
  clear(viewHost);
  current = { id, api: VIEWS[id](viewHost) || {} };
  drawNav();
  viewHost.scrollTop = 0;
}

function globalKeys(e) {
  const mod = e.metaKey || e.ctrlKey;
  if (mod && e.key.toLowerCase() === "k") { e.preventDefault(); if (!modalOpen()) openPalette(); return; }
  if (isTyping() || modalOpen()) return;
  if (mod && e.key.toLowerCase() === "z") { e.preventDefault(); e.shiftKey ? redo() : undo(); }
  else if (mod && e.key.toLowerCase() === "y") { e.preventDefault(); redo(); }
}

async function loadEvents() {
  try { const r = await api("/api/events?since=0"); state.events = r.events; state.lastId = Math.max(state.lastId, r.last_id); } catch {}
}

let kbTimer;
function refreshKeyboard(delay = 700) {
  clearTimeout(kbTimer);
  kbTimer = setTimeout(async () => {
    try {
      const [kb, st] = await Promise.all([api("/api/keyboard"), api("/api/status")]);
      setKeyboard(kb);
      Object.assign(state.status, st);
      notify("keyboard"); notify("status");
    } catch {}
  }, delay);
}

let cfgTimer;
function reloadConfig(delay = 150) {
  clearTimeout(cfgTimer);
  cfgTimer = setTimeout(async () => {
    if (isTyping() || modalOpen()) return reloadConfig(1000);
    try { adoptRemote(await api("/api/config")); } catch {}
  }, delay);
}

async function resync() {
  refreshKeyboard(0);
  if (isTyping()) return;
  try { adoptRemote(await api("/api/config")); } catch {}
}

function handle(ev) {
  state.events.push(ev);
  if (state.events.length > 400) state.events.shift();
  const s = state.status;
  switch (ev.kind) {
    case "board": s.connected = !!ev.connected; s.port = ev.port || null; if (!ev.connected) state.keyboard.info = null; notify("status"); refreshKeyboard(); break;
    case "keyboard": refreshKeyboard(); break;
    case "config": if (ev.name !== TAB) reloadConfig(); break;
    case "settings": if (ev.name === "key_history") { state.cfg.settings.key_history = !!ev.ok; notify("cfg", "remote"); } break;
    case "layer": s.layer = ev.layer; notify("layer"); break;
    case "front": s.front = ev.names || []; notify("status"); break;
    case "down": state.down.add(ev.key); notify("key", { key: ev.key, down: true }); break;
    case "up": state.down.delete(ev.key); notify("key", { key: ev.key, down: false }); break;
    case "watch": notify("watch", { key: ev.key }); break;
  }
  notify("event", ev);
}

function onPlainKey(key, down) {
  if (down) state.down.add(key); else state.down.delete(key);
  notify("key", { key, down });
}

function ingest(ev) {
  if (ev.id <= state.lastId) return;
  state.lastId = ev.id;
  handle(ev);
}

async function onLive(ev, helloId) {
  if (ev) return ingest(ev);
  if (helloId !== undefined && helloId < state.lastId) state.lastId = 0;
  try {
    const r = await api(`/api/events?since=${state.lastId}`);
    r.events.forEach(ingest);
    state.lastId = Math.max(state.lastId, r.last_id, helloId || 0);
  } catch {}
  for (const k of state.down) notify("key", { key: k, down: false });
  state.down.clear();
  resync();
}

let aborter = null;
async function eventsLoop() {
  for (;;) {
    if (isLive()) { await new Promise((r) => setTimeout(r, 2000)); continue; }
    if (document.hidden) { await new Promise((r) => document.addEventListener("visibilitychange", r, { once: true })); refreshKeyboard(); continue; }
    aborter = new AbortController();
    try {
      const r = await api(`/api/events?since=${state.lastId}&wait=25`, { signal: aborter.signal });
      r.events.forEach(ingest);
      state.lastId = Math.max(state.lastId, r.last_id);
    } catch (e) {
      if (e.name !== "AbortError") await new Promise((r) => setTimeout(r, 2500));
    }
  }
}
document.addEventListener("visibilitychange", () => { if (document.hidden) aborter?.abort(); else if (state.cfg) resync(); });

boot();
