/**
 * Unit tests for computer-action normalization (pure, no browser).
 * Run with: node --test src/js/llm_test/action_normalize.test.mjs
 *
 * Regression guard for the real mobile bug: Haiku emitted name="left_click"
 * with coordinate="[568, 592]", which silently no-op'd every tap.
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import { normCoord, normalizeComputerAction } from "./action_normalize.js";

test("normCoord: arrays, JSON-strings, loose strings, undefined", () => {
  assert.deepEqual(normCoord([568, 592]), [568, 592]);
  assert.deepEqual(normCoord("[568, 592]"), [568, 592]);
  assert.deepEqual(normCoord("568,592"), [568, 592]);
  assert.deepEqual(normCoord("(568, 592)"), [568, 592]);
  assert.equal(normCoord(undefined), undefined);
});

test("normalizeComputerAction: canonical computer tool passes through", () => {
  const out = normalizeComputerAction("computer", { action: "left_click", coordinate: [10, 20] });
  assert.equal(out.action, "left_click");
  assert.deepEqual(out.coordinate, [10, 20]);
});

test("normalizeComputerAction: bare action verb becomes the action (the bug)", () => {
  // Exactly the malformed call captured from a real Haiku mobile run.
  const out = normalizeComputerAction("left_click", { coordinate: "[568, 592]" });
  assert.equal(out.action, "left_click");
  assert.deepEqual(out.coordinate, [568, 592]);
});

test("normalizeComputerAction: key/type verbs route through", () => {
  assert.equal(normalizeComputerAction("key", { key: "Escape" }).action, "key");
  assert.equal(normalizeComputerAction("type", { text: "90" }).action, "type");
});

test("normalizeComputerAction: tap/click synonyms map to left_click", () => {
  assert.equal(normalizeComputerAction("tap", { coordinate: [1, 2] }).action, "left_click");
  assert.equal(normalizeComputerAction("click", { coordinate: [1, 2] }).action, "left_click");
});

test("normalizeComputerAction: bare coordinate with no action infers left_click", () => {
  const out = normalizeComputerAction("computer", { coordinate: "[3, 4]" });
  assert.equal(out.action, "left_click");
  assert.deepEqual(out.coordinate, [3, 4]);
});

test("normalizeComputerAction: left_click_drag normalizes both coordinates", () => {
  const out = normalizeComputerAction("left_click_drag", { start_coordinate: "[1, 2]", coordinate: "[3, 4]" });
  assert.equal(out.action, "left_click_drag");
  assert.deepEqual(out.start_coordinate, [1, 2]);
  assert.deepEqual(out.coordinate, [3, 4]);
});

test("normalizeComputerAction: does not mutate the input object", () => {
  const input = { coordinate: "[5, 6]" };
  normalizeComputerAction("left_click", input);
  assert.equal(input.coordinate, "[5, 6]"); // unchanged
});
