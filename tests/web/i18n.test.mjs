import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const dir = path.join(path.dirname(fileURLToPath(import.meta.url)), "../../web/ui/js");
const src = fs.readFileSync(path.join(dir, "i18n.js"), "utf8");
const dict = (name) => {
  const body = src.match(new RegExp(`const ${name} = \\{([\\s\\S]*?)\\n\\};`))[1];
  return Object.fromEntries([...body.matchAll(/"([a-z0-9_.]+)":\s*"((?:[^"\\]|\\.)*)"/gi)].map((m) => [m[1], m[2]]));
};
const ES = dict("ES"), EN = dict("EN");
const code = fs.readdirSync(dir).filter((f) => f.endsWith(".js") && f !== "i18n.js").map((f) => fs.readFileSync(path.join(dir, f), "utf8")).join("\n");

test("Spanish and English have the same keys", () => {
  assert.deepEqual(Object.keys(ES).filter((k) => !(k in EN)), []);
  assert.deepEqual(Object.keys(EN).filter((k) => !(k in ES)), []);
});

test("every literal key used by the editor exists", () => {
  const used = new Set([...code.matchAll(/\b(?:t|tOS|osText|armed)\(\s*"([a-z0-9_]+\.[a-z0-9_.]*[a-z0-9])"(?!\s*\+)/gi)].map((m) => m[1]));
  const known = (k) => k in EN || Object.keys(EN).some((e) => e.startsWith(k + "."));
  assert.deepEqual([...used].filter((k) => !known(k)), []);
});

test("placeholders match between languages", () => {
  const vars = (s) => [...s.matchAll(/\{([a-z_]+)\}/g)].map((m) => m[1]).sort().join();
  const bad = Object.keys(EN).filter((k) => vars(EN[k]) !== vars(ES[k]));
  assert.deepEqual(bad, []);
});

test("help texts stay at one short sentence", () => {
  const help = (k) => /(_help|_sub|\.sub|\.hint|\.privacy|_text)$/.test(k) && !/^capture\.c\d|consent/.test(k);
  const long = Object.keys(EN).filter(help).filter((k) => [EN[k], ES[k]].some((s) => s.length > 110 || /[.!?]\s+\p{Lu}/u.test(s)));
  assert.deepEqual(long, []);
});

test("no emoji in any string", () => {
  const all = Object.values(EN).concat(Object.values(ES)).join(" ") + code;
  assert.equal(/\p{Extended_Pictographic}/u.test(all), false);
});
