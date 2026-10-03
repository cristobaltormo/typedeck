import { h, ic, clear } from "./dom.js";
import { api } from "./api.js";
import { t } from "./i18n.js";
import { toast } from "./toast.js";
import { keyboard, neighbor, hex, TYPE_ICON } from "./keyboard.js";
import { layout as getLayout, legendLang } from "./layouts.js";
import { field, textInput, select, toggle, swatches, iconPicker, armed } from "./controls.js";
import { actionFields, typeOptions } from "./actionform.js";
import { openKeyboardDrawer, layoutLabel } from "./kbinfo.js";
import { exportLayer, importPack } from "./share.js";
import { state, notify, edit, undo, redo, canUndo, canRedo, keysOf, keyDef, ensureKey, pruneKey, newAction, describe, layerCount, layoutId } from "./store.js";

const GESTURES = ["tap", "hold", "double"];
const clone = (o) => JSON.parse(JSON.stringify(o));
const usageOf = (id) => parseInt(id, 16);

export function workView(root) {
  const layerbar = h("div", { class: "layerbar", role: "tablist" });
  const kbHost = h("div", { class: "grid" });
  const infoLine = h("div", { class: "kbinfo" });
  const bannerHost = h("div");
  const stage = h("div", { class: "stage" }, bannerHost, kbHost, infoLine);
  const dock = h("div", { class: "dock" });
  root.append(h("div", { class: "work" }, h("div", { class: "canvas" }, layerbar, stage), dock));
  let kb = null, gesture = "tap";

  const accent = () => (state.scope === "global" ? state.cfg.settings.accent : state.cfg.layers[state.scope].color || state.cfg.settings.accent);

  function renderLayers() {
    clear(layerbar);
    state.cfg.layers.forEach((l, i) => {
      const live = state.status.layer === i;
      layerbar.append(h("button", { type: "button", role: "tab", class: `layer ${live ? "live" : ""}`, "aria-selected": String(state.scope === i), style: l.color ? { "--lc": l.color } : {}, title: live ? t("strip.live") : "",
        onclick: () => {
          if (state.scope === i) state.panel = state.panel === "layer" ? "key" : "layer"; else { state.scope = i; state.panel = "key"; }
          renderAll();
        } }, h("span", { class: "n" }, String(i + 1)), l.name));
    });
    if (layerCount() < 9) layerbar.append(h("button", { type: "button", class: "layer add", onclick: addLayer, "aria-label": t("strip.add") }, ic("plus", 15), t("strip.add")));
    layerbar.append(h("span", { class: "sep" }),
      h("button", { type: "button", class: "layer", "aria-selected": String(state.scope === "global"), title: t("strip.global_tip"), onclick: () => { state.scope = "global"; state.panel = "key"; renderAll(); } }, ic("globe", 15), t("strip.global")));
  }

  function addLayer() {
    edit("", (cfg) => { cfg.layers.push({ name: t("layer.default_name", { n: cfg.layers.length + 1 }), color: "", icon: "", auto_apps: [], keys: {} }); });
    state.scope = layerCount() - 1; state.panel = "layer";
    renderAll();
  }

  function getKd(usage) {
    const id = hex(usage);
    if (state.scope === "global") return { kd: state.cfg.global[id] };
    const own = state.cfg.layers[state.scope].keys[id];
    const inh = state.cfg.global[id];
    return { kd: own || inh, inherited: !own && !!inh };
  }

  function renderKeyboard() {
    renderBanner();
    kb?.destroy();
    kb = keyboard(layoutId(), legendLang(), getKd, { selected: usageOf(state.selKey), onSelect: selectKey, onMove: moveKey, color: accent(), maxU: 60, fitHeight: () => stage.clientHeight - infoLine.offsetHeight - bannerHost.offsetHeight - 70 });
    clear(kbHost).append(kb.el);
  }

  function moveKey(from, to, copy) {
    const a = hex(from), b = hex(to);
    const keys = keysOf();
    if (!keys[a]) return;
    edit("", () => {
      const src = clone(keys[a]), dst = keys[b] ? clone(keys[b]) : null;
      keys[b] = src;
      if (copy) return;
      if (dst) keys[a] = dst; else delete keys[a];
    });
    state.selKey = b; state.panel = "key";
    renderKeyboard(); renderDock();
    toast(t(copy ? "key.copied_to" : keys[a] ? "key.swapped" : "key.moved", { from: keyName(from), to: keyName(to) }));
  }

  function keyName(usage) {
    const k = getLayout(layoutId(), legendLang()).keys.find((x) => x.usage === usage);
    return k ? (k.legend.icon ? t("key.arrow") + " " + t("arrow." + k.legend.icon) : k.legend.main) : hex(usage);
  }

  function selectKey(usage) { state.selKey = hex(usage); state.panel = "key"; renderKeyboard(); renderDock(); }

  const bannerSig = () => { const st = state.status; return [st.connected, !!state.keyboard.info?.present, st.board_error].join("|"); };
  let lastSig = bannerSig();
  function renderBanner() {
    clear(bannerHost);
    lastSig = bannerSig();
    const st = state.status, boardOk = !!st.connected, kbOk = boardOk && !!state.keyboard.info?.present;
    const total = state.cfg.layers.reduce((n, l) => n + Object.keys(l.keys).length, 0);
    const step = (done, text, action) => h("li", { class: `step-row ${done ? "done" : ""}` }, h("span", { class: "tick" }, done ? ic("check", 13) : null), h("span", { class: "grow" }, text), action);
    if (total === 0) {
      bannerHost.append(h("div", { class: "setup" }, h("div", {}, h("h3", {}, t("setup.title")), h("p", { class: "dim" }, t("setup.sub"))),
        h("ol", {}, step(boardOk, boardOk ? t("setup.board_ok") : t(st.board_error === "outdated_firmware" ? "info.fw_old" : "setup.board")),
          step(kbOk, kbOk ? t("setup.kbd_ok") : t("setup.kbd"), kbOk ? h("button", { type: "button", class: "txtbtn", onclick: () => openKeyboardDrawer() }, t("info.details")) : null),
          step(false, t("setup.pack"), h("button", { type: "button", class: "btn sm primary", onclick: () => { location.hash = "#/gallery"; } }, t("setup.pack_cta"))))));
    } else if (!boardOk || !kbOk) {
      bannerHost.append(h("div", { class: "banner" }, ic("alert", 18), h("p", {}, t(!boardOk ? "setup.warn_board" : "setup.warn_kbd")),
        boardOk ? h("button", { type: "button", class: "btn sm", onclick: async () => { try { await api("/api/board", { method: "POST", body: { cmd: "reboot" } }); toast(t("setup.rebooting")); } catch (e) { toast(e.message, "bad"); } } }, t("setup.reboot")) : null));
    }
  }

  function renderInfo() {
    clear(infoLine);
    const kbd = state.keyboard, info = kbd.info, st = state.status;
    if (!st.connected) infoLine.append(h("span", {}, t(st.board_error === "outdated_firmware" ? "info.fw_old" : "info.no_board")));
    else if (!info?.present) infoLine.append(h("span", {}, t("info.no_keyboard")));
    else {
      const name = kbd.identity?.display || [info.mfr, info.prod].filter(Boolean).join(" ") || t("info.unnamed");
      const sure = kbd.layout ? kbd.layout.confident || kbd.layout.source === "user" : true;
      infoLine.append(...[h("b", {}, name), h("span", {}, layoutLabel(layoutId())), sure ? null : h("span", { class: "unsure" }, t("info.unconfirmed")),
        h("span", {}, `${getLayout(layoutId()).keys.length} ${t("info.keys")}`), info.power_ma ? h("span", {}, `${info.power_ma} mA`) : null].filter(Boolean));
    }
    infoLine.append(h("button", { type: "button", onclick: () => openKeyboardDrawer() }, t("info.details")),
      toggle(t("note.follow"), state.follow, async (v) => {
        state.follow = v;
        try { await api("/api/learn", { method: "POST", body: { on: v } }); } catch (e) { toast(e.message, "bad"); state.follow = false; renderInfo(); }
      }));
  }

  function renderDock() {
    clear(dock);
    dock.append(state.panel === "layer" && state.scope !== "global" ? layerDock() : keyDock());
  }

  function keyDock() {
    const id = state.selKey, usage = usageOf(id);
    const L = getLayout(layoutId(), legendLang());
    const k = L.keys.find((x) => x.usage === usage);
    const name = k ? (k.legend.icon ? t("key.arrow") + " " + t("arrow." + k.legend.icon) : k.legend.main || id) : id;
    const kd = keyDef(id);
    const scopeName = state.scope === "global" ? t("strip.global") : state.cfg.layers[state.scope].name;
    const hasAction = kd && (kd.tap || kd.hold || kd.double);
    const tool = (label, icon, on, disabled, cls = "") => h("button", { type: "button", class: `tool ${cls}`, title: label, "aria-label": label, disabled, onclick: on }, ic(icon, 16));

    const head = h("div", { class: "dock-head" },
      h("span", { class: "cap-mini" }, k?.legend.icon ? ic(k.legend.icon, 15) : name),
      h("div", { class: "keyname" }, h("span", { class: "t" }, t("key.title", { key: name })), h("span", { class: "s" }, scopeName)),
      h("div", { class: "acts" },
        tool(t("key.copy"), "copy", () => { state.clip = clone(kd); toast(t("key.copied")); renderDock(); }, !kd),
        tool(t("key.paste"), "paste", () => { edit("", () => { keysOf()[id] = clone(state.clip); }); renderDock(); }, !state.clip),
        tool(t("key.clear"), "trash", () => { edit("", () => { delete keysOf()[id]; }); state.testResult = null; renderDock(); }, !kd, "danger")));

    const tabs = h("div", { class: "gtabs", role: "tablist" }, GESTURES.map((g) => h("button", { type: "button", role: "tab", class: "gtab", dataset: { g }, "aria-selected": String(gesture === g),
      onclick: () => { gesture = g; renderDock(); } }, t("gesture." + g), kd?.[g] ? h("i", { class: "on" }) : null)));
    const inherited = state.scope !== "global" && !kd && state.cfg.global[id];
    const notice = inherited ? h("div", { class: "notice" }, ic("globe", 17), h("p", {}, t("key.inherited")),
      h("div", { class: "btns" },
        h("button", { type: "button", class: "btn sm", onclick: () => { state.scope = "global"; renderAll(); } }, t("key.edit_global")),
        h("button", { type: "button", class: "btn sm primary", onclick: () => { edit("", () => { keysOf()[id] = clone(state.cfg.global[id]); }); renderDock(); } }, t("key.override")))) : null;
    return h("div", {}, head, notice, h("div", { class: "dock-body" }, tabs, gesturePane(gesture, kd, id), lookGroup(kd, id)));
  }

  function gesturePane(g, kd, id) {
    const a = kd?.[g];
    const help = g === "tap" && !a && (kd?.hold || kd?.double) ? t("gesture.tap_pass") : t("gesture." + g + "_help", { ms: g === "hold" ? state.cfg.settings.hold_ms : state.cfg.settings.double_ms });
    const pane = h("div", { class: "pane", role: "tabpanel" });
    if (!a) pane.append(h("p", { class: "hint" }, help));
    const sel = select("", a?.type || "", typeOptions(), (v) => {
      edit("", () => {
        const k = ensureKey(id);
        if (!v) { delete k[g]; pruneKey(id); } else { const prev = k[g]; k[g] = { ...newAction(v), ...(prev?.confirm ? { confirm: true } : {}) }; }
      });
      state.testResult = null; renderKeyboard(); renderDock();
    });
    pane.append(sel);
    if (a) {
      pane.append(...actionFields(a, () => renderDock(), { gesture: g }));
      pane.append(toggle(t("act.confirm"), !!a.confirm, (v) => edit("confirm", () => { if (v) a.confirm = true; else delete a.confirm; }), t("act.confirm_sub")));
      pane.append(testBox(a));
    }
    return pane;
  }

  function testBox(a) {
    const out = h("div");
    const run = h("button", { type: "button", class: "btn primary sm", onclick: async () => {
      clear(out).append(h("div", { class: "console" }, t("test.running")));
      let r;
      try { r = await api("/api/test", { method: "POST", body: a }); } catch (e) { r = { ok: false, output: e.message, ms: 0 }; }
      clear(out).append(h("div", { class: `console ${r.ok ? "ok" : "bad"}` }, h("div", { class: "tag" }, ic(r.ok ? "check" : "alert", 14), r.ok ? t("test.ok") : t("test.fail"), h("span", { class: "faint" }, ` ${r.ms} ms`)), r.output || t("test.no_output")));
    } }, ic("play", 14), t("test.run"));
    return h("div", { class: "stack", style: { gap: "8px" } }, h("div", {}, run), out);
  }

  function lookGroup(kd, id) {
    const first = kd && (kd.tap || kd.hold || kd.double);
    return h("div", { class: "group" }, h("h3", {}, t("sec.look")),
      textInput(t("look.label"), kd?.label || "", (v) => { edit("label", () => { const k = ensureKey(id); if (v) k.label = v; else delete k.label; pruneKey(id); }); }, { max: 40, placeholder: first ? describe(first) : "" }),
      field(t("look.color"), swatches(kd?.color || null, (v) => { edit("color", () => { const k = ensureKey(id); if (v) k.color = v; else delete k.color; pruneKey(id); }); })),
      field(t("look.icon"), iconPicker(kd?.icon || null, (v) => { edit("", () => { const k = ensureKey(id); if (v) k.icon = v; else delete k.icon; pruneKey(id); }); })));
  }

  function layerDock() {
    const l = state.cfg.layers[state.scope], i = state.scope;
    const head = h("div", { class: "dock-head" }, h("span", { class: "cap-mini" }, String(i + 1)), h("div", { class: "keyname" }, h("span", { class: "t" }, l.name), h("span", { class: "s" }, t("layer.title"))),
      h("div", { class: "acts" }, h("button", { type: "button", class: "btn sm", onclick: () => { state.panel = "key"; renderDock(); } }, t("common.done"))));
    const apps = h("div", { class: "stack", style: { gap: "8px" } });
    const drawApps = () => {
      clear(apps);
      const wrap = h("div", { class: "row wrap", style: { gap: "6px" } }, (l.auto_apps || []).map((n, k) => h("span", { class: "tag row", style: { gap: "6px" } }, n,
        h("button", { type: "button", class: "tool", style: { height: "18px", padding: "0" }, "aria-label": t("common.remove"), onclick: () => { edit("", () => { l.auto_apps.splice(k, 1); }); drawApps(); } }, ic("x", 12)))));
      const inp = h("input", { type: "text", placeholder: t("layer.auto_add"), spellcheck: false });
      inp.setAttribute("list", "apps");
      const add = () => { const v = inp.value.trim(); if (v && !(l.auto_apps || []).includes(v)) { edit("", () => { (l.auto_apps ||= []).push(v); }); drawApps(); } };
      inp.addEventListener("keydown", (e) => { if (e.key === "Enter") add(); });
      inp.addEventListener("change", add);
      apps.append(wrap, inp, h("datalist", { id: "apps" }, state.appList.map((n) => h("option", { value: n }))), h("p", { class: "help" }, state.cfg.settings.auto_layer ? t("layer.auto_help") : t("layer.auto_off")));
    };
    drawApps();
    const move = (d) => { edit("", (c) => { const j = i + d; [c.layers[i], c.layers[j]] = [c.layers[j], c.layers[i]]; }); state.scope = i + d; renderAll(); };
    const cols = h("div", { class: "dock-body" },
      h("div", { class: "pane" },
        textInput(t("layer.name"), l.name, (v) => { edit("lname", () => { l.name = v || "-"; }); renderLayers(); }, { max: 30 }),
        field(t("look.color"), swatches(l.color || null, (v) => { edit("lcolor", () => { l.color = v || ""; }); renderLayers(); renderKeyboard(); })),
        field(t("look.icon"), iconPicker(l.icon || null, (v) => { edit("", () => { l.icon = v || ""; }); }))),
      h("div", { class: "group" }, h("h3", {}, t("layer.auto")), apps),
      h("div", { class: "group" }, h("h3", {}, t("layer.manage")),
        h("div", { class: "row wrap" },
          h("button", { type: "button", class: "btn sm", disabled: i === 0, onclick: () => move(-1) }, ic("arrow-left", 15), t("layer.left")),
          h("button", { type: "button", class: "btn sm", disabled: i === layerCount() - 1, onclick: () => move(1) }, ic("arrow-right", 15), t("layer.right")),
          h("button", { type: "button", class: "btn sm", disabled: layerCount() >= 9, onclick: () => { edit("", (c) => { const cp = clone(l); cp.name = t("layer.copy_of", { name: l.name }); c.layers.splice(i + 1, 0, cp); }); state.scope = i + 1; renderAll(); } }, ic("copy", 15), t("layer.duplicate")),
          h("button", { type: "button", class: "btn sm", onclick: () => exportLayer(i) }, ic("download", 15), t("share.export_layer")),
          h("button", { type: "button", class: "btn sm", onclick: importPack }, ic("upload", 15), t("share.import_btn")),
          layerCount() > 1 && armed("layer.delete", () => { edit("", (c) => { c.layers.splice(i, 1); }); state.scope = Math.max(0, i - 1); state.panel = "key"; renderAll(); }, { small: true, icon: "trash" }))));
    return h("div", {}, head, cols);
  }

  function renderAll() { renderLayers(); renderKeyboard(); renderInfo(); renderDock(); }
  renderAll();
  const ro = new ResizeObserver(() => { if (kb) { kb.destroy(); renderKeyboard(); } });
  ro.observe(stage);

  const onKey = (e) => {
    if (state.view !== "keys" || document.querySelector(".scrim, .drawer-scrim")) return;
    if (["INPUT", "TEXTAREA", "SELECT"].includes(document.activeElement?.tagName)) return;
    const dirs = { ArrowUp: [0, -1], ArrowDown: [0, 1], ArrowLeft: [-1, 0], ArrowRight: [1, 0] };
    if (dirs[e.key] && !e.metaKey && !e.ctrlKey && kb) { e.preventDefault(); selectKey(neighbor(kb.layout, usageOf(state.selKey), ...dirs[e.key])); return; }
    const id = state.selKey;
    if ((e.key === "Delete" || e.key === "Backspace") && keyDef(id)) { e.preventDefault(); edit("", () => { delete keysOf()[id]; }); renderDock(); return; }
  };
  document.addEventListener("keydown", onKey);

  return {
    on(topic, detail) {
      if (topic === "cfg") { renderLayers(); renderKeyboard(); renderInfo(); if (detail !== "edit") renderDock(); }
      else if (topic === "layer") renderLayers();
      else if (topic === "status" || topic === "keyboard") { renderInfo(); if (topic === "keyboard" || bannerSig() !== lastSig) renderKeyboard(); }
      else if (topic === "key") kb?.caps.get(usageOf(detail.key))?.el.classList.toggle("down", detail.down);
      else if (topic === "watch" && state.follow) { const u = usageOf(detail.key); if (kb?.caps.has(u) && !kb.caps.get(u).k.mod) selectKey(u); }
    },
    unmount() {
      document.removeEventListener("keydown", onKey);
      ro.disconnect();
      kb?.destroy();
      if (state.follow) { state.follow = false; api("/api/learn", { method: "POST", body: { on: false } }).catch(() => {}); }
    },
  };
}
