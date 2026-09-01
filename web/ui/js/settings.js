import { h, ic, clear } from "./dom.js";
import { api } from "./api.js";
import { t, lang } from "./i18n.js";
import { toast } from "./toast.js";
import { toggle, seg, swatches, armed, select, textInput } from "./controls.js";
import { state, edit, replaceConfig, settings } from "./store.js";

const section = (title, sub, ...rows) => h("section", { class: "section" }, h("h2", {}, title), sub ? h("p", { class: "sub" }, sub) : null, h("div", {}, rows));
const setting = (what, control, sub) => h("div", { class: "setting" }, h("div", { class: "what" }, h("b", {}, what), sub ? h("small", {}, sub) : null), h("div", { class: "ctl" }, control));

function slider(value, min, max, step, unit, onInput) {
  const out = h("output", { class: "dim" }, `${value} ${unit}`);
  const el = h("input", { type: "range", min, max, step, value });
  el.addEventListener("input", () => { out.textContent = `${el.value} ${unit}`; onInput(Number(el.value)); });
  return h("div", {}, h("div", { class: "row", style: { justifyContent: "flex-end" } }, out), el);
}

export function settingsView(root) {
  const s = settings();
  const set = (path, v) => edit("s:" + path.join("."), (cfg) => { let o = cfg.settings; for (const k of path.slice(0, -1)) o = (o[k] ??= {}); o[path.at(-1)] = v; });
  const sw = (label, checked, path, sub) => setting(label, toggle("", checked, (v) => set(path, v)), sub);

  const backups = h("div", { class: "rowlist" });
  async function loadBackups() {
    try { state.backups = await api("/api/backups"); } catch { state.backups = []; }
    clear(backups);
    if (!state.backups.length) backups.append(h("div", { class: "empty-state" }, h("p", {}, t("backup.none"))));
    for (const b of state.backups.slice(0, 12)) {
      backups.append(h("div", { class: "li" }, h("span", { class: "grow" }, new Date(b.ts * 1000).toLocaleString(lang())), h("span", { class: "dim" }, t("backup.summary", { layers: b.layers, keys: b.keys })),
        h("button", { type: "button", class: "btn sm", onclick: async () => {
          try { const r = await api("/api/backups/restore", { method: "POST", body: { name: b.name } }); replaceConfig(r.config); toast(t("backup.restored")); loadBackups(); } catch (e) { toast(e.message, "bad"); }
        } }, ic("history", 15), t("backup.restore"))));
    }
  }

  const file = h("input", { type: "file", accept: "application/json,.json", hidden: true });
  file.addEventListener("change", async () => {
    const f = file.files[0]; if (!f) return;
    try { replaceConfig(JSON.parse(await f.text())); toast(t("io.imported")); } catch { toast(t("io.invalid"), "bad"); }
    file.value = "";
  });

  root.append(h("div", { class: "page" }, h("header", {}, h("h1", {}, t("settings.title")), h("p", {}, t("settings.sub"))),
    section(t("settings.look"), null,
      setting(t("settings.language"), seg([["es", "Español"], ["en", "English"]], s.language, (v) => { set(["language"], v); location.reload(); })),
      setting(t("settings.theme"), seg([["auto", t("theme.auto")], ["dark", t("theme.dark")], ["light", t("theme.light")]], s.theme, (v) => set(["theme"], v))),
      setting(t("settings.accent"), swatches(s.accent, (v) => { if (v) set(["accent"], v); }, { allowNone: false })),
      setting(t("settings.density"), seg([["comfortable", t("density.comfortable")], ["compact", t("density.compact")]], s.density, (v) => set(["density"], v)))),
    section(t("settings.input"), t("settings.input_sub"),
      setting(t("settings.input_mode"), select("", s.input, [["auto", t("input.auto")], ["hardware", t("input.hardware")], ["software", t("input.software")]], (v) => set(["input"], v)), t("settings.input_mode_sub")),
      setting(t("settings.typing_layout"), select("", s.typing_layout, [["auto", t("typing.auto")], ["es-iso", "Español (macOS)"], ["es-pc", "Español (Linux)"], ["us", "US"]], (v) => set(["typing_layout"], v)), t("settings.typing_layout_sub")),
      setting(t("gesture.hold"), slider(s.hold_ms, 200, 1500, 50, "ms", (v) => set(["hold_ms"], v)), t("settings.hold_sub")),
      setting(t("gesture.double"), slider(s.double_ms, 120, 800, 20, "ms", (v) => set(["double_ms"], v)), t("settings.double_sub")),
      setting(t("settings.volume_step"), slider(s.volume_step, 1, 25, 1, "%", (v) => set(["volume_step"], v)), t("settings.volume_step_sub")),
      sw(t("settings.auto_layer"), s.auto_layer, ["auto_layer"], t("settings.auto_layer_sub"))),
    section(t("settings.hud"), t("settings.hud_sub"),
      sw(t("hud.enabled"), s.hud.enabled, ["hud", "enabled"], t("hud.enabled_sub")),
      sw(t("hud.on_action"), s.hud.on_action, ["hud", "on_action"], t("hud.on_action_sub")),
      sw(t("hud.on_auto"), s.hud.on_auto, ["hud", "on_auto"], t("hud.on_auto_sub")),
      sw(t("hud.sound"), s.hud.sound, ["hud", "sound"]),
      setting(t("hud.position"), seg([["top", t("pos.top")], ["center", t("pos.center")], ["bottom", t("pos.bottom")]], s.hud.position, (v) => set(["hud", "position"], v))),
      setting(t("hud.seconds"), slider(s.hud.seconds, 0.6, 5, 0.2, "s", (v) => set(["hud", "seconds"], v))),
      setting(t("hud.test"), h("button", { type: "button", class: "btn", onclick: () => api("/api/hud", { method: "POST", body: { title: "Typedeck", subtitle: t("diag.hud_sub") } }) }, ic("hud", 16), t("hud.test")))),
    obsSection(s, set),
    section(t("settings.backups"), t("settings.backups_sub"),
      h("div", { class: "row wrap", style: { padding: "12px 0" } }, h("button", { type: "button", class: "btn sm", onclick: async () => { await api("/api/backups/create", { method: "POST", body: {} }); toast(t("backup.created")); loadBackups(); } }, ic("plus", 15), t("backup.create"))), backups),
    section(t("settings.io"), t("settings.io_sub"),
      h("div", { class: "row wrap", style: { padding: "14px 0" } },
        h("button", { type: "button", class: "btn", onclick: () => exportConfig() }, ic("download", 16), t("io.export")),
        h("button", { type: "button", class: "btn", onclick: () => file.click() }, ic("upload", 16), t("io.import")),
        armed("io.reset", () => resetConfig(), { icon: "trash" }), file))));
  loadBackups();
  return {};
}

export function exportConfig() {
  const copy = JSON.parse(JSON.stringify(state.cfg));
  if (copy.settings?.obs) copy.settings.obs.password = "";
  const blob = new Blob([JSON.stringify(copy, null, 2)], { type: "application/json" });
  const a = h("a", { href: URL.createObjectURL(blob), download: `typedeck-${new Date().toISOString().slice(0, 10)}.json` });
  a.click(); URL.revokeObjectURL(a.href);
}

function resetConfig() {
  const keep = state.cfg.settings;
  replaceConfig({ version: 3, settings: keep, global: { "53": { tap: { type: "layer", to: "next" }, label: "Capa", icon: "layers" } }, layers: [{ name: t("layer.default_name", { n: 1 }), color: "", icon: "", auto_apps: [], keys: {} }], keyboards: state.cfg.keyboards || {} });
  state.scope = 0; toast(t("io.reset_done")); location.hash = "#/keys";
}

function obsSection(settings, set) {
  const s = { ...settings, obs: { host: "127.0.0.1", port: 4455, password: "", ...(settings.obs || {}) } };
  const out = h("div", { class: "console", hidden: true });
  const field = (label, value, path, o = {}) => h("div", { class: "setting" }, h("div", { class: "what" }, h("b", {}, label), o.sub ? h("small", {}, o.sub) : null),
    h("div", { class: "ctl" }, textInput("", value, (v) => set(path, o.number ? Number(v) || 4455 : v), { type: o.type || "text", placeholder: o.ph || "" })));
  const test = async () => {
    out.hidden = false; out.className = "console"; out.textContent = t("test.running");
    try {
      const r = await api("/api/obs/probe", { method: "POST", body: state.cfg.settings.obs });
      if (!r.ok) { out.className = "console bad"; out.textContent = r.error; return; }
      state.obsInfo = r.info; out.className = "console ok";
      out.replaceChildren(h("div", { class: "tag" }, ic("check", 14), t("obs.connected", { v: r.info.version || "" })), t("obs.found", { s: r.info.scenes.length, i: r.info.inputs.length }));
    } catch (e) { out.className = "console bad"; out.textContent = e.message; }
  };
  return h("section", { class: "section" }, h("h2", {}, t("settings.obs")), h("p", { class: "sub" }, t("settings.obs_sub")),
    h("div", {}, field(t("obs.host"), s.obs.host, ["obs", "host"], { ph: "127.0.0.1" }), field(t("obs.port"), s.obs.port, ["obs", "port"], { type: "number", number: true, ph: "4455" }),
      field(t("obs.password"), s.obs.password, ["obs", "password"], { type: "password", sub: t("obs.password_sub") })),
    h("div", { class: "row wrap", style: { padding: "14px 0" } }, h("button", { type: "button", class: "btn", onclick: test }, ic("play", 15), t("obs.test"))), out);
}
