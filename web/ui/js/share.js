import { h, ic } from "./dom.js";
import { api } from "./api.js";
import { t } from "./i18n.js";
import { toast } from "./toast.js";
import { openModal } from "./modal.js";
import { clone, slug } from "./util.js";
import { MAX_LAYERS, insertLayer } from "./layerops.js";
import { state, edit, layerCount } from "./store.js";

export function download(name, data) {
  const blob = new Blob([JSON.stringify(data, null, 2)], { type: "application/json" });
  const a = h("a", { href: URL.createObjectURL(blob), download: name });
  document.body.append(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(a.href), 1000);
}

export function pickFile(onFile) {
  const input = h("input", { type: "file", accept: "application/json,.json", hidden: true });
  input.addEventListener("change", () => { const f = input.files[0]; input.remove(); if (f) onFile(f); });
  input.addEventListener("cancel", () => input.remove());
  document.body.append(input);
  input.click();
}

export function exportLayer(index) {
  const l = clone(state.cfg.layers[index]);
  download(`typedeck-${slug(l.name)}.json`, { typedeck_macros: "1", name: l.name, layers: [l] });
}

export function exportEverything() {
  download("typedeck-macros.json", { typedeck_macros: "1", name: "Typedeck", layers: clone(state.cfg.layers), global: clone(state.cfg.global) });
}

export const importPack = () => pickFile(inspectFile);

export async function inspectFile(f) {
  if (f.size > 1 << 20) return toast(t("share.invalid"), "bad");
  let raw;
  try { raw = JSON.parse(await f.text()); } catch { return toast(t("share.invalid"), "bad"); }
  try { review(await api("/api/macros/inspect", { method: "POST", body: { pack: raw } })); } catch (e) { toast(`${t("share.invalid")} ${e.message}`, "bad"); }
}

const targetLayer = () => (state.scope === "global" ? 0 : state.scope);

function apply(pack, mode) {
  let added = 0;
  edit("", (cfg) => {
    if (mode === "merge") {
      const layer = cfg.layers[targetLayer()];
      for (const l of pack.layers) for (const [id, kd] of Object.entries(l.keys || {})) { layer.keys[id] = kd; added++; }
      for (const [id, kd] of Object.entries(pack.global || {})) { cfg.global[id] = kd; added++; }
      return;
    }
    for (const l of pack.layers) {
      if (!insertLayer(cfg, cfg.layers.length, { ...l, keys: l.keys || {}, name: l.name || t("layer.default_name", { n: cfg.layers.length + 1 }) })) break;
      added += Object.keys(l.keys || {}).length;
    }
    for (const [id, kd] of Object.entries(pack.global || {})) if (!cfg.global[id]) { cfg.global[id] = kd; added++; }
  });
  toast(t("share.done", { n: added }));
}

function review(r) {
  const pack = r.pack;
  const fits = layerCount() + pack.layers.length <= MAX_LAYERS;
  let mode = fits ? "new" : "merge";
  const radio = (value, label, sub, disabled) => h("label", { class: `choice ${disabled ? "disabled" : ""}` },
    h("input", { type: "radio", name: "import-mode", value, checked: mode === value, disabled, onchange: () => { mode = value; } }), h("span", {}, h("b", {}, label), h("small", {}, sub)));
  const risky = r.risky.length
    ? h("div", { class: "notice warn" }, ic("alert", 16), h("div", { class: "grow" }, h("p", {}, t("share.risky", { n: r.risky.length })),
      h("ul", { class: "risk" }, r.risky.slice(0, 8).map((x) => h("li", {}, h("span", { class: "tag" }, t("type." + x.type)), h("code", {}, x.text || "-"))))))
    : null;
  const panel = h("div", { class: "dialog" },
    h("header", { class: "dialog-head" }, h("h2", { class: "grow" }, t("share.title"))),
    h("p", { class: "dim" }, t("share.summary", { name: pack.name || "-", layers: pack.layers.length, keys: r.keys })),
    risky,
    h("fieldset", { class: "choices" }, h("legend", { class: "sr" }, t("share.title")),
      radio("new", t("share.as_layers"), t("share.as_layers_sub"), !fits),
      radio("merge", t("share.merge"), t("share.merge_sub", { name: state.cfg.layers[targetLayer()].name }))),
    h("footer", { class: "dialog-foot" },
      h("button", { type: "button", class: "btn", onclick: () => modal.close() }, t("common.cancel")),
      h("button", { type: "button", class: "btn primary", onclick: () => { modal.close(); apply(pack, mode); } }, ic("download", 14), t("share.import"))));
  const modal = openModal(panel, { label: t("share.title") });
}
