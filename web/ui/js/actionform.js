import { h, ic } from "./dom.js";
import { t } from "./i18n.js";
import { api } from "./api.js";
import { toast } from "./toast.js";
import { field, textInput, area, select, toggle } from "./controls.js";
import { state, edit, newAction } from "./store.js";
import { TYPE_ICON } from "./keyboard.js";
import { createRecorder, describeStep } from "./recorder.js";
import { holdEditing } from "./live.js";

export const TYPES = ["app", "url", "shell", "ssh", "http", "hotkey", "text", "sequence", "if", "media", "system", "history", "obs", "timer", "layer", "hud"];
const MEDIA = ["playpause", "next", "prev", "volup", "voldown", "mute"];
const OBS_CMDS = ["scene", "scene_next", "scene_prev", "stream", "stream_start", "stream_stop", "record", "record_pause", "mute", "replay_save", "virtualcam", "studio", "studio_transition"];
const CONDS = ["app", "layer", "time", "os", "obs_stream", "obs_record", "clipboard"];
const MAX_STEPS = 100;
const SYSTEM = ["screenshot", "lock", "screensaver", "sleepdisplay", "sleep", "darkmode", "caffeinate"];

let dl;
function datalist() {
  dl = h("datalist", { id: "apps" }, state.appList.map((n) => h("option", { value: n })));
  return dl;
}

export function actionFields(a, rerender, { gesture = "tap", inSeq = false } = {}) {
  const list = [];
  const upd = (tag, fn) => edit(tag, fn);
  const text = (label, key, o) => textInput(label, a[key], (v) => upd("f:" + key, () => { a[key] = v; }), o);
  switch (a.type) {
    case "app": {
      const missing = state.appSet.size && a.app && !state.appSet.has(a.app.toLowerCase());
      list.push(textInput(t("f.app"), a.app, (v) => upd("f:app", () => { a.app = v; }), { list: "apps", placeholder: t("f.app_ph"), warn: missing ? t("warn.app_missing") : "" }),
        datalist(), select(t("f.mode"), a.mode, [["open", t("mode.open")], ["toggle", t("mode.toggle")], ["quit", t("mode.quit")]], (v) => upd("", () => { a.mode = v; }), inSeq ? "" : t("mode.help")));
      break;
    }
    case "url": list.push(text(t("f.url"), "url", { placeholder: "https://...", help: t("f.url_help") })); break;
    case "shell": list.push(area(t("f.cmd"), a.cmd, (v) => upd("f:cmd", () => { a.cmd = v; }), { placeholder: "open ~/Downloads", help: t("f.cmd_help") }),
      toggle(t("f.show_output"), a.show_output, (v) => upd("", () => { a.show_output = v; }), t("f.show_output_sub"))); break;
    case "ssh": list.push(text(t("f.host"), "host", { placeholder: "mi-servidor", help: t("f.host_help") }),
      area(t("f.cmd"), a.cmd, (v) => upd("f:cmd", () => { a.cmd = v; }), { placeholder: "uptime -p" }),
      toggle(t("f.show_output"), a.show_output !== false, (v) => upd("", () => { a.show_output = v; }), t("f.show_output_sub"))); break;
    case "http":
      list.push(select(t("f.method"), a.method || "GET", ["GET", "POST", "PUT", "DELETE"].map((m) => [m, m]), (v) => { upd("", () => { a.method = v; }); rerender(); }), text(t("f.url"), "url", { placeholder: "https://..." }));
      if ((a.method || "GET") !== "GET") list.push(area(t("f.body"), a.body, (v) => upd("f:body", () => { a.body = v; }), { placeholder: "{\"key\": \"value\"}" }));
      break;
    case "hotkey": list.push(text(t("f.keys"), "keys", { placeholder: "cmd+shift+4", help: t("f.keys_help") })); break;
    case "text": list.push(area(t("f.text"), a.text, (v) => upd("f:text", () => { a.text = v; }), { placeholder: t("f.text_ph"), help: t("f.text_help") })); break;
    case "media": list.push(select(t("f.control"), a.cmd, MEDIA.map((c) => [c, t("media." + c)]), (v) => upd("", () => { a.cmd = v; }), t("media.help"))); break;
    case "system": list.push(select(t("f.control"), a.cmd, SYSTEM.map((c) => [c, t("system." + c)]), (v) => upd("", () => { a.cmd = v; }), t("system.help"))); break;
    case "history": list.push(select(t("f.control"), a.cmd || "toggle", ["toggle", "on", "off"].map((c) => [c, t("hist.act_" + c)]), (v) => upd("", () => { a.cmd = v; }), t("hist.act_help"))); break;
    case "obs": {
      const info = state.obsInfo;
      list.push(select(t("f.control"), a.cmd, OBS_CMDS.map((c) => [c, t("obs." + c)]), (v) => { upd("", () => { a.cmd = v; if (v !== "scene" && v !== "mute") delete a.target; }); rerender(); }, t("obs.help")));
      if (a.cmd === "scene" || a.cmd === "mute") {
        const kind = a.cmd === "scene" ? "scenes" : "inputs";
        list.push(textInput(t(a.cmd === "scene" ? "obs.scene_name" : "obs.input_name"), a.target, (v) => upd("f:target", () => { a.target = v; }),
          { list: "obs-" + kind, placeholder: a.cmd === "scene" ? "Juego" : "Mic/Aux", help: info ? "" : t("obs.no_list") }),
          h("datalist", { id: "obs-" + kind }, (info?.[kind] || []).map((n) => h("option", { value: n }))));
      }
      if (!info) loadObsInfo(rerender);
      break;
    }
    case "timer": list.push(h("div", { class: "two" }, textInput(t("f.minutes"), a.minutes, (v) => upd("f:min", () => { a.minutes = Math.max(0.1, parseFloat(v) || 1); }), { type: "number", min: 0.1, step: 1 }), text(t("f.timer_label"), "label", { placeholder: "Pomodoro" }))); break;
    case "layer": {
      const opts = [["next", t("layer.next")], ["prev", t("layer.prev")], ...state.cfg.layers.map((l, i) => [i, l.name])];
      list.push(select(t("f.layer"), a.to, opts, (v) => upd("", () => { a.to = v === "next" || v === "prev" ? v : Number(v); })));
      if (gesture === "hold" && !inSeq) list.push(toggle(t("f.momentary"), !!a.momentary, (v) => upd("", () => { if (v) a.momentary = true; else delete a.momentary; }), t("f.momentary_sub")));
      break;
    }
    case "hud": list.push(text(t("f.hud_text"), "text", { placeholder: t("f.hud_ph") })); break;
    case "wait": list.push(textInput(t("f.ms"), a.ms, (v) => upd("f:ms", () => { a.ms = Math.max(0, parseInt(v, 10) || 0); }), { type: "number", min: 0, step: 100 })); break;
    case "sequence": list.push(sequenceEditor(a, rerender)); break;
    case "if": list.push(ifEditor(a, rerender)); break;
  }
  return list;
}

function stepList(steps, rerender, types) {
  const box = h("div", { class: "steps" });
  steps.forEach((s, i) => {
    const ctl = (name, icon, fn, disabled) => h("button", { type: "button", class: "btn sm icon", "aria-label": t("seq." + name), title: t("seq." + name), disabled, onclick: fn }, ic(icon, 15));
    box.append(h("div", { class: `step ${s.type === "wait" ? "wait" : ""}` },
      h("div", { class: "row", style: { gap: "6px" } },
        h("span", { class: "idx" }, String(i + 1)),
        h("div", { class: "grow" }, select("", s.type, types.map((x) => [x, t("type." + x)]), (v) => { edit("", () => { steps[i] = newAction(v); }); rerender(); })),
        ctl("up", "arrow-up", () => { edit("", () => { [steps[i - 1], steps[i]] = [steps[i], steps[i - 1]]; }); rerender(); }, i === 0),
        ctl("down", "arrow-down", () => { edit("", () => { [steps[i + 1], steps[i]] = [steps[i], steps[i + 1]]; }); rerender(); }, i === steps.length - 1),
        ctl("dup", "copy", () => { if (steps.length >= MAX_STEPS) return toast(t("seq.max"), "bad"); edit("", () => { steps.splice(i + 1, 0, JSON.parse(JSON.stringify(s))); }); rerender(); }),
        ctl("del", "trash", () => { edit("", () => { steps.splice(i, 1); }); rerender(); })),
      ...actionFields(s, rerender, { inSeq: true })));
  });
  const add = select("", "", [["", t("seq.add")], ...types.map((x) => [x, t("type." + x)])], (v) => { if (!v) return; if (steps.length >= MAX_STEPS) return toast(t("seq.max"), "bad"); edit("", () => { steps.push(newAction(v)); }); rerender(); });
  return h("div", { class: "stack", style: { gap: "10px" } }, box, add);
}

function sequenceEditor(a, rerender) {
  a.steps ||= [];
  const types = [...TYPES.filter((x) => x !== "sequence"), "wait"];
  return h("div", { class: "stack", style: { gap: "10px" } }, h("p", { class: "help" }, t("seq.help")), recorderBar(a, rerender), stepList(a.steps, rerender, types));
}

function recorderBar(a, rerender) {
  const host = h("div", { class: "recorder" });
  let rec = null, onKey = null, timings = true;

  function draw() {
    host.replaceChildren();
    if (!rec) {
      host.append(h("button", { type: "button", class: "btn sm", onclick: start }, ic("record", 15), t("rec.start")),
        h("span", { class: "faint" }, t("rec.hint")));
      return;
    }
    const last = rec.steps.slice(-6);
    host.append(h("div", { class: "rec-live", role: "status" },
      h("div", { class: "row", style: { gap: "8px" } }, h("span", { class: "rec-dot" }), h("b", {}, t("rec.running")), h("span", { class: "faint" }, t("rec.count", { n: rec.steps.length }))),
      h("div", { class: "rec-steps" }, last.length ? last.map((s) => h("span", { class: `tag ${s.type}` }, describeStep(s))) : h("span", { class: "faint" }, t("rec.empty"))),
      rec.full ? h("p", { class: "help bad" }, t("seq.max")) : null,
      toggle(t("rec.timings"), timings, (v) => { timings = v; rec.setTimings(v); }, t("rec.timings_sub")),
      h("div", { class: "row", style: { gap: "8px" } },
        h("button", { type: "button", class: "btn sm primary", onclick: () => stop(true) }, ic("check", 15), t("rec.save")),
        h("button", { type: "button", class: "btn sm", onclick: () => stop(false) }, t("common.cancel")))));
  }

  function start() {
    rec = createRecorder({ timings, max: Math.max(0, MAX_STEPS - a.steps.length) });
    holdEditing(true);
    onKey = (e) => {
      if (e.target.closest?.(".recorder button, .recorder input, .recorder label")) return;
      e.preventDefault(); e.stopPropagation();
      if (rec.key(e)) draw();
    };
    window.addEventListener("keydown", onKey, true);
    draw();
  }

  function stop(save) {
    window.removeEventListener("keydown", onKey, true);
    holdEditing(false);
    const steps = rec.steps;
    rec = null;
    if (save && steps.length) { edit("", () => { a.steps.push(...steps); }); toast(t("rec.saved", { n: steps.length })); rerender(); return; }
    draw();
  }

  draw();
  return host;
}

function ifEditor(a, rerender) {
  a.steps ||= []; a.else ||= [];
  const types = TYPES.filter((x) => x !== "sequence" && x !== "if").concat("wait");
  const list = [];
  list.push(select(t("cond.when"), a.cond, CONDS.map((c) => [c, t("cond." + c)]), (v) => { edit("", () => { a.cond = v; a.target = v === "os" ? "darwin" : ""; }); rerender(); }));
  if (a.cond === "os") list.push(select(t("cond.value"), a.target || "darwin", [["darwin", "macOS"], ["windows", "Windows"], ["linux", "Linux"]], (v) => edit("", () => { a.target = v; })));
  else if (!a.cond.startsWith("obs_")) list.push(textInput(t("cond.value"), a.target, (v) => edit("f:target", () => { a.target = v; }), { placeholder: t("cond.ph." + a.cond), help: t("cond.help." + a.cond) }));
  else list.push(h("p", { class: "help" }, t("cond.obs_help")));
  list.push(toggle(t("cond.invert"), !!a.not, (v) => edit("", () => { if (v) a.not = true; else delete a.not; }), t("cond.invert_sub")));
  list.push(h("div", { class: "group" }, h("h3", {}, t("cond.then")), stepList(a.steps, rerender, types)));
  list.push(h("div", { class: "group" }, h("h3", {}, t("cond.else")), stepList(a.else, rerender, types)));
  return h("div", { class: "stack", style: { gap: "12px" } }, ...list);
}

let obsAsked = false;

async function loadObsInfo(rerender) {
  if (obsAsked) return;
  obsAsked = true;
  try {
    const r = await api("/api/obs/probe", { method: "POST", body: {} });
    if (r.ok) { state.obsInfo = r.info; rerender(); }
  } catch {  }
  setTimeout(() => { obsAsked = false; }, 30000);
}

export const typeOptions = () => [["", t("type.none")], ...TYPES.map((x) => [x, t("type." + x)])];
export { TYPE_ICON };
