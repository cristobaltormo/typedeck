import { state } from "./store.js";

const SH = (main, shift) => ({ main, shift });

const PUNCT = {
  us: { 0x35: SH("`", "~"), 0x2D: SH("-", "_"), 0x2E: SH("=", "+"), 0x2F: SH("[", "{"), 0x30: SH("]", "}"), 0x31: SH("\\", "|"),
    0x33: SH(";", ":"), 0x34: SH("'", "\""), 0x36: SH(",", "<"), 0x37: SH(".", ">"), 0x38: SH("/", "?"), 0x32: SH("#", "~"), 0x64: SH("\\", "|") },
  es: { 0x35: SH("º", "ª"), 0x2D: SH("'", "?"), 0x2E: SH("¡", "¿"), 0x2F: SH("`", "^"), 0x30: SH("+", "*"), 0x31: SH("ç", "Ç"),
    0x33: SH("ñ", "Ñ"), 0x34: SH("´", "¨"), 0x36: SH(",", ";"), 0x37: SH(".", ":"), 0x38: SH("-", "_"), 0x32: SH("ç", "Ç"), 0x64: SH("<", ">") },
};
const DIGITS = { us: ["!", "@", "#", "$", "%", "^", "&", "*", "(", ")"], es: ["!", "\"", "·", "$", "%", "&", "/", "(", ")", "="] };

const NAMES = {
  es: { 0x29: "Esc", 0x2A: "Retroceso", 0x2B: "Tab", 0x39: "Bloq Mayús", 0x28: "Intro", 0x2C: "", 0x46: "ImpPant", 0x47: "BloqDesp", 0x48: "Pausa",
    0x49: "Insert", 0x4A: "Inicio", 0x4B: "RePág", 0x4C: "Supr", 0x4D: "Fin", 0x4E: "AvPág", 0x53: "Num", 0x58: "Intro", 0x65: "Menú",
    0xE0: "Ctrl", 0xE1: "Mayús", 0xE2: "Alt", 0xE3: "Win", 0xE4: "Ctrl", 0xE5: "Mayús", 0xE6: "Alt Gr", 0xE7: "Win" },
  en: { 0x29: "Esc", 0x2A: "Backspace", 0x2B: "Tab", 0x39: "Caps", 0x28: "Enter", 0x2C: "", 0x46: "PrtSc", 0x47: "ScrLk", 0x48: "Pause",
    0x49: "Insert", 0x4A: "Home", 0x4B: "PgUp", 0x4C: "Del", 0x4D: "End", 0x4E: "PgDn", 0x53: "Num", 0x58: "Enter", 0x65: "Menu",
    0xE0: "Ctrl", 0xE1: "Shift", 0xE2: "Alt", 0xE3: "Win", 0xE4: "Ctrl", 0xE5: "Shift", 0xE6: "Alt", 0xE7: "Win" },
};
const ARROWS = { 0x4F: "arrow-right", 0x50: "arrow-left", 0x51: "arrow-down", 0x52: "arrow-up" };

export function legend(usage, lang) {
  const l = lang === "en" ? "en" : "es";
  const set = PUNCT[l === "en" ? "us" : "es"];
  if (usage >= 0x04 && usage <= 0x1D) return { main: String.fromCharCode(65 + usage - 4) };
  if (usage >= 0x1E && usage <= 0x27) { const i = usage - 0x1E; return { main: String((i + 1) % 10), shift: DIGITS[l === "en" ? "us" : "es"][i] }; }
  if (set[usage]) return set[usage];
  if (usage >= 0x3A && usage <= 0x45) return { main: "F" + (usage - 0x3A + 1) };
  if (usage >= 0x68 && usage <= 0x73) return { main: "F" + (usage - 0x68 + 13) };
  if (usage >= 0x59 && usage <= 0x61) return { main: String(usage - 0x58) };
  if (usage === 0x62) return { main: "0" };
  const kp = { 0x54: "/", 0x55: "*", 0x56: "-", 0x57: "+", 0x63: "." };
  if (kp[usage]) return { main: kp[usage] };
  if (ARROWS[usage]) return { icon: ARROWS[usage] };
  const n = NAMES[l][usage];
  return { main: n === undefined ? "0x" + usage.toString(16).toUpperCase() : n };
}

export const isModifier = (u) => u >= 0xE0 && u <= 0xE7;

export function legendLang() {
  const sys = state.system?.typing_layout;
  if (sys === "es-iso" || sys === "es-pc") return "es";
  if (sys === "us") return "en";
  return state.cfg?.settings.language || "es";
}

export function layout(id, lang = legendLang()) {
  const src = state.layouts?.[id] || state.layouts?.["full-iso"];
  if (!src) return { keys: [], width: 1, height: 1 };
  return { ...src, keys: src.keys.map((k) => ({ usage: k.u, x: k.x, y: k.y, w: k.w, h: k.h, mod: !!k.mod, legend: legend(k.u, lang) })) };
}
export const layoutList = () => Object.values(state.layouts || {});
