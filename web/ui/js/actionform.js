import { h, ic } from "./dom.js";
import { t } from "./i18n.js";
import { toast } from "./toast.js";
import { field, textInput, area, select, toggle } from "./controls.js";
import { state, edit, newAction } from "./store.js";
import { TYPE_ICON } from "./keyboard.js";

export const TYPES = ["app", "url", "shell", "ssh", "http", "hotkey", "text", "sequence", "media", "system", "timer", "layer", "hud"];
const MEDIA = ["playpause", "next", "prev", "volup", "voldown", "mute"];
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
  }
  return list;
}

function sequenceEditor(a, rerender) {
  a.steps ||= [];
  const box = h("div", { class: "steps" });
  const types = [...TYPES.filter((x) => x !== "sequence"), "wait"];
  a.steps.forEach((s, i) => {
    const ctl = (name, fn, disabled) => h("button", { type: "button", class: "btn sm icon", "aria-label": t("seq." + name), title: t("seq." + name), disabled, onclick: fn }, ic(name === "up" ? "arrow-up" : name === "down" ? "arrow-down" : "trash", 15));
    box.append(h("div", { class: "step" },
      h("div", { class: "row", style: { gap: "6px" } },
        h("div", { class: "grow" }, select("", s.type, types.map((x) => [x, t("type." + x)]), (v) => { edit("", () => { a.steps[i] = newAction(v); }); rerender(); })),
        ctl("up", () => { edit("", () => { [a.steps[i - 1], a.steps[i]] = [a.steps[i], a.steps[i - 1]]; }); rerender(); }, i === 0),
        ctl("down", () => { edit("", () => { [a.steps[i + 1], a.steps[i]] = [a.steps[i], a.steps[i + 1]]; }); rerender(); }, i === a.steps.length - 1),
        ctl("del", () => { edit("", () => { a.steps.splice(i, 1); }); rerender(); })),
      ...actionFields(s, rerender, { inSeq: true })));
  });
  const add = select("", "", [["", t("seq.add")], ...types.map((x) => [x, t("type." + x)])], (v) => { if (!v) return; if (a.steps.length >= 30) return toast(t("seq.max"), "bad"); edit("", () => { a.steps.push(newAction(v)); }); rerender(); });
  return h("div", { class: "stack", style: { gap: "10px" } }, h("p", { class: "help" }, t("seq.help")), box, add);
}

export const typeOptions = () => [["", t("type.none")], ...TYPES.map((x) => [x, t("type." + x)])];
export { TYPE_ICON };
