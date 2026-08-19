import { state } from "./store.js";

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const $ = (s) => document.querySelector(s);
const $$ = (s) => [...document.querySelectorAll(s)];
const byText = (sel, text) => $$(sel).find((e) => e.textContent.trim().includes(text));
const typeInto = (el, v) => { el.value = v; el.dispatchEvent(new Event("input", { bubbles: true })); el.dispatchEvent(new Event("change", { bubbles: true })); };
const pane = () => $(".pane");
const gesture = async (g) => { $(`.gtab[data-g="${g}"]`).click(); await sleep(40); };

export async function run() {
  const results = [];
  const ok = (name, cond, extra = "") => results.push({ name, ok: !!cond, extra: String(extra) });
  try {
    location.hash = "#/keys"; await sleep(120);
    ok("teclado ISO con 105 teclas", $$(".kb .cap").length === 105, $$(".kb .cap").length);
    ok("las capas se muestran numeradas", $$(".layerbar .layer .n").length >= 2);

    $('.cap[data-u="4"]').click(); await sleep(40);
    ok("seleccionar la tecla A", state.selKey === "04" && /A/.test($(".dock-head .keyname").textContent));

    const sel = pane().querySelector("select"); sel.value = "app"; sel.dispatchEvent(new Event("change", { bubbles: true })); await sleep(60);
    ok("elegir acción app en Pulsar", state.cfg.layers[0].keys["04"]?.tap?.type === "app");
    typeInto($('input[list="apps"]'), "Safari"); await sleep(50);
    ok("escribir la app", state.cfg.layers[0].keys["04"].tap.app === "Safari");
    ok("la tecla muestra el nombre y queda marcada", $('.cap[data-u="4"] .ml')?.textContent === "Safari" && $('.cap[data-u="4"]').classList.contains("mapped"));

    await gesture("hold");
    const sel2 = pane().querySelector("select"); sel2.value = "url"; sel2.dispatchEvent(new Event("change", { bubbles: true })); await sleep(60);
    typeInto(pane().querySelector('input[placeholder^="https"]'), "https://example.com"); await sleep(50);
    ok("gesto Mantener con enlace", state.cfg.layers[0].keys["04"].hold?.url === "https://example.com");
    ok("indicador de gesto en la tecla", !!$('.cap[data-u="4"] .pips'));

    $('.tool[aria-label="Deshacer"]').click(); await sleep(50);
    ok("deshacer", state.cfg.layers[0].keys["04"].hold?.url === "");
    $('.tool[aria-label="Rehacer"]').click(); await sleep(50);
    ok("rehacer", state.cfg.layers[0].keys["04"].hold?.url === "https://example.com");

    const sw = $$(".dock .group .swatch:not(.none):not(.custom)")[3]; sw.click(); await sleep(50);
    ok("color de tecla", /^#/.test(state.cfg.layers[0].keys["04"].color || "") && $('.cap[data-u="4"]').classList.contains("colored"));

    document.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowRight", bubbles: true })); await sleep(50);
    ok("flechas mueven la selección por geometría", state.selKey !== "04", state.selKey);

    const n0 = state.cfg.layers.length;
    byText(".layer.add", "Capa").click(); await sleep(60);
    ok("añadir capa abre su panel", state.cfg.layers.length === n0 + 1 && state.panel === "layer");
    typeInto($(".dock input[type=text]"), "Pruebas"); await sleep(50);
    ok("renombrar capa", state.cfg.layers.at(-1).name === "Pruebas" && byText(".layer", "Pruebas"));

    document.dispatchEvent(new KeyboardEvent("keydown", { key: "k", metaKey: true })); await sleep(60);
    ok("abrir paleta", !!$(".palette"));
    typeInto($(".palette input"), "ajustes"); await sleep(40);
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true })); await sleep(140);
    ok("la paleta navega", location.hash === "#/settings" && !$(".palette"), location.hash);

    byText(".seg button", "Oscuro").click(); await sleep(50);
    ok("tema oscuro", state.cfg.settings.theme === "dark" && document.documentElement.dataset.theme === "dark");
    $$(".swatches .swatch:not(.custom)")[2].click(); await sleep(50);
    ok("color de acento", getComputedStyle(document.documentElement).getPropertyValue("--accent").trim() === state.cfg.settings.accent);

    location.hash = "#/gallery"; await sleep(120);
    ok("galería con paquetes", $$(".pack").length >= 6, $$(".pack").length);
    const layers0 = state.cfg.layers.length;
    byText(".pack .btn.primary", "Añadir").click(); await sleep(80);
    ok("añadir paquete crea una capa", state.cfg.layers.length === layers0 + 1);

    location.hash = "#/sheet"; await sleep(120);
    ok("hoja con un bloque por capa", $$(".sheet-block").length >= state.cfg.layers.length);
    location.hash = "#/activity"; await sleep(140);
    ok("actividad carga", $$(".stat").length === 4);
    location.hash = "#/diag"; await sleep(140);
    ok("diagnóstico carga", !!byText("h2", "Estado"));

    location.hash = "#/keys"; await sleep(100);
    byText(".status", "").click(); await sleep(100);
    ok("ficha del teclado se abre", !!$(".drawer"));
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true })); await sleep(60);
    ok("la ficha se cierra con Esc", !$(".drawer"));
  } catch (e) {
    ok("excepción", false, e.stack || e.message);
  }
  const pre = document.createElement("pre");
  pre.id = "selftest";
  pre.textContent = JSON.stringify(results);
  document.body.append(pre);
}
