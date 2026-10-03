import { h, ic, clear } from "./dom.js";
import { api } from "./api.js";
import { t } from "./i18n.js";
import { state, subscribe, notify, applySettings, undo, redo, canUndo, canRedo } from "./store.js";
import { workView } from "./workbench.js";
import { galleryView } from "./gallery.js";
import { activityView } from "./activity.js";
import { sheetView } from "./sheet.js";
import { diagView } from "./diag.js";
import { compatView } from "./compat.js";
import { settingsView } from "./settings.js";
import { openPalette } from "./palette.js";
import { openKeyboardDrawer } from "./kbinfo.js";
import { startLive, isLive } from "./live.js";

const VIEWS = { keys: workView, gallery: galleryView, activity: activityView, sheet: sheetView, compat: compatView, diag: diagView, settings: settingsView };
const NAV = [["keys", "keyboard"], ["gallery", "grid"], null, ["activity", "activity"], ["sheet", "list"], ["compat", "layers"], ["diag", "cpu"]];
let current = null, viewHost, navEl, rightEl, titleEl, deviceEl;

async function boot() {
  const root = document.getElementById("app");
  try {
    const [cfg, apps, packs, status, kb, layouts, system] = await Promise.all([api("/api/config"), api("/api/apps"), api("/api/packs"), api("/api/status"), api("/api/keyboard"), api("/api/layouts"), api("/api/system").catch(() => null)]);
    state.cfg = cfg; state.appList = apps; state.appSet = new Set(apps.map((a) => a.toLowerCase()));
    state.packs = packs; Object.assign(state.status, status); state.lastId = status.last_id || 0;
    state.layouts = Object.fromEntries(layouts.map((l) => [l.id, l])); state.system = system;
    state.keyboard = { connected: kb.connected, info: kb.info || null, prefs: kb.prefs || null, identity: kb.identity || null, layout: kb.layout || null };
  } catch (e) {
    root.append(h("div", { class: "page" }, h("h1", {}, "Typedeck"), h("p", { class: "dim" }, e.message)));
    return;
  }
  applySettings();
  document.title = state.status.app_name || "Typedeck";
  navEl = h("nav", { class: "nav", "aria-label": "Typedeck" });
  deviceEl = h("div", { class: "device" });
  titleEl = h("h1", { class: "pagetitle" });
  rightEl = h("div", { class: "right" });
  viewHost = h("div", { class: "view" });
  root.append(h("aside", { class: "side" }, brand(), navEl, deviceEl),
    h("div", { class: "content" }, h("header", { class: "top" }, titleEl, rightEl), h("main", {}, viewHost)));
  drawTabs(); drawRight();
  window.addEventListener("hashchange", route);
  document.addEventListener("keydown", globalKeys);
  subscribe((topic, detail) => {
    if (topic === "palette") openPalette();
    if (topic === "cfg" || topic === "save" || topic === "status" || topic === "live") drawRight();
    current?.api?.on?.(topic, detail);
  });
  route();
  history_();
  const q = location.search;
  if (q.includes("scene=")) { (await import("./scenes.js")).run(new URLSearchParams(q).get("scene")); return; }
  if (q.includes("selftest")) { startLive(onLive); (await import("./selftest.js")).run(); return; }
  if (!q.includes("nolive")) { startLive(onLive); eventsLoop(); }
}

function brand() {
  const mark = h("span", { class: "ic" });
  mark.innerHTML = `<svg width="24" height="24" viewBox="0 0 32 32" aria-hidden="true"><rect x="2" y="2" width="28" height="28" rx="7" fill="var(--accent)"/><rect x="8" y="7" width="7" height="7" rx="1.6" fill="#fff"/><rect x="17" y="7" width="7" height="7" rx="1.6" fill="#fff" opacity=".55"/><rect x="8" y="17" width="16" height="7" rx="1.6" fill="#fff" opacity=".85"/></svg>`;
  return h("div", { class: "brand" }, mark, h("span", {}, (state.status.app_name || "Typedeck").toLowerCase()));
}

function drawTabs() {
  const view = location.hash.replace("#/", "") || "keys";
  const item = ([id, icon]) => h("button", { type: "button", class: "navitem", title: t("nav." + id), "aria-current": view === id ? "page" : undefined, onclick: () => { location.hash = `#/${id}`; } }, ic(icon, 18), h("span", {}, t("nav." + id)));
  clear(navEl).append(...NAV.map((n) => (n ? item(n) : h("hr", { class: "navsep" }))), h("span", { class: "grow" }), item(["settings", "gear"]));
  titleEl.textContent = t("nav." + (VIEWS[view] ? view : "keys"));
}

function drawRight() {
  const s = state.status, kb = state.keyboard;
  clear(rightEl).append(
    h("span", { class: `savestate ${state.save === "error" ? "err" : ""} ${state.save === "idle" ? "" : "show"}`, title: state.saveError || "" }, t("save." + (state.save === "idle" ? "saved" : state.save))),
    h("button", { type: "button", class: "tool", disabled: !canUndo(), title: t("tool.undo") + " (Cmd Z)", "aria-label": t("tool.undo"), onclick: undo }, ic("undo", 17)),
    h("button", { type: "button", class: "tool", disabled: !canRedo(), title: t("tool.redo") + " (Cmd Shift Z)", "aria-label": t("tool.redo"), onclick: redo }, ic("redo", 17)),
    h("button", { type: "button", class: "tool find", title: t("tool.palette"), onclick: () => notify("palette") }, ic("search", 15), h("span", { class: "dim" }, t("tool.palette")), h("span", { class: "kbd" }, "Cmd K")));
  const on = s.connected && kb.info?.present;
  clear(deviceEl).append(h("button", { type: "button", class: "status", onclick: () => openKeyboardDrawer(), title: t("info.details") },
    h("span", { class: `dot ${s.connected ? (on ? "on" : "") : "bad"}` }),
    h("span", { class: "who" }, h("b", {}, on ? (kb.identity?.display || kb.info?.prod || t("info.unnamed")) : s.connected ? t("board.no_kbd") : t("board.off")),
      h("small", {}, s.connected ? t("board.on") : t("info.no_board")), state.live ? h("small", { class: "live" }, t("live.on")) : null)));
}

function route() {
  const id = location.hash.replace("#/", "") || "keys";
  const factory = VIEWS[id] || VIEWS.keys;
  current?.api?.unmount?.();
  state.view = VIEWS[id] ? id : "keys";
  clear(viewHost);
  current = { id, api: factory(viewHost) };
  drawTabs();
  viewHost.scrollTop = 0;
}

function globalKeys(e) {
  const mod = e.metaKey || e.ctrlKey;
  if (mod && e.key.toLowerCase() === "k") { e.preventDefault(); openPalette(); return; }
  if (["INPUT", "TEXTAREA", "SELECT"].includes(document.activeElement?.tagName)) return;
  if (mod && e.key.toLowerCase() === "z") { e.preventDefault(); e.shiftKey ? redo() : undo(); }
}

async function history_() {
  try { const r = await api("/api/events?since=0"); state.events = r.events; state.lastId = r.last_id; } catch {  }
}

let kbTimer;
function refreshKeyboard() {
  clearTimeout(kbTimer);
  kbTimer = setTimeout(async () => {
    try {
      const [kb, st] = await Promise.all([api("/api/keyboard"), api("/api/status")]);
      state.keyboard = { connected: kb.connected, info: kb.info || null, prefs: kb.prefs || null, identity: kb.identity || null, layout: kb.layout || null };
      Object.assign(state.status, st);
      notify("keyboard"); notify("status");
    } catch {  }
  }, 700);
}

function handle(ev) {
  state.events.push(ev);
  if (state.events.length > 400) state.events.shift();
  const s = state.status;
  switch (ev.kind) {
    case "board": s.connected = !!ev.connected; s.port = ev.port || null; if (!ev.connected) { state.keyboard.info = null; } refreshKeyboard(); break;
    case "keyboard": refreshKeyboard(); break;
    case "layer": s.layer = ev.layer; notify("layer"); break;
    case "front": s.front = ev.names || []; notify("status"); break;
    case "down": state.down.add(ev.key); notify("key", { key: ev.key, down: true }); break;
    case "up": state.down.delete(ev.key); notify("key", { key: ev.key, down: false }); break;
    case "watch": notify("watch", { key: ev.key }); break;
  }
  notify("event", ev);
}

function ingest(ev) {
  if (ev.id <= state.lastId) return;
  state.lastId = ev.id;
  handle(ev);
}

async function onLive(ev, lastId) {
  if (ev) return ingest(ev);
  try {
    const r = await api(`/api/events?since=${state.lastId}`);
    r.events.forEach(ingest);
    state.lastId = Math.max(state.lastId, r.last_id, lastId || 0);
  } catch {  }
  refreshKeyboard();
}

let aborter = null;
async function eventsLoop() {
  while (true) {
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
document.addEventListener("visibilitychange", () => { if (document.hidden) aborter?.abort(); });

boot();
