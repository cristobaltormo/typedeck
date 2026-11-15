import { h, ic, clear, isTyping } from "./dom.js";
import { api } from "./api.js";
import { t } from "./i18n.js";
import { toast, guarded } from "./toast.js";
import { keyboard, neighbor } from "./keyboard.js";
import { layout as getLayout, legendLang, keyName } from "./layouts.js";
import { field, textInput, select, toggle, swatches, iconPicker, armed } from "./controls.js";
import { actionFields, actionExtras, typeOptions, wantsConfirm, stopRecording } from "./actionform.js";
import { openKeyboardDrawer, layoutLabel } from "./kbinfo.js";
import { isSoftware, boardMissing, offerCard, hookDownCard, useWithoutBoard, useBoard, osText } from "./capture.js";
import { exportLayer, importPack } from "./share.js";
import { modalOpen } from "./modal.js";
import { clone, hex, usageOf } from "./util.js";
import { state, edit, keysOf, keyDef, ensureKey, pruneKey, newAction, describe, firstAction, layerCount, layoutId, addLayer, moveLayer, removeLayer, canAddLayer } from "./store.js";

const GESTURES = ["tap", "hold", "double"];

export function workView(root) {
  const layerbar = h("div", { class: "layerbar", role: "tablist", "aria-label": t("layer.title") });
  const kbHost = h("div", { class: "kb-host" });
  const infoLine = h("div", { class: "kbinfo" });
  const bannerHost = h("div", { class: "banner-host" });
  const stage = h("div", { class: "stage" }, bannerHost, kbHost, infoLine);
  const dock = h("aside", { class: "dock", "aria-label": t("insp.aria") });
  root.append(h("div", { class: "work" }, h("div", { class: "canvas" }, layerbar, stage), dock));
  let kb = null, gesture = "tap";

  const accent = () => (state.scope === "global" ? state.cfg.settings.accent : state.cfg.layers[state.scope]?.color || state.cfg.settings.accent);
  const scopeName = () => (state.scope === "global" ? t("strip.global") : state.cfg.layers[state.scope].name);

  function renderLayers() {
    clear(layerbar);
    state.cfg.layers.forEach((l, i) => {
      const sel = state.scope === i;
      const tab = h("button", { type: "button", role: "tab", class: "layer", "aria-selected": String(sel), style: l.color ? { "--lc": l.color } : {},
        title: state.status.layer === i ? t("strip.live") : undefined,
        onclick: () => { if (sel) state.panel = state.panel === "layer" ? "key" : "layer"; else { state.scope = i; state.panel = "key"; } renderAll(); } },
        h("span", { class: "n" }, String(i + 1)), h("span", { class: "name" }, l.name), state.status.layer === i ? h("span", { class: "live", "aria-label": t("strip.live") }) : null);
      layerbar.append(tab);
    });
    if (canAddLayer()) layerbar.append(h("button", { type: "button", class: "layer add", onclick: () => { addLayer(); state.panel = "layer"; renderAll(); }, title: t("strip.add_tip") }, ic("plus", 14), t("strip.add")));
    layerbar.append(h("span", { class: "sep" }),
      h("button", { type: "button", role: "tab", class: "layer", "aria-selected": String(state.scope === "global"), title: t("strip.global_tip"), onclick: () => { state.scope = "global"; state.panel = "key"; renderAll(); } }, ic("globe", 14), h("span", { class: "name" }, t("strip.global"))));
    if (state.scope !== "global") layerbar.append(h("button", { type: "button", class: "tool layer-edit", "aria-pressed": String(state.panel === "layer"), title: t("layer.edit"), "aria-label": t("layer.edit"), onclick: () => { state.panel = state.panel === "layer" ? "key" : "layer"; renderDock(); renderLayers(); } }, ic("sliders", 15)));
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
    const refocus = document.activeElement?.closest?.(".kb") === kb?.el;
    kb?.destroy();
    kb = keyboard(layoutId(), legendLang(), getKd, { selected: usageOf(state.selKey), onSelect: selectKey, onMove: moveKey, color: accent(), maxU: 64, minU: 22, observe: stage,
      fitHeight: () => stage.clientHeight - infoLine.offsetHeight - bannerHost.offsetHeight - 64 });
    clear(kbHost).append(kb.el);
    if (refocus) kb.caps.get(usageOf(state.selKey))?.el.focus({ preventScroll: true });
  }

  function moveKey(from, to, copy) {
    const a = hex(from), b = hex(to);
    const keys = keysOf();
    if (!keys[a]) return;
    const swap = !copy && !!keys[b];
    edit("", () => {
      const src = clone(keys[a]), dst = keys[b] ? clone(keys[b]) : null;
      keys[b] = src;
      if (copy) return;
      if (dst) keys[a] = dst; else delete keys[a];
    });
    state.selKey = b; state.panel = "key";
    renderKeyboard(); renderDock();
    toast(t(copy ? "key.copied_to" : swap ? "key.swapped" : "key.moved", { from: keyName(from), to: keyName(to) }));
  }

  function selectKey(usage) {
    if (state.selKey === hex(usage) && state.panel === "key") return;
    state.selKey = hex(usage); state.panel = "key";
    kb?.select(usage);
    renderDock(); renderLayers();
  }

  const bannerSig = () => { const st = state.status; return [st.connected, !!state.keyboard.info?.present, st.board_error, st.capture, boardMissing(), totalMacros() === 0].join("|"); };
  const totalMacros = () => state.cfg.layers.reduce((n, l) => n + Object.keys(l.keys).length, 0) + Object.keys(state.cfg.global || {}).length;
  let lastSig = "";
  function renderBanner() {
    const sig = bannerSig();
    if (sig === lastSig) return;
    lastSig = sig;
    clear(bannerHost);
    const st = state.status, soft = isSoftware(), boardOk = !!st.connected, kbOk = boardOk && !!state.keyboard.info?.present;
    const step = (done, text, action) => h("li", { class: `step-row ${done ? "done" : ""}` }, h("span", { class: "tick" }, done ? ic("check", 12) : null), h("span", { class: "grow" }, text), action);
    if (totalMacros() === 0) {
      const pack = step(false, t("setup.pack"), h("a", { class: "btn sm primary", href: "#/gallery" }, t("setup.pack_cta")));
      const offer = boardMissing() ? h("button", { type: "button", class: "btn sm", onclick: useWithoutBoard }, t("capture.use_without")) : null;
      const steps = soft ? [step(boardOk, boardOk ? t("setup.soft") : osText("setup.soft_off"), boardOk ? null : h("button", { type: "button", class: "btn sm", onclick: useBoard }, t("capture.use_board"))), pack]
        : [step(boardOk, boardOk ? t("setup.board_ok") : t(st.board_error === "outdated_firmware" ? "info.fw_old" : "setup.board"), offer),
          step(kbOk, kbOk ? t("setup.kbd_ok") : t("setup.kbd"), kbOk ? h("button", { type: "button", class: "btn sm ghost", onclick: () => openKeyboardDrawer() }, t("info.details")) : null), pack];
      bannerHost.append(h("section", { class: "setup", "aria-labelledby": "setup-title" }, h("div", {}, h("h2", { id: "setup-title" }, t("setup.title")), h("p", { class: "dim" }, t(soft ? "setup.sub_soft" : "setup.sub"))), h("ol", {}, steps)));
    } else if (soft) {
      if (!boardOk) bannerHost.append(hookDownCard());
    } else if (!boardOk && boardMissing()) {
      bannerHost.append(offerCard());
    } else if (!boardOk || !kbOk) {
      bannerHost.append(h("div", { class: "banner", role: "status" }, ic("alert", 16), h("p", {}, t(!boardOk ? "setup.warn_board" : "setup.warn_kbd")),
        boardOk ? h("button", { type: "button", class: "btn sm", onclick: guarded(async () => { await api("/api/board", { method: "POST", body: { cmd: "reboot" } }); toast(t("setup.rebooting")); }) }, t("setup.reboot")) : null));
    }
  }

  function renderInfo() {
    clear(infoLine);
    const kbd = state.keyboard, info = kbd.info, st = state.status;
    const soft = isSoftware();
    if (!st.connected) infoLine.append(h("span", {}, soft ? osText("setup.soft_off") : t(st.board_error === "outdated_firmware" ? "info.fw_old" : "info.no_board")));
    else if (!info?.present) infoLine.append(h("span", {}, t("info.no_keyboard")));
    else {
      const name = soft ? t("capture.sys_keyboard") : kbd.identity?.display || [info.mfr, info.prod].filter(Boolean).join(" ") || t("info.unnamed");
      const sure = kbd.layout ? kbd.layout.confident || kbd.layout.source === "user" : true;
      infoLine.append(...[h("b", {}, name), h("span", {}, layoutLabel(layoutId())), sure ? null : h("span", { class: "unsure" }, t("info.unconfirmed")),
        h("span", {}, `${getLayout(layoutId()).keys.length} ${t("info.keys")}`)].filter(Boolean));
    }
    infoLine.append(h("button", { type: "button", class: "btn sm ghost", onclick: () => openKeyboardDrawer() }, t("info.details")),
      toggle(t("note.follow"), state.follow, async (v) => {
        state.follow = v;
        try { await api("/api/learn", { method: "POST", body: { on: v } }); } catch (e) { toast(e.message, "bad"); state.follow = false; renderInfo(); }
      }, null, t("note.follow")));
    infoLine.lastChild.title = t("note.follow_help");
  }

  function renderDock() {
    stopRecording();
    const scroll = dock.scrollTop;
    clear(dock);
    dock.append(state.panel === "layer" && state.scope !== "global" ? layerDock() : keyDock());
    dock.scrollTop = scroll;
  }

  function keyDock() {
    const id = state.selKey, usage = usageOf(id);
    const k = getLayout(layoutId(), legendLang()).keys.find((x) => x.usage === usage);
    const name = keyName(usage);
    const kd = keyDef(id);
    const tool = (label, icon, on, disabled, cls = "") => h("button", { type: "button", class: `tool ${cls}`, title: label, "aria-label": label, disabled, onclick: on }, ic(icon, 16));

    const head = h("header", { class: "dock-head" },
      h("span", { class: "cap-mini", "aria-hidden": "true" }, k?.legend.icon ? ic(k.legend.icon, 14) : (k?.legend.main || id).slice(0, 6) || " "),
      h("div", { class: "keyname" }, h("h2", {}, t("key.title", { key: name })), h("span", {}, scopeName())),
      h("div", { class: "acts" },
        tool(t("key.copy"), "copy", () => { state.clip = clone(kd); toast(t("key.copied")); renderDock(); }, !kd),
        tool(t("key.paste"), "paste", () => { edit("", () => { keysOf()[id] = clone(state.clip); }); renderKeyboard(); renderDock(); }, !state.clip),
        tool(t("key.clear"), "trash", () => { edit("", () => { delete keysOf()[id]; }); renderDock(); }, !kd, "danger")));

    const inherited = state.scope !== "global" && !kd && state.cfg.global[id];
    if (inherited) {
      const g = state.cfg.global[id];
      const rows = GESTURES.filter((x) => g[x]).map((x) => h("div", { class: "inh-row" }, h("span", { class: "k" }, t("gesture." + x)), h("span", { class: "v" }, t("type." + g[x].type), h("small", {}, describe(g[x])))));
      return h("div", {}, head, h("div", { class: "dock-body" },
        h("div", { class: "notice" }, ic("globe", 16), h("p", {}, t("key.inherited")),
          h("div", { class: "btns" },
            h("button", { type: "button", class: "btn sm", onclick: () => { state.scope = "global"; renderAll(); } }, t("key.edit_global")),
            h("button", { type: "button", class: "btn sm", onclick: () => { edit("", () => { keysOf()[id] = clone(state.cfg.global[id]); }); renderDock(); } }, t("key.override")))),
        h("section", { class: "group" }, h("h3", {}, t("key.inherited_title")), h("div", { class: "inh" }, rows))));
    }
    const tabs = h("div", { class: "gtabs", role: "tablist", "aria-label": t("sec.action") }, GESTURES.map((g) => h("button", { type: "button", role: "tab", class: "gtab", dataset: { g }, "aria-selected": String(gesture === g),
      onclick: () => { gesture = g; renderDock(); dock.querySelector(`.gtab[data-g="${g}"]`)?.focus(); } }, t("gesture." + g), kd?.[g] ? h("i", { class: "on", "aria-label": t("gesture.set") }) : null)));
    return h("div", {}, head, h("div", { class: "dock-body" }, tabs, gesturePane(gesture, kd, id), lookGroup(kd, id)));
  }

  function gesturePane(g, kd, id) {
    const a = kd?.[g];
    const help = g === "tap" && !a && (kd?.hold || kd?.double) ? t("gesture.tap_pass") : t("gesture." + g + "_help", { ms: g === "hold" ? state.cfg.settings.hold_ms : state.cfg.settings.double_ms });
    const pane = h("div", { class: "pane", role: "tabpanel" });
    pane.append(select("", a?.type || "", typeOptions(), (v) => {
      edit("", () => {
        const k = ensureKey(id);
        if (!v) { delete k[g]; pruneKey(id); } else { const prev = k[g], next = newAction(v); k[g] = { ...next, ...(prev?.confirm && wantsConfirm(next) ? { confirm: true } : {}) }; }
      });
      renderKeyboard(); renderDock();
    }, a ? "" : help, t("sec.action")));
    if (a) {
      const lonely = state.scope !== "global" && !state.cfg.global[id] ? { name: state.cfg.layers[state.scope].name, apply: () => makeGlobal(id) } : null;
      pane.append(...actionFields(a, () => renderDock(), { gesture: g, lonely }), ...actionExtras(a));
    }
    return pane;
  }

  function makeGlobal(id) {
    edit("", (cfg) => { cfg.global[id] = clone(cfg.layers[state.scope].keys[id]); delete cfg.layers[state.scope].keys[id]; });
    state.scope = "global";
    renderAll();
    toast(t("layer.made_global"));
  }

  function lookGroup(kd, id) {
    const first = firstAction(kd);
    const set = (tag, key, v) => edit(tag, () => { const k = ensureKey(id); if (v) k[key] = v; else delete k[key]; pruneKey(id); });
    return h("section", { class: "group" }, h("h3", {}, t("sec.look")),
      textInput(t("look.label"), kd?.label || "", (v) => set("label", "label", v), { max: 40, placeholder: first ? describe(first) : "" }),
      field(t("look.color"), swatches(kd?.color || null, (v) => set("color", "color", v))),
      field(t("look.icon"), iconPicker(kd?.icon || null, (v) => set("", "icon", v))));
  }

  function layerDock() {
    const l = state.cfg.layers[state.scope], i = state.scope;
    const head = h("header", { class: "dock-head" }, h("span", { class: "cap-mini", "aria-hidden": "true" }, String(i + 1)), h("div", { class: "keyname" }, h("h2", {}, l.name), h("span", {}, t("layer.title"))),
      h("div", { class: "acts" }, h("button", { type: "button", class: "btn sm", onclick: () => { state.panel = "key"; renderDock(); renderLayers(); } }, t("common.done"))));
    const apps = h("div", { class: "stack tight" });
    const drawApps = () => {
      clear(apps);
      const list = l.auto_apps || [];
      const inp = h("input", { type: "text", placeholder: t("layer.auto_add"), spellcheck: false, "aria-label": t("layer.auto_add") });
      inp.setAttribute("list", "apps");
      const add = () => { const v = inp.value.trim(); if (v && !list.includes(v)) { edit("", () => { (l.auto_apps ||= []).push(v); }); drawApps(); apps.querySelector("input")?.focus(); } };
      inp.addEventListener("keydown", (e) => { if (e.key === "Enter") add(); });
      inp.addEventListener("change", add);
      apps.append(list.length ? h("ul", { class: "taglist" }, list.map((n, k) => h("li", { class: "tag" }, n,
        h("button", { type: "button", class: "tag-x", "aria-label": `${t("common.remove")} ${n}`, onclick: () => { edit("", () => { l.auto_apps.splice(k, 1); }); drawApps(); } }, ic("x", 12))))) : null,
        inp, h("p", { class: "help" }, state.cfg.settings.auto_layer ? t("layer.auto_help") : t("layer.auto_off")));
    };
    drawApps();
    const move = (d) => { moveLayer(i, i + d); renderAll(); };
    return h("div", {}, head, h("div", { class: "dock-body" },
      h("div", { class: "pane" },
        textInput(t("layer.name"), l.name, (v) => { edit("lname", () => { l.name = v || "-"; }); renderLayers(); }, { max: 30 }),
        field(t("look.color"), swatches(l.color || null, (v) => { edit("lcolor", () => { l.color = v || ""; }); })),
        field(t("look.icon"), iconPicker(l.icon || null, (v) => { edit("", () => { l.icon = v || ""; }); }))),
      h("section", { class: "group" }, h("h3", {}, t("layer.auto")), apps),
      h("section", { class: "group" }, h("h3", {}, t("layer.manage")),
        h("div", { class: "row wrap" },
          h("button", { type: "button", class: "btn sm", disabled: i === 0, onclick: () => move(-1) }, ic("arrow-left", 14), t("layer.left")),
          h("button", { type: "button", class: "btn sm", disabled: i === layerCount() - 1, onclick: () => move(1) }, ic("arrow-right", 14), t("layer.right")),
          h("button", { type: "button", class: "btn sm", disabled: !canAddLayer(), onclick: () => { const cp = clone(l); cp.name = t("layer.copy_of", { name: l.name }); addLayer(cp, i + 1); state.panel = "layer"; renderAll(); } }, ic("copy", 14), t("layer.duplicate")),
          h("button", { type: "button", class: "btn sm", onclick: () => exportLayer(i) }, ic("download", 14), t("share.export_layer")),
          h("button", { type: "button", class: "btn sm", onclick: importPack }, ic("upload", 14), t("share.import_btn"))),
        layerCount() > 1 ? h("div", { class: "row" }, armed("layer.delete", () => { removeLayer(i); state.panel = "key"; renderAll(); }, { small: true, icon: "trash" })) : null)));
  }

  function renderAll() { renderLayers(); renderKeyboard(); renderInfo(); renderDock(); }
  renderAll();

  const ARROWS = { ArrowUp: [0, -1], ArrowDown: [0, 1], ArrowLeft: [-1, 0], ArrowRight: [1, 0] };
  const onKey = (e) => {
    if (state.view !== "keys" || modalOpen() || isTyping() || e.metaKey || e.ctrlKey || e.altKey) return;
    const focus = document.activeElement;
    if (focus && focus !== document.body && !focus.closest(".kb")) return;
    if (ARROWS[e.key] && kb) { e.preventDefault(); const next = neighbor(kb.layout, usageOf(state.selKey), ...ARROWS[e.key]); selectKey(next); if (focus?.closest(".kb")) kb.caps.get(next)?.el.focus(); return; }
    const id = state.selKey;
    if ((e.key === "Delete" || e.key === "Backspace") && keyDef(id)) { e.preventDefault(); edit("", () => { delete keysOf()[id]; }); renderDock(); toast(t("key.cleared", { key: keyName(usageOf(id)) })); }
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
      stopRecording();
      document.removeEventListener("keydown", onKey);
      kb?.destroy();
      if (state.follow) { state.follow = false; api("/api/learn", { method: "POST", body: { on: false } }).catch(() => {}); }
    },
  };
}
