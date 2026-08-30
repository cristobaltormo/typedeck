import { state, notify, applySettings } from "./store.js";
import { openPalette } from "./palette.js";
import { openKeyboardDrawer } from "./kbinfo.js";

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const app = (a, mode = "open") => ({ type: "app", app: a, mode });
const demo = () => ({
  version: 3,
  settings: { language: "es", theme: "dark", accent: "#2563eb", density: "comfortable", keysize: 88, input: "auto", typing_layout: "auto",
    hud: { enabled: true, position: "bottom", seconds: 1.2, on_action: false, on_auto: true, sound: false }, hold_ms: 450, double_ms: 280, auto_layer: true, volume_step: 6 },
  global: { "53": { tap: { type: "layer", to: "next" }, label: "Capa", icon: "layers" }, "58": { tap: { type: "url", url: "http://127.0.0.1:7788/" }, label: "Editor", icon: "sliders" },
    "39": { tap: { type: "hotkey", keys: "esc" }, hold: { type: "layer", to: "next", momentary: true }, label: "Capa", icon: "layers" } },
  layers: [
    { name: "Trabajo", color: "#2563eb", icon: "code", auto_apps: ["Code", "Xcode"], keys: {
      "3A": { tap: app("Visual Studio Code", "toggle"), icon: "code", color: "#2563eb", label: "VS Code" },
      "3B": { tap: app("Termius", "toggle"), icon: "terminal", color: "#16a34a", label: "Terminal", hold: { type: "ssh", host: "my-server", cmd: "uptime -p", show_output: true } },
      "3C": { tap: app("Google Chrome"), icon: "globe", color: "#f59e0b" },
      "3D": { tap: { type: "ssh", host: "my-server", cmd: "df -h / | tail -1", show_output: true }, icon: "server", color: "#16a34a", label: "Disco" },
      "3E": { tap: { type: "shell", cmd: "open ~/Downloads" }, icon: "folder", label: "Descargas" },
      "3F": { tap: app("Discord"), icon: "chat", color: "#8b5cf6" },
      "40": { tap: { type: "text", text: "{date}" }, icon: "calendar", label: "Fecha", double: { type: "text", text: "{datetime}" } },
      "41": { tap: { type: "system", cmd: "screenshot" }, icon: "camera", color: "#ec4899", label: "Captura" },
      "42": { tap: { type: "system", cmd: "lock", confirm: true }, icon: "lock", color: "#cc4848", label: "Bloquear" },
      "43": { tap: { type: "timer", minutes: 25, label: "Pomodoro" }, icon: "clock", label: "Pomodoro", color: "#f97316" },
      "44": { tap: app("Fantasma No Instalada"), label: "Falta" },
      "5F": { tap: app("Spotify"), icon: "music", color: "#16a34a", label: "Spotify" }, "60": { tap: { type: "media", cmd: "playpause" }, icon: "play" }, "61": { tap: { type: "media", cmd: "next" }, icon: "skip-next" } } },
    { name: "Multimedia", color: "#8b5cf6", icon: "music", auto_apps: ["Spotify"], keys: { "5C": { tap: { type: "media", cmd: "prev" }, icon: "skip-prev" } } },
    { name: "Servidor", color: "#16a34a", icon: "server", auto_apps: [], keys: {} },
  ],
  keyboards: {},
});

export async function run(scene) {
  state.cfg = demo();
  if (scene === "light") state.cfg.settings.theme = "light";
  if (scene === "empty" || scene === "offline") for (const l of state.cfg.layers) l.keys = {};
  if (scene === "empty") state.cfg.global = {};
  if (scene === "numpad") state.cfg.keyboards = { "258A:0016": { layout: "numpad" } };
  applySettings();
  Object.assign(state.status, { layer: 0, connected: true, port: "/dev/cu.usbmodemCHIDJB1", firmware: "3", version: "0.3.0", uptime: 4000, front: ["Google Chrome"] });
  state.keyboard = { connected: true, prefs: null, info: { present: true, firmware: "3", vid: "258A", pid: "0016", bcd: "0001", usb: "0200", mfr: "BY Tech", prod: "Usb Gaming Keyboard", serial: "", power_ma: 500, ifaces: 2,
    reports: [{ id: 0, dir: "input", kind: "keyboard", bits: 64, detail: "interfaz 0: hasta 6 teclas a la vez" }, { id: 1, dir: "input", kind: "system", detail: "interfaz 1: encendido, suspender y despertar" }, { id: 2, dir: "input", kind: "consumer", detail: "interfaz 1: teclas multimedia" },
      { id: 3, dir: "input", kind: "vendor", detail: "interfaz 1: datos del fabricante" }, { id: 4, dir: "input", kind: "nkro", detail: "interfaz 1: 120 teclas a la vez" }, { id: 7, dir: "input", kind: "mouse", detail: "interfaz 1: ratón integrado" }] } };
  if (scene === "offline") { state.status.connected = false; state.keyboard.info = null; }
  location.hash = "#/" + (new URLSearchParams(location.search).get("view") || "keys");
  await sleep(150);
  notify("cfg", "replace");
  state.scope = 0; state.selKey = "3B"; state.panel = "key";
  if (scene === "seq") {
    state.cfg.layers[0].keys["45"] = { tap: { type: "sequence", steps: [app("Zoom"), { type: "wait", ms: 400 }, { type: "media", cmd: "mute" }, { type: "hud", text: "Modo reunión" }] }, icon: "mic", color: "#ec4899", label: "Reunión" };
    state.selKey = "45";
  }
  if (scene === "obs") {
    state.cfg.layers[0].keys["45"] = { tap: { type: "obs", cmd: "scene", target: "#2" }, icon: "camera", color: "#cc4848", label: "Escena 2" };
    state.selKey = "45";
  }
  if (scene === "hold") state.selKey = "39";
  if (scene === "layer") state.panel = "layer";
  notify("cfg", "replace");
  if (scene === "palette") { await sleep(80); openPalette(); }
  if (scene === "drawer") { await sleep(80); openKeyboardDrawer(); }
}
