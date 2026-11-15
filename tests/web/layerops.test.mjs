import test from "node:test";
import assert from "node:assert/strict";
import { moveLayer, removeLayer, insertLayer, looksLikeConfig, MAX_LAYERS } from "../../web/ui/js/layerops.js";

const layer = (name, keys = {}) => ({ name, color: "", icon: "", auto_apps: [], keys });
const to = (n) => ({ tap: { type: "layer", to: n } });
const cfg = () => ({
  settings: {},
  global: { "53": to(2), "54": { tap: { type: "layer", to: "next" } } },
  layers: [layer("A", { "04": to(1) }), layer("B", { "05": { tap: { type: "sequence", steps: [{ type: "if", cond: "layer", target: "3", steps: [to(0).tap], else: [] }] } } }), layer("C")],
});

test("moving a layer keeps every reference on the same layer", () => {
  const c = cfg();
  moveLayer(c, 2, 0);
  assert.deepEqual(c.layers.map((l) => l.name), ["C", "A", "B"]);
  assert.equal(c.global["53"].tap.to, 0);
  assert.equal(c.global["54"].tap.to, "next");
  assert.equal(c.layers[1].keys["04"].tap.to, 2);
  const cond = c.layers[2].keys["05"].tap.steps[0];
  assert.equal(cond.target, "1");
  assert.equal(cond.steps[0].to, 1);
});

test("removing a layer shifts the later references and repoints the removed one", () => {
  const c = cfg();
  removeLayer(c, 1);
  assert.deepEqual(c.layers.map((l) => l.name), ["A", "C"]);
  assert.equal(c.global["53"].tap.to, 1);
  assert.equal(c.layers[0].keys["04"].tap.to, 0);
});

test("the last layer cannot be removed", () => {
  const c = { global: {}, layers: [layer("A")] };
  removeLayer(c, 0);
  assert.equal(c.layers.length, 1);
});

test("inserting a layer shifts the references after it and respects the limit", () => {
  const c = cfg();
  assert.equal(insertLayer(c, 1, layer("New")), true);
  assert.deepEqual(c.layers.map((l) => l.name), ["A", "New", "B", "C"]);
  assert.equal(c.global["53"].tap.to, 3);
  assert.equal(c.layers[0].keys["04"].tap.to, 2);
  while (c.layers.length < MAX_LAYERS) insertLayer(c, c.layers.length, layer("x"));
  assert.equal(insertLayer(c, 0, layer("over")), false);
  assert.equal(c.layers.length, MAX_LAYERS);
});

test("only a full setup file is accepted as a configuration", () => {
  assert.equal(looksLikeConfig(cfg()), true);
  assert.equal(looksLikeConfig({ typedeck_macros: "1", layers: [layer("A")] }), false);
  assert.equal(looksLikeConfig({ layers: [] , settings: {} }), false);
  assert.equal(looksLikeConfig({ layers: [layer("A")] }), false);
  assert.equal(looksLikeConfig([1, 2]), false);
  assert.equal(looksLikeConfig(null), false);
});
