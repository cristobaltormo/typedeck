export const norm = (s) => String(s ?? "").toLowerCase().normalize("NFD").replace(/[̀-ͯ]/g, "");
export const clone = (o) => (o === undefined ? undefined : JSON.parse(JSON.stringify(o)));
export const hex = (u) => u.toString(16).toUpperCase().padStart(2, "0");
export const usageOf = (id) => parseInt(id, 16);
export const slug = (s) => norm(s).replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "") || "macros";
export const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
