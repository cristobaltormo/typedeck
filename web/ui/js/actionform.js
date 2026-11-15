import { h, ic, clear } from "./dom.js";
import { t } from "./i18n.js";
import { api } from "./api.js";
import { toast } from "./toast.js";
import { textInput, area, select, toggle } from "./controls.js";
import { state, edit, newAction } from "./store.js";
import { createRecorder, describeStep } from "./recorder.js";
import { holdEditing } from "./live.js";
import { clone } from "./util.js";

export const TYPES = ["app", "url", "shell", "ssh", "http", "hotkey", "text", "sequence", "if", "media", "system", "history", "obs", "timer", "layer", "hud"];
const MEDIA = ["playpause", "next", "prev", "volup", "voldown", "mute"];
const OBS_CMDS = ["scene", "scene_next", "scene_prev", "stream", "stream_start", "stream_stop", "record", "record_pause", "mute", "replay_save", "virtualcam", "studio", "studio_transition"];
const CONDS = ["app", "layer", "time", "os", "obs_stream", "obs_record", "clipboard"];
const SYSTEM = ["screenshot", "lock", "screensaver", "sleepdisplay", "sleep", "darkmode", "caffeinate"];
const RISKY_SYSTEM = ["lock", "sleep", "sleepdisplay"];
export const MAX_STEPS = 100;
const DELAY_S = 4;

const osName = () => state.system?.os || "darwin";
const byOS = (m) => m[osName()] ?? m.darwin;
const URL_OK = /^([a-z][a-z0-9+.-]*:|\/|~|[A-Za-z]:\\|\.)/i;

function obsDatalist(kind) {
  const id = "obs-" + kind;
  let dl = document.getElementById(id);
  if (!dl) { dl = h("datalist", { id }); document.body.append(dl); }
  dl.replaceChildren(...(state.obsInfo?.[kind] || []).map((n) => h("option", { value: n })));
  return id;
}

export function actionFields(a, rerender, { gesture = "tap", inSeq = false, lonely = null } = {}) {
  const list = [];
  const upd = (tag, fn) => edit(tag, fn);
  const text = (label, key, o) => textInput(label, a[key], (v) => upd("f:" + key, () => { a[key] = v; }), o);
  switch (a.type) {
    case "app": {
      const missing = state.appSet.size && a.app && !state.appSet.has(a.app.toLowerCase());
      list.push(textInput(t("f.app"), a.app, (v) => upd("f:app", () => { a.app = v; }), { list: "apps", placeholder: t("f.app_ph"), warn: missing ? t("warn.app_missing") : "" }),
        select(t("f.mode"), a.mode, [["open", t("mode.open")], ["toggle", t("mode.toggle")], ["quit", t("mode.quit")]], (v) => {
          upd("", () => { a.mode = v; if (inSeq) return; if (v === "quit") a.confirm = true; else delete a.confirm; });
          rerender();
        }, t("mode." + (a.mode || "open") + "_help")));
      break;
    }
    case "url": list.push(text(t("f.url"), "url", { placeholder: "https://...", help: t("f.url_help"), warn: a.url && !URL_OK.test(a.url.trim()) ? t("warn.url_scheme") : "" })); break;
    case "shell": list.push(area(t("f.cmd"), a.cmd, (v) => upd("f:cmd", () => { a.cmd = v; }), { placeholder: byOS({ darwin: "open ~/Downloads", windows: "explorer %USERPROFILE%\\Downloads", linux: "xdg-open ~/Downloads" }), help: t("f.cmd_help") }),
      toggle(t("f.show_output"), a.show_output, (v) => upd("", () => { a.show_output = v; }), t("f.show_output_sub"))); break;
    case "ssh": list.push(text(t("f.host"), "host", { placeholder: "my-server", help: t("f.host_help") }),
      area(t("f.cmd"), a.cmd, (v) => upd("f:cmd", () => { a.cmd = v; }), { placeholder: "uptime -p" }),
      toggle(t("f.show_output"), a.show_output !== false, (v) => upd("", () => { a.show_output = v; }), t("f.show_output_sub"))); break;
    case "http":
      list.push(select(t("f.method"), a.method || "GET", ["GET", "POST", "PUT", "DELETE"].map((m) => [m, m]), (v) => { upd("", () => { a.method = v; }); rerender(); }), text(t("f.url"), "url", { placeholder: "https://..." }));
      if ((a.method || "GET") !== "GET") list.push(area(t("f.body"), a.body, (v) => upd("f:body", () => { a.body = v; }), { placeholder: "{\"key\": \"value\"}", help: t("f.body_help") }));
      break;
    case "hotkey": list.push(text(t("f.keys"), "keys", { placeholder: byOS({ darwin: "cmd+shift+4", windows: "ctrl+shift+s", linux: "ctrl+alt+t" }), help: t("f.keys_help." + osName()) })); break;
    case "text": list.push(area(t("f.text"), a.text, (v) => upd("f:text", () => { a.text = v; }), { placeholder: t("f.text_ph"), help: t("f.text_help") })); break;
    case "media": list.push(select(t("f.control"), a.cmd, MEDIA.map((c) => [c, t("media." + c)]), (v) => upd("", () => { a.cmd = v; }), t("media.help"))); break;
    case "system": list.push(select(t("f.control"), a.cmd, SYSTEM.map((c) => [c, t("system." + c)]), (v) => {
      upd("", () => { a.cmd = v; if (!inSeq) { if (RISKY_SYSTEM.includes(v)) a.confirm = true; else delete a.confirm; } });
      rerender();
    }, t("system.help." + a.cmd))); break;
    case "history": list.push(h("p", { class: `state-line ${state.cfg.settings.key_history ? "on" : ""}` }, t(state.cfg.settings.key_history ? "hist.now_on" : "hist.now_off")),
      select(t("f.control"), a.cmd || "toggle", ["toggle", "on", "off"].map((c) => [c, t("hist.act_" + c)]), (v) => upd("", () => { a.cmd = v; }), t("hist.act_help"))); break;
    case "obs": {
      list.push(select(t("f.control"), a.cmd, OBS_CMDS.map((c) => [c, t("obs." + c)]), (v) => { upd("", () => { a.cmd = v; if (v !== "scene" && v !== "mute") delete a.target; }); rerender(); }, t("obs.help")));
      if (a.cmd === "scene" || a.cmd === "mute") {
        const kind = a.cmd === "scene" ? "scenes" : "inputs";
        list.push(textInput(t(a.cmd === "scene" ? "obs.scene_name" : "obs.input_name"), a.target, (v) => upd("f:target", () => { a.target = v; }),
          { list: obsDatalist(kind), placeholder: a.cmd === "scene" ? "Game" : "Mic/Aux", help: state.obsInfo ? "" : t("obs.no_list") }));
      }
      if (!state.obsInfo) loadObsInfo(rerender);
      break;
    }
    case "timer": list.push(h("div", { class: "two" },
      textInput(t("f.minutes"), a.minutes, (v) => upd("f:min", () => { a.minutes = Math.max(0.1, parseFloat(v) || 1); }), { type: "number", min: 0.1, step: 1 }),
      text(t("f.timer_label"), "label", { placeholder: "Pomodoro" })),
      h("div", { class: "chips", role: "group", "aria-label": t("f.minutes") }, [5, 10, 25, 45, 60].map((m) => h("button", { type: "button", class: "chip", "aria-pressed": String(Number(a.minutes) === m), onclick: () => { upd("", () => { a.minutes = m; }); rerender(); } }, `${m} min`)))); break;
    case "layer": list.push(...layerFields(a, rerender, gesture !== "double" && !inSeq, lonely)); break;
    case "hud": list.push(text(t("f.hud_text"), "text", { placeholder: t("f.hud_ph"), help: t("f.hud_help") })); break;
    case "wait": list.push(textInput(t("f.ms"), a.ms, (v) => upd("f:ms", () => { a.ms = Math.max(0, parseInt(v, 10) || 0); }), { type: "number", min: 0, step: 100 })); break;
    case "sequence": list.push(sequenceEditor(a, rerender)); break;
    case "if": list.push(ifEditor(a, rerender)); break;
  }
  return list;
}

function layerFields(a, rerender, hold, lonely) {
  const list = [];
  const mode = a.momentary && hold ? "hold" : a.toggle ? "toggle" : "switch";
  const modes = [...(hold ? [["hold", t("layer.mode_hold")]] : []), ["switch", t("layer.mode_switch")], ["toggle", t("layer.mode_toggle")]];
  list.push(select(t("layer.mode"), mode, modes, (v) => {
    edit("", () => {
      delete a.momentary; delete a.toggle;
      if (v === "hold") a.momentary = true;
      if (v === "toggle") { a.toggle = true; if (typeof a.to !== "number") a.to = Math.min(1, state.cfg.layers.length - 1); }
    });
    rerender();
  }, t("layer.help_" + mode)));
  const opts = [...(mode === "toggle" ? [] : [["next", t("layer.next")], ["prev", t("layer.prev")]]), ...state.cfg.layers.map((l, i) => [i, `${i + 1}. ${l.name}`])];
  list.push(select(t(mode === "toggle" ? "layer.toggle_with" : "f.layer"), a.to, opts, (v) => { edit("", () => { a.to = v === "next" || v === "prev" ? v : Number(v); }); rerender(); },
    a.to === "next" || a.to === "prev" ? t("layer.help_" + a.to) : ""));
  if (lonely && mode !== "hold" && state.cfg.layers.length > 1) list.push(h("div", { class: "notice" }, ic("globe", 16), h("p", {}, t("layer.lonely", { layer: lonely.name })),
    h("button", { type: "button", class: "btn sm", onclick: lonely.apply }, t("layer.make_global"))));
  return list;
}

function stepList(steps, rerender, types) {
  const box = h("ol", { class: "steps" });
  steps.forEach((s, i) => {
    const ctl = (name, icon, fn, disabled) => h("button", { type: "button", class: "btn sm icon ghost", "aria-label": t("seq." + name), title: t("seq." + name), disabled, onclick: fn }, ic(icon, 14));
    box.append(h("li", { class: `step ${s.type === "wait" ? "wait" : ""}` },
      h("div", { class: "step-head" },
        h("span", { class: "idx" }, String(i + 1)),
        h("div", { class: "grow" }, select("", s.type, types.map((x) => [x, t("type." + x)]), (v) => { edit("", () => { steps[i] = newAction(v); }); rerender(); }, "", t("seq.type"))),
        ctl("up", "arrow-up", () => { edit("", () => { [steps[i - 1], steps[i]] = [steps[i], steps[i - 1]]; }); rerender(); }, i === 0),
        ctl("down", "arrow-down", () => { edit("", () => { [steps[i + 1], steps[i]] = [steps[i], steps[i + 1]]; }); rerender(); }, i === steps.length - 1),
        ctl("dup", "copy", () => { if (steps.length >= MAX_STEPS) return toast(t("seq.max"), "bad"); edit("", () => { steps.splice(i + 1, 0, clone(s)); }); rerender(); }),
        ctl("del", "trash", () => { edit("", () => { steps.splice(i, 1); }); rerender(); })),
      ...actionFields(s, rerender, { inSeq: true })));
  });
  const add = select("", "", [["", t("seq.add")], ...types.map((x) => [x, t("type." + x)])], (v) => {
    if (!v) return;
    if (steps.length >= MAX_STEPS) return toast(t("seq.max"), "bad");
    edit("", () => { steps.push(newAction(v)); });
    rerender();
  }, "", t("seq.add"));
  add.classList.add("step-add");
  return h("div", { class: "stack" }, steps.length ? box : null, add);
}

function sequenceEditor(a, rerender) {
  a.steps ||= [];
  const types = [...TYPES.filter((x) => x !== "sequence"), "wait"];
  return h("div", { class: "stack" }, h("p", { class: "help" }, t("seq.help")), recorderBar(a, rerender), stepList(a.steps, rerender, types));
}

let activeRecording = null;
export function stopRecording() { activeRecording?.(false); }

function recorderBar(a, rerender) {
  const host = h("div", { class: "recorder" });
  let rec = null, onKey = null, timings = true;

  function draw() {
    host.replaceChildren();
    if (!rec) {
      host.append(h("button", { type: "button", class: "btn sm", onclick: start }, ic("record", 14), t("rec.start")), h("span", { class: "help" }, t("rec.hint")));
      return;
    }
    const last = rec.steps.slice(-6);
    host.append(h("div", { class: "rec-live", role: "status" },
      h("div", { class: "row" }, h("span", { class: "rec-dot" }), h("b", {}, t("rec.running")), h("span", { class: "faint" }, t("rec.count", { n: rec.steps.length }))),
      h("div", { class: "rec-steps" }, last.length ? last.map((s) => h("span", { class: `tag ${s.type}` }, describeStep(s))) : h("span", { class: "faint" }, t("rec.empty"))),
      rec.full ? h("p", { class: "help bad" }, t("seq.max")) : null,
      toggle(t("rec.timings"), timings, (v) => { timings = v; rec.setTimings(v); }, t("rec.timings_sub")),
      h("div", { class: "row" },
        h("button", { type: "button", class: "btn sm primary", onclick: () => stop(true) }, ic("check", 14), t("rec.save")),
        h("button", { type: "button", class: "btn sm", onclick: () => stop(false) }, t("common.cancel")))));
  }

  function start() {
    stopRecording();
    rec = createRecorder({ timings, max: Math.max(0, MAX_STEPS - a.steps.length) });
    holdEditing(true);
    activeRecording = stop;
    onKey = (e) => {
      if (!host.isConnected) { stop(false); return; }
      if (e.target.closest?.(".recorder button, .recorder input, .recorder label")) return;
      e.preventDefault(); e.stopPropagation();
      if (rec.key(e)) draw();
    };
    window.addEventListener("keydown", onKey, true);
    draw();
  }

  function stop(save) {
    if (!rec) return;
    window.removeEventListener("keydown", onKey, true);
    holdEditing(false);
    if (activeRecording === stop) activeRecording = null;
    const steps = rec.steps;
    rec = null;
    if (save && steps.length && host.isConnected) { edit("", () => { a.steps.push(...steps); }); toast(t("rec.saved", { n: steps.length })); rerender(); return; }
    if (host.isConnected) draw();
  }

  draw();
  return host;
}

function ifEditor(a, rerender) {
  a.steps ||= []; a.else ||= [];
  const types = TYPES.filter((x) => x !== "sequence" && x !== "if").concat("wait");
  const list = [];
  list.push(select(t("cond.when"), a.cond, CONDS.map((c) => [c, t("cond." + c)]), (v) => { edit("", () => { a.cond = v; a.target = v === "os" ? osName() : ""; }); rerender(); }));
  if (a.cond === "os") list.push(select(t("cond.value"), a.target || "darwin", [["darwin", "macOS"], ["windows", "Windows"], ["linux", "Linux"]], (v) => edit("", () => { a.target = v; })));
  else if (!a.cond.startsWith("obs_")) list.push(textInput(t("cond.value"), a.target, (v) => edit("f:target", () => { a.target = v; }), { placeholder: t("cond.ph." + a.cond), help: t("cond.help." + a.cond) }));
  else list.push(h("p", { class: "help" }, t("cond.obs_help")));
  list.push(toggle(t("cond.invert"), !!a.not, (v) => edit("", () => { if (v) a.not = true; else delete a.not; }), t("cond.invert_sub")));
  list.push(h("div", { class: "subgroup" }, h("h4", {}, t("cond.then")), stepList(a.steps, rerender, types)));
  list.push(h("div", { class: "subgroup" }, h("h4", {}, t("cond.else")), stepList(a.else, rerender, types)));
  return h("div", { class: "stack" }, ...list);
}

let obsAsked = false;
async function loadObsInfo(rerender) {
  if (obsAsked) return;
  obsAsked = true;
  try {
    const r = await api("/api/obs/probe", { method: "POST", body: {} });
    if (r.ok) { state.obsInfo = r.info; obsDatalist("scenes"); obsDatalist("inputs"); }
  } catch {}
  setTimeout(() => { obsAsked = false; }, 30000);
}

export const typeOptions = () => [["", t("type.none")], ...TYPES.map((x) => [x, t("type." + x)])];

function needsInput(a) {
  switch (a.type) {
    case "app": return !(a.app || "").trim();
    case "url": case "http": return !(a.url || "").trim();
    case "shell": return !(a.cmd || "").trim();
    case "ssh": return !(a.host || "").trim() || !(a.cmd || "").trim();
    case "hotkey": return !(a.keys || "").trim();
    case "text": return !(a.text || "").trim();
    case "obs": return (a.cmd === "scene" || a.cmd === "mute") && !(a.target || "").trim();
    case "sequence": return !(a.steps || []).length;
    default: return false;
  }
}

const allTypes = (a) => [a.type, ...(a.steps || []).flatMap(allTypes), ...(a.else || []).flatMap(allTypes)];
const typesText = (a) => allTypes(a).some((x) => x === "text" || x === "hotkey");

export function wantsConfirm(a) {
  switch (a.type) {
    case "shell": case "ssh": case "sequence": case "if": return true;
    case "app": return a.mode === "quit";
    case "http": return (a.method || "GET") !== "GET";
    case "system": return RISKY_SYSTEM.includes(a.cmd);
    case "obs": return ["stream", "stream_start", "stream_stop", "record"].includes(a.cmd);
  }
  return false;
}

function testKind(a) {
  if (a.type === "layer" || a.type === "history" || a.type === "wait") return "none";
  if (a.type === "system" && a.cmd === "sleep") return "none";
  return typesText(a) || (a.type === "system" && a.cmd === "lock") ? "delay" : "run";
}

function testBox(a, kind) {
  const out = h("div", { "aria-live": "polite" });
  const label = h("span", {}, t(kind === "delay" ? "test.run_in" : "test.run", { n: DELAY_S }));
  const btn = h("button", { type: "button", class: "btn sm", disabled: needsInput(a) }, ic("play", 14), label);
  let timer = null;
  const reset = () => { clearInterval(timer); timer = null; label.textContent = t(kind === "delay" ? "test.run_in" : "test.run", { n: DELAY_S }); };
  const go = async () => {
    clear(out).append(h("div", { class: "console" }, t("test.running")));
    let r;
    try { r = await api("/api/test", { method: "POST", body: a }); } catch (e) { r = { ok: false, output: e.message }; }
    clear(out).append(h("div", { class: `console ${r.ok ? "ok" : "bad"}` },
      h("div", { class: "console-head" }, ic(r.ok ? "check" : "alert", 14), r.ok ? t("test.ok") : t("test.fail"), r.ms !== undefined ? h("span", { class: "faint" }, `${r.ms} ms`) : null),
      r.output || t("test.no_output")));
  };
  btn.addEventListener("click", () => {
    if (kind !== "delay") return go();
    if (timer) { reset(); clear(out); return; }
    let n = DELAY_S;
    label.textContent = t("test.cancel_in", { n });
    clear(out).append(h("div", { class: "console" }, t("test.delay_wait")));
    timer = setInterval(() => {
      if (!btn.isConnected) { reset(); return; }
      n--;
      if (n > 0) { label.textContent = t("test.cancel_in", { n }); return; }
      reset(); go();
    }, 1000);
  });
  const note = needsInput(a) ? t("test.need_input") : kind === "delay" ? t("test.delay_help", { n: DELAY_S }) : "";
  return h("div", { class: "stack tight" }, h("div", {}, btn), note ? h("p", { class: "help" }, note) : null, out);
}

export function actionExtras(a) {
  const list = [];
  if (wantsConfirm(a) || a.confirm) list.push(toggle(t("act.confirm"), !!a.confirm, (v) => edit("confirm", () => { if (v) a.confirm = true; else delete a.confirm; }), t("act.confirm_sub")));
  const kind = testKind(a);
  if (kind === "none") list.push(h("p", { class: "help" }, t("test.none." + (a.type === "system" ? "sleep" : a.type))));
  else list.push(testBox(a, kind));
  return list;
}
