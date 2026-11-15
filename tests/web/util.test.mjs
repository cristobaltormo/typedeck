import test from "node:test";
import assert from "node:assert/strict";
import { norm, hex, slug, clone } from "../../web/ui/js/util.js";

test("search folding ignores case and accents", () => {
  assert.equal(norm("Canción ÑANDÚ"), "cancion nandu");
  assert.equal(norm(undefined), "");
});

test("usages print as two hex digits", () => {
  assert.equal(hex(4), "04");
  assert.equal(hex(0xe7), "E7");
});

test("file names are safe slugs", () => {
  assert.equal(slug("Edición de vídeo / OBS"), "edicion-de-video-obs");
  assert.equal(slug("***"), "macros");
});

test("clone is deep", () => {
  const a = { x: { y: [1] } };
  const b = clone(a);
  b.x.y.push(2);
  assert.deepEqual(a.x.y, [1]);
});
