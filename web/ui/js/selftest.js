import { state } from "./store.js";
import { t } from "./i18n.js";
import { api } from "./api.js";

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
    location.hash = "#/gallery"; await sleep(60);
    location.hash = "#/keys"; await sleep(120);
    ok("ISO keyboard with 105 keys", $$(".kb .cap").length === 105, $$(".kb .cap").length);
    ok("layers are shown numbered", $$(".layerbar .layer .n").length >= 2);

    $('.cap[data-u="4"]').click(); await sleep(40);
    ok("select key A", state.selKey === "04" && /A/.test($(".dock-head .keyname").textContent));

    const sel = pane().querySelector("select"); sel.value = "app"; sel.dispatchEvent(new Event("change", { bubbles: true })); await sleep(60);
    ok("choose the app action on Tap", state.cfg.layers[0].keys["04"]?.tap?.type === "app");
    typeInto($('input[list="apps"]'), "Safari"); await sleep(50);
    ok("type the app", state.cfg.layers[0].keys["04"].tap.app === "Safari");
    ok("the key shows the name and is marked", $('.cap[data-u="4"] .ml')?.textContent === "Safari" && $('.cap[data-u="4"]').classList.contains("mapped"));

    await gesture("hold");
    const sel2 = pane().querySelector("select"); sel2.value = "url"; sel2.dispatchEvent(new Event("change", { bubbles: true })); await sleep(60);
    typeInto(pane().querySelector('input[placeholder^="https"]'), "https://example.com"); await sleep(50);
    ok("Hold gesture with a link", state.cfg.layers[0].keys["04"].hold?.url === "https://example.com");
    ok("gesture indicator on the key", !!$('.cap[data-u="4"] .pips'));

    $(`.tool[aria-label="${t("tool.undo")}"]`).click(); await sleep(50);
    ok("undo", state.cfg.layers[0].keys["04"].hold?.url === "");
    $(`.tool[aria-label="${t("tool.redo")}"]`).click(); await sleep(50);
    ok("redo", state.cfg.layers[0].keys["04"].hold?.url === "https://example.com");

    const sw = $$(".dock .group .swatch:not(.none):not(.custom)")[3]; sw.click(); await sleep(50);
    ok("key color", /^#/.test(state.cfg.layers[0].keys["04"].color || "") && $('.cap[data-u="4"]').classList.contains("colored"));

    const centre = (u) => { const r = $(`.cap[data-u="${u}"]`).getBoundingClientRect(); return { clientX: r.left + r.width / 2, clientY: r.top + r.height / 2 }; };
    const pointer = (type, target, pos) => target.dispatchEvent(new PointerEvent(type, { bubbles: true, cancelable: true, button: 0, pointerId: 1, ...pos }));
    const drop = async (from, to) => {
      const a = centre(from), b = centre(to), src = $(`.cap[data-u="${from}"]`);
      pointer("pointerdown", src, a);
      await sleep(20); pointer("pointermove", window, { clientX: a.clientX + 12, clientY: a.clientY + 4 });
      await sleep(20); pointer("pointermove", window, b);
      await sleep(20); pointer("pointerup", window, b);
    };
    ok("the keyboard cannot be selected as text", getComputedStyle($(".kb")).userSelect === "none");
    await drop(4, 6); await sleep(80);
    ok("dragging a macro to an empty key moves it", state.cfg.layers[0].keys["06"]?.tap?.app === "Safari" && !state.cfg.layers[0].keys["04"], Object.keys(state.cfg.layers[0].keys).join());
    await drop(6, 4); await sleep(80);
    ok("dragging it back restores it", state.cfg.layers[0].keys["04"]?.tap?.app === "Safari" && !state.cfg.layers[0].keys["06"]);
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowRight", bubbles: true })); await sleep(50);
    ok("arrows move the selection by geometry", state.selKey !== "04", state.selKey);

    const n0 = state.cfg.layers.length;
    byText(".layer.add", t("strip.add")).click(); await sleep(60);
    ok("adding a layer opens its panel", state.cfg.layers.length === n0 + 1 && state.panel === "layer");
    typeInto($(".dock input[type=text]"), "Tests"); await sleep(50);
    ok("rename a layer", state.cfg.layers.at(-1).name === "Tests" && byText(".layer", "Tests"));

    document.dispatchEvent(new KeyboardEvent("keydown", { key: "k", metaKey: true })); await sleep(60);
    ok("open the palette", !!$(".palette"));
    typeInto($(".palette input"), t("nav.settings").toLowerCase()); await sleep(40);
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true })); await sleep(140);
    ok("the palette navigates", location.hash === "#/settings" && !$(".palette"), location.hash);

    byText(".seg button", t("theme.dark")).click(); await sleep(50);
    ok("dark theme", state.cfg.settings.theme === "dark" && document.documentElement.dataset.theme === "dark");
    $$(".swatches .swatch:not(.custom)")[2].click(); await sleep(50);
    ok("accent color", getComputedStyle(document.documentElement).getPropertyValue("--accent").trim() === state.cfg.settings.accent);

    ok("OBS Studio settings", !!byText("h2", t("settings.obs")) && !!byText("button", t("obs.test")));
    typeInto($('input[type=password]'), "clave"); await sleep(40);
    ok("the OBS password is stored in the settings", state.cfg.settings.obs?.password === "clave");

    location.hash = "#/gallery"; await sleep(120);
    ok("gallery with packs", $$(".pack").length >= 6, $$(".pack").length);
    ok("a pack preview shows the keys it uses, big, with its macros listed", !!$(".pack .mini.zoomable .cap") && $$(".pack .chip").length > 0);
    $(".pack .mini.zoomable").click(); await sleep(150);
    ok("clicking a pack preview enlarges it", !!$(".dialog.wide .cap"));
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true })); await sleep(80);
    ok("Escape closes the enlarged preview", !$(".dialog.wide"));
    const layers0 = state.cfg.layers.length;
    byText(".pack .btn.primary", t("gallery.add")).click(); await sleep(80);
    ok("adding a pack creates a layer", state.cfg.layers.length === layers0 + 1);

    location.hash = "#/keys"; await sleep(100);
    state.scope = 0; state.selKey = "05"; state.panel = "key"; location.hash = "#/gallery"; await sleep(60); location.hash = "#/keys"; await sleep(120);
    const typeSel = pane().querySelector("select"); typeSel.value = "sequence"; typeSel.dispatchEvent(new Event("change", { bubbles: true })); await sleep(80);
    ok("choosing a sequence shows the recorder", !!byText(".recorder button", t("rec.start")));
    byText(".recorder button", t("rec.start")).click(); await sleep(60);
    ok("recording", !!$(".rec-live"));
    const press = (key, code, extra = {}) => window.dispatchEvent(new KeyboardEvent("keydown", { key, code, bubbles: true, cancelable: true, ...extra }));
    press("h", "KeyH"); press("i", "KeyI"); press("t", "KeyT", { ctrlKey: true }); await sleep(40);
    ok("the recorder collects text and shortcuts", $$(".rec-steps .tag").map((x) => x.textContent).join("|") === "hi|ctrl+t", $$(".rec-steps .tag").map((x) => x.textContent).join("|"));
    byText(".rec-live button", t("rec.save")).click(); await sleep(80);
    const steps = state.cfg.layers[0].keys["05"].tap.steps;
    ok("recorded steps end up in the sequence", steps.length >= 2 && steps[0].type === "text" && steps.at(-1).keys === "ctrl+t", JSON.stringify(steps));
    const add = $$("select").find((x) => x.options[0]?.textContent === t("seq.add"));
    add.value = "if"; add.dispatchEvent(new Event("change", { bubbles: true })); await sleep(80);
    const cond = state.cfg.layers[0].keys["05"].tap.steps.at(-1);
    ok("add a condition to the sequence", cond?.type === "if" && cond.cond === "app", JSON.stringify(cond));
    ok("the condition shows Then and Otherwise", !!byText("h3", t("cond.then")) && !!byText("h3", t("cond.else")));

    const field = $('.pane input[type=text], .pane textarea');
    document.hasFocus = () => true;
    const editing = async (want) => { for (let i = 0; i < 200; i++) { if ((await api("/api/status")).editing === want) return true; await sleep(30); } return false; };
    field.focus();
    ok("with a field focused the program pauses capture", await editing(true));
    field.blur();
    ok("leaving the field resumes capture", await editing(false));

    location.hash = "#/sheet"; await sleep(120);
    typeInto($('header input[type=search]'), t("type.sequence").toLowerCase()); await sleep(60);
    ok("the sheet finds the sequence", $$(".macro-table tbody tr:not([hidden])").length >= 1);
    typeInto($('header input[type=search]'), "zzzz"); await sleep(60);
    ok("the sheet hides what does not match", $$(".macro-table tbody tr:not([hidden])").length === 0 && $$(".sheet-block:not([hidden])").length === 0);
    typeInto($('header input[type=search]'), ""); await sleep(40);

    ok("sheet with one block per layer", $$(".sheet-block").length >= state.cfg.layers.length);
    location.hash = "#/activity"; await sleep(140);
    ok("activity loads", $$(".stat").length === 4);
    location.hash = "#/history"; await sleep(250);
    ok("history has its own entry in the menu", !!byText(".navitem", t("nav.history")) && !!byText("h1", t("hist.title")));
    ok("history explains how to turn it on and what happens to the data", !!$(".hist-switch .switch, .hist-switch input") && ($(".privacy")?.textContent || "").includes(t("hist.privacy")));
    location.hash = "#/diag"; await sleep(140);
    ok("diagnostics loads", !!byText("h2", t("diag.status")));

    location.hash = "#/keys"; await sleep(100);
    byText(".status", "").click(); await sleep(100);
    ok("the keyboard card opens", !!$(".drawer"));
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true })); await sleep(60);
    ok("the card closes with Esc", !$(".drawer"));
  } catch (e) {
    ok("exception", false, e.stack || e.message);
  }
  const pre = document.createElement("pre");
  pre.id = "selftest";
  pre.textContent = JSON.stringify(results);
  document.body.append(pre);
}
