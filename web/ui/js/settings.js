import { h, ic, clear, pageHead, emptyState } from "./dom.js";
import { api } from "./api.js";
import { t, lang } from "./i18n.js";
import { toast, guarded } from "./toast.js";
import { toggle, seg, swatches, armed, select, textInput } from "./controls.js";
import { state, edit, replaceConfig, settings, notify } from "./store.js";
import { isSoftware, useBoard, useWithoutBoard } from "./capture.js";
import { download, pickFile, inspectFile } from "./share.js";
import { looksLikeConfig, blankLayer } from "./layerops.js";
import { clone } from "./util.js";

const section = (title, sub, ...rows) => h("section", { class: "section" }, h("h2", {}, title), sub ? h("p", { class: "sub" }, sub) : null, h("div", { class: "settings" }, rows));
const setting = (what, control, sub) => h("div", { class: "setting" }, h("div", { class: "what" }, h("span", { class: "name" }, what), sub ? h("small", {}, sub) : null), h("div", { class: "ctl" }, control));

function slider(label, value, min, max, step, unit, onInput) {
  const out = h("output", {}, `${value} ${unit}`);
  const el = h("input", { type: "range", min, max, step, value, "aria-label": label });
  el.addEventListener("input", () => { out.textContent = `${el.value} ${unit}`; onInput(Number(el.value)); });
  return h("div", { class: "slider" }, el, out);
}

export function settingsView(root) {
  const page = h("div", { class: "page" });
  root.append(page);
  const set = (path, v) => edit("s:" + path.join("."), (cfg) => { let o = cfg.settings; for (const k of path.slice(0, -1)) o = (o[k] ??= {}); o[path.at(-1)] = v; });
  const backups = h("div", { class: "rowlist" });

  async function loadBackups() {
    try { state.backups = await api("/api/backups"); } catch { state.backups = []; }
    clear(backups);
    if (!state.backups.length) { backups.append(emptyState(null, t("backup.none"))); return; }
    for (const b of state.backups.slice(0, 12)) {
      backups.append(h("div", { class: "li" }, h("span", { class: "grow" }, new Date(b.ts * 1000).toLocaleString(lang())), h("span", { class: "dim" }, t("backup.summary", { layers: b.layers, keys: b.keys })),
        h("button", { type: "button", class: "btn sm", onclick: guarded(async () => {
          const r = await api("/api/backups/restore", { method: "POST", body: { name: b.name } });
          replaceConfig(r.config); toast(t("backup.restored")); loadBackups();
        }) }, ic("history", 14), t("backup.restore"))));
    }
  }

  function draw() {
    const s = settings();
    const sw = (label, checked, path, sub) => setting(label, toggle("", checked, (v) => set(path, v), null, label), sub);
    const sel = (label, value, opts, path) => select("", value, opts, (v) => set(path, v), null, label);
    clear(page).append(pageHead(t("settings.sub")),
      section(t("settings.look"), null,
        setting(t("settings.language"), seg([["es", "Español"], ["en", "English"]], s.language, (v) => { set(["language"], v); notify("lang"); }, t("settings.language"))),
        setting(t("settings.theme"), seg([["auto", t("theme.auto")], ["dark", t("theme.dark")], ["light", t("theme.light")]], s.theme, (v) => set(["theme"], v), t("settings.theme"))),
        setting(t("settings.accent"), swatches(s.accent, (v) => { if (v) set(["accent"], v); }, { allowNone: false })),
        setting(t("settings.density"), seg([["comfortable", t("density.comfortable")], ["compact", t("density.compact")]], s.density, (v) => set(["density"], v), t("settings.density")))),
      section(t("capture.title"), t("capture.sub"),
        setting(t("capture.mode"), seg([["board", t("capture.board")], ["software", t("capture.software")]], isSoftware() ? "software" : "board", async (v) => { await (v === "software" ? useWithoutBoard() : useBoard()); draw(); }, t("capture.mode")), t("capture.mode_sub"))),
      section(t("settings.input"), t("settings.input_sub"),
        isSoftware() ? setting(t("settings.input_mode"), h("span", { class: "dim" }, t("capture.input_note")))
          : setting(t("settings.input_mode"), sel(t("settings.input_mode"), s.input, [["auto", t("input.auto")], ["hardware", t("input.hardware")], ["software", t("input.software")]], ["input"]), t("settings.input_mode_sub")),
        setting(t("settings.typing_layout"), sel(t("settings.typing_layout"), s.typing_layout, [["auto", t("typing.auto")], ["es-iso", t("typing.es_mac")], ["es-win", t("typing.es_win")], ["es-pc", t("typing.es_linux")], ["us", t("typing.us")]], ["typing_layout"]), t("settings.typing_layout_sub")),
        setting(t("gesture.hold"), slider(t("gesture.hold"), s.hold_ms, 200, 1500, 50, "ms", (v) => set(["hold_ms"], v)), t("settings.hold_sub")),
        setting(t("gesture.double"), slider(t("gesture.double"), s.double_ms, 120, 800, 20, "ms", (v) => set(["double_ms"], v)), t("settings.double_sub")),
        setting(t("settings.volume_step"), slider(t("settings.volume_step"), s.volume_step, 1, 25, 1, "%", (v) => set(["volume_step"], v)), t("settings.volume_step_sub")),
        sw(t("settings.auto_layer"), s.auto_layer, ["auto_layer"], t("settings.auto_layer_sub"))),
      section(t("settings.hud"), t("settings.hud_sub"),
        sw(t("hud.enabled"), s.hud.enabled, ["hud", "enabled"], t("hud.enabled_sub")),
        sw(t("hud.on_action"), s.hud.on_action, ["hud", "on_action"], t("hud.on_action_sub")),
        sw(t("hud.on_auto"), s.hud.on_auto, ["hud", "on_auto"], t("hud.on_auto_sub")),
        sw(t("hud.sound"), s.hud.sound, ["hud", "sound"]),
        sw(t("settings.notify_kbd"), s.notify_keyboard !== false, ["notify_keyboard"], t("settings.notify_kbd_sub")),
        setting(t("hud.position"), seg([["top", t("pos.top")], ["center", t("pos.center")], ["bottom", t("pos.bottom")]], s.hud.position, (v) => set(["hud", "position"], v), t("hud.position"))),
        setting(t("hud.seconds"), slider(t("hud.seconds"), s.hud.seconds, 0.6, 5, 0.2, "s", (v) => set(["hud", "seconds"], v))),
        setting(t("hud.test"), h("button", { type: "button", class: "btn sm", onclick: guarded(() => api("/api/hud", { method: "POST", body: { title: "Typedeck", subtitle: t("diag.hud_sub") } })) }, ic("hud", 14), t("hud.test")))),
      obsSection(s, set),
      section(t("settings.backups"), t("settings.backups_sub"),
        h("div", { class: "section-actions" }, h("button", { type: "button", class: "btn sm", onclick: guarded(async () => { await api("/api/backups/create", { method: "POST", body: {} }); toast(t("backup.created")); loadBackups(); }) }, ic("plus", 14), t("backup.create"))),
        backups),
      section(t("settings.io"), t("settings.io_sub"),
        h("div", { class: "section-actions" },
          h("button", { type: "button", class: "btn sm", onclick: exportConfig }, ic("download", 14), t("io.export")),
          h("button", { type: "button", class: "btn sm", onclick: () => pickFile(importConfig) }, ic("upload", 14), t("io.import")),
          armed("io.reset", resetConfig, { small: true, icon: "trash" }))));
  }

  draw();
  loadBackups();
  return {
    on(topic, detail) {
      if (topic !== "cfg" || detail === "edit") return;
      const y = root.scrollTop;
      draw();
      root.scrollTop = y;
    },
  };
}

async function importConfig(f) {
  let c;
  try { c = JSON.parse(await f.text()); } catch { return toast(t("io.invalid"), "bad"); }
  if (c?.typedeck_macros) return inspectFile(f);
  if (!looksLikeConfig(c)) return toast(t("io.invalid"), "bad");
  replaceConfig({ ...c, global: c.global || {}, settings: { ...state.cfg.settings, ...c.settings } });
  state.scope = 0;
  toast(t("io.imported"));
}

export function exportConfig() {
  const copy = clone(state.cfg);
  if (copy.settings?.obs) copy.settings.obs.password = "";
  download(`typedeck-${new Date().toISOString().slice(0, 10)}.json`, copy);
}

function resetConfig() {
  replaceConfig({ version: state.cfg.version || 3, settings: state.cfg.settings, global: { "53": { tap: { type: "layer", to: "next" }, label: t("layer.key_label"), icon: "layers" } },
    layers: [blankLayer(t("layer.default_name", { n: 1 }))], keyboards: state.cfg.keyboards || {} });
  state.scope = 0; toast(t("io.reset_done")); location.hash = "#/keys";
}

function obsSection(settings, set) {
  const obs = { host: "127.0.0.1", port: 4455, password: "", ...(settings.obs || {}) };
  const out = h("div", { class: "console", hidden: true, "aria-live": "polite" });
  const row = (label, value, path, o = {}) => setting(label, textInput("", value, (v) => set(path, o.number ? Number(v) || 4455 : v), { type: o.type || "text", placeholder: o.ph || "", aria: label }), o.sub);
  const test = async () => {
    out.hidden = false; out.className = "console"; out.textContent = t("test.running");
    try {
      const r = await api("/api/obs/probe", { method: "POST", body: state.cfg.settings.obs || obs });
      if (!r.ok) { out.className = "console bad"; out.textContent = r.error; return; }
      state.obsInfo = r.info; out.className = "console ok";
      out.replaceChildren(h("div", { class: "console-head" }, ic("check", 14), t("obs.connected", { v: r.info.version || "" })), t("obs.found", { s: r.info.scenes.length, i: r.info.inputs.length }));
    } catch (e) { out.className = "console bad"; out.textContent = e.message; }
  };
  return section(t("settings.obs"), t("settings.obs_sub"),
    row(t("obs.host"), obs.host, ["obs", "host"], { ph: "127.0.0.1" }),
    row(t("obs.port"), obs.port, ["obs", "port"], { type: "number", number: true, ph: "4455" }),
    row(t("obs.password"), obs.password, ["obs", "password"], { type: "password", sub: t("obs.password_sub") }),
    h("div", { class: "section-actions" }, h("button", { type: "button", class: "btn sm", onclick: test }, ic("play", 14), t("obs.test"))), out);
}
