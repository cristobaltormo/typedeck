import test from "node:test";
import assert from "node:assert/strict";
import { createRecorder } from "../../web/ui/js/recorder.js";

const ev = (key, code, extra = {}) => ({ key, code, ctrlKey: false, altKey: false, shiftKey: false, metaKey: false, repeat: false, ...extra });

test("typed characters become one text step", () => {
  const r = createRecorder();
  [["h", "KeyH"], ["o", "KeyO"], ["l", "KeyL"], ["a", "KeyA"]].forEach(([k, c], i) => r.key(ev(k, c), 100 + i * 90));
  assert.deepEqual(r.steps, [{ type: "text", text: "hola" }]);
});

test("a shortcut keeps its modifiers and uses the physical key", () => {
  const r = createRecorder();
  r.key(ev("$", "Digit4", { metaKey: true, shiftKey: true }), 100);
  assert.deepEqual(r.steps, [{ type: "hotkey", keys: "shift+cmd+4" }]);
});

test("pauses become wait steps rounded to 50 ms and short ones are dropped", () => {
  const r = createRecorder();
  r.key(ev("t", "KeyT", { ctrlKey: true }), 0);
  r.key(ev("Enter", "Enter"), 120);
  r.key(ev("Escape", "Escape"), 1000);
  assert.deepEqual(r.steps.map((s) => s.type + ":" + (s.keys || s.ms)), ["hotkey:ctrl+t", "hotkey:enter", "wait:900", "hotkey:esc"]);
});

test("a long pause inside a text splits it with a wait", () => {
  const r = createRecorder();
  r.key(ev("a", "KeyA"), 0);
  r.key(ev("b", "KeyB"), 3000);
  assert.deepEqual(r.steps, [{ type: "text", text: "a" }, { type: "wait", ms: 3000 }, { type: "text", text: "b" }]);
});

test("timings off records no waits; modifiers and repeats are ignored", () => {
  const r = createRecorder({ timings: false });
  r.key(ev("Shift", "ShiftLeft"), 0);
  r.key(ev("a", "KeyA", { repeat: true }), 10);
  r.key(ev("F5", "F5"), 5000);
  assert.deepEqual(r.steps, [{ type: "hotkey", keys: "f5" }]);
});

test("stops growing at the step limit", () => {
  const r = createRecorder({ max: 2, timings: false });
  r.key(ev("F1", "F1"), 0); r.key(ev("F2", "F2"), 1); r.key(ev("F3", "F3"), 2);
  assert.equal(r.steps.length, 2);
  assert.equal(r.full, true);
});
