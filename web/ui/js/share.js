import { h, ic } from "./dom.js";
import { api } from "./api.js";
import { t } from "./i18n.js";
import { toast } from "./toast.js";
import { state, edit, layerCount } from "./store.js";

const clone = (o) => JSON.parse(JSON.stringify(o));
const slug = (s) => (s || "macros").toLowerCase().normalize("NFD").replace(/[̀-ͯ]/g, "").replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "") || "macros";

function download(name, data) {
  const blob = new Blob([JSON.stringify(data, null, 2)], { type: "application/json" });
  const a = h("a", { href: URL.createObjectURL(blob), download: name });
  a.click(); URL.revokeObjectURL(a.href);
}

export function exportLayer(index) {
  const l = clone(state.cfg.layers[index]);
  download(`typedeck-${slug(l.name)}.json`, { typedeck_macros: "1", name: l.name, layers: [l] });
}

export function exportEverything() {
  download("typedeck-macros.json", { typedeck_macros: "1", name: "Typedeck", layers: clone(state.cfg.layers), global: clone(state.cfg.global) });
}

export function importPack() {
  const input = h("input", { type: "file", accept: "application/json,.json", hidden: true });
  input.addEventListener("change", async () => {
    const f = input.files[0];
    if (!f) return;
    if (f.size > 1 << 20) return toast(t("share.invalid"), "bad");
    let raw;
    try { raw = JSON.parse(await f.text()); } catch { return toast(t("share.invalid"), "bad"); }
    try {
      const r = await api("/api/macros/inspect", { method: "POST", body: { pack: raw } });
      review(r);
    } catch (e) { toast(`${t("share.invalid")} ${e.message}`, "bad"); }
  });
  document.body.append(input);
  input.click();
  setTimeout(() => input.remove(), 60000);
}

function apply(pack, mode) {
  let added = 0;
  edit("", (cfg) => {
    if (mode === "merge") {
      const layer = cfg.layers[state.scope === "global" ? 0 : state.scope];
      for (const l of pack.layers) for (const [id, kd] of Object.entries(l.keys)) { layer.keys[id] = kd; added++; }
      for (const [id, kd] of Object.entries(pack.global || {})) { cfg.global[id] = kd; added++; }
      return;
    }
    for (const l of pack.layers) {
      if (cfg.layers.length >= 9) break;
      cfg.layers.push({ ...l, name: l.name || t("layer.default_name", { n: cfg.layers.length + 1 }) });
      added += Object.keys(l.keys).length;
    }
    for (const [id, kd] of Object.entries(pack.global || {})) if (!cfg.global[id]) { cfg.global[id] = kd; added++; }
  });
  toast(t("share.done", { n: added }));
}

function review(r) {
  const pack = r.pack;
  let mode = layerCount() + pack.layers.length <= 9 ? "new" : "merge";
  const close = () => { scrim.remove(); document.removeEventListener("keydown", onKey, true); };
  const onKey = (e) => { if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); close(); } };
  const radio = (value, label, sub, disabled) => h("label", { class: "choice" }, h("input", { type: "radio", name: "mode", checked: mode === value, disabled, onchange: () => { mode = value; } }), h("span", {}, h("b", {}, label), h("small", {}, sub)));
  const risky = r.risky.length
    ? h("div", { class: "notice warn" }, ic("alert", 17), h("div", {}, h("p", {}, t("share.risky", { n: r.risky.length })),
      h("ul", { class: "risk" }, r.risky.slice(0, 8).map((x) => h("li", {}, h("span", { class: "tag" }, t("type." + x.type)), h("code", {}, x.text || "-"))))))
    : null;
  const scrim = h("div", { class: "scrim", onclick: (e) => { if (e.target === scrim) close(); } },
    h("div", { class: "dialog", role: "dialog", "aria-modal": "true", "aria-label": t("share.title") },
      h("h2", {}, t("share.title")),
      h("p", { class: "dim" }, t("share.summary", { name: pack.name || "-", layers: pack.layers.length, keys: r.keys })),
      risky,
      h("div", { class: "choices" },
        radio("new", t("share.as_layers"), t("share.as_layers_sub"), layerCount() + pack.layers.length > 9),
        radio("merge", t("share.merge"), t("share.merge_sub", { name: state.cfg.layers[state.scope === "global" ? 0 : state.scope].name }))),
      h("div", { class: "row", style: { gap: "8px", justifyContent: "flex-end" } },
        h("button", { type: "button", class: "btn", onclick: close }, t("common.cancel")),
        h("button", { type: "button", class: "btn primary", onclick: () => { close(); apply(pack, mode); } }, ic("download", 15), t("share.import")))));
  document.addEventListener("keydown", onKey, true);
  document.body.append(scrim);
}
