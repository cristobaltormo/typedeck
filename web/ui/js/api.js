export const token = document.querySelector('meta[name="token"]')?.content || "";
export const TAB = Math.random().toString(36).slice(2, 12);
const query = new URLSearchParams(location.search);
export const DRY = query.has("selftest") || query.has("scene");
const WRITES = new Set(["/api/config", "/api/backups/restore", "/api/backups/create", "/api/stats/reset", "/api/hud", "/api/layer", "/api/board", "/api/capture", "/api/learn", "/api/test", "/api/keyboard/layout", "/api/macros/inspect", "/api/history/clear"]);

export async function api(path, { method = "GET", body, signal, keepalive } = {}) {
  if (DRY && method === "POST" && WRITES.has(path)) return { ok: true, config: body, reply: [] };
  const res = await fetch(path, {
    method, signal, keepalive,
    headers: { "X-Token": token, "X-Tab": TAB, ...(body !== undefined ? { "Content-Type": "application/json" } : {}) },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  let data = null;
  try { data = await res.json(); } catch {}
  if (!res.ok) throw new Error((data && data.error) || `HTTP ${res.status}`);
  return data;
}
