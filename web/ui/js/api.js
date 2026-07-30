const TOKEN = document.querySelector('meta[name="token"]')?.content || "";
const _q = new URLSearchParams(location.search);
const DRY = _q.has("selftest") || _q.has("scene");
const WRITES = new Set(["/api/config", "/api/backups/restore", "/api/backups/create", "/api/stats/reset", "/api/hud", "/api/layer", "/api/board", "/api/learn", "/api/test", "/api/keyboard/layout"]);

export async function api(path, { method = "GET", body, signal } = {}) {
  if (DRY && method === "POST" && WRITES.has(path)) return { ok: true, config: body, reply: [] };
  const res = await fetch(path, {
    method, signal,
    headers: { "X-Token": TOKEN, ...(body !== undefined ? { "Content-Type": "application/json" } : {}) },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  let data = null;
  try { data = await res.json(); } catch {  }
  if (!res.ok) throw new Error((data && data.error) || `HTTP ${res.status}`);
  return data;
}
