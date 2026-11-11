/**
 * Unit tests for the objective checkpoint + gate logic (pure, no browser).
 * Run with: node --test src/js/llm_test/checkpoints.test.mjs
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import { compareCheckpoint, evaluateGate, extractPhaseResults } from "./checkpoints.js";

const snap = {
  state: {
    bpm: 90, subdiv: 16, totalRows: 6, totalNodes: 53, isPlaying: true,
    activeTab: "wave", viewMode: "pads", channel: "main",
    camScale: 1.5, camOffsetX: 40, camOffsetY: -12,
  },
  rows: [{ muted: false, soloed: false }, { muted: true, soloed: false }, { muted: false, soloed: true }],
};

test("compareCheckpoint: scalar match passes", () => {
  assert.equal(compareCheckpoint(snap, { bpm: 90 }).pass, true);
  assert.equal(compareCheckpoint(snap, { subdiv: 16, isPlaying: true }).pass, true);
  assert.equal(compareCheckpoint(snap, { activeTab: "wave" }).pass, true);
});

test("compareCheckpoint: scalar mismatch fails with detail", () => {
  const r = compareCheckpoint(snap, { bpm: 91 });
  assert.equal(r.pass, false);
  assert.deepEqual(r.mismatches, [{ key: "bpm", expected: 91, actual: 90 }]);
});

test("compareCheckpoint: per-row mute/solo", () => {
  assert.equal(compareCheckpoint(snap, { row: 1, muted: true }).pass, true);
  assert.equal(compareCheckpoint(snap, { row: 0, muted: true }).pass, false);
  assert.equal(compareCheckpoint(snap, { row: 2, soloed: true }).pass, true);
  assert.equal(compareCheckpoint(snap, { row: 9, muted: true }).pass, false); // out of range
});

test("compareCheckpoint: relational operators (gesture/stress tests)", () => {
  // camScale=1.5, camOffsetX=40, totalNodes=53 in snap
  assert.equal(compareCheckpoint(snap, { camScale: { gt: 1.0 } }).pass, true);
  assert.equal(compareCheckpoint(snap, { camScale: { lt: 1.0 } }).pass, false);
  assert.equal(compareCheckpoint(snap, { camOffsetX: { ne: 0 } }).pass, true);
  assert.equal(compareCheckpoint(snap, { camOffsetX: { ne: 40 } }).pass, false);
  assert.equal(compareCheckpoint(snap, { totalNodes: { gte: 53, lte: 53 } }).pass, true);
  assert.equal(compareCheckpoint(snap, { totalNodes: { gt: 53 } }).pass, false);
  // exact still works alongside the new keys
  assert.equal(compareCheckpoint(snap, { totalNodes: 53, camScale: { gte: 1.5 } }).pass, true);
});

test("compareCheckpoint: unknown operator fails closed", () => {
  const r = compareCheckpoint(snap, { camScale: { approx: 1.5 } });
  assert.equal(r.pass, false);
});

test("compareCheckpoint: missing snapshot fails closed", () => {
  assert.equal(compareCheckpoint(null, { bpm: 90 }).pass, false);
  assert.equal(compareCheckpoint({}, { bpm: 90 }).pass, false);
});

test("extractPhaseResults parses the PHASE RESULTS block", () => {
  const summary = "### PHASE RESULTS\n- Phase 1: PASS — ok\n- Phase 2: FAIL — broke\n- Phase 3: PARTIAL — meh";
  assert.deepEqual(extractPhaseResults(summary), [
    { phase: 1, result: "PASS", note: "ok" },
    { phase: 2, result: "FAIL", note: "broke" },
    { phase: 3, result: "PARTIAL", note: "meh" },
  ]);
});

test("evaluateGate: fails on failed checkpoint, missing required, and phase FAIL", () => {
  const res = {
    checkpoints: [
      { name: "x", pass: true },
      { name: "y", pass: false, mismatches: [{ key: "bpm", expected: 90, actual: 120 }] },
    ],
    summary: "### PHASE RESULTS\n- Phase 1: PASS — ok\n- Phase 2: FAIL — broke",
  };
  const g = evaluateGate(res, ["x", "z"]);
  assert.equal(g.ok, false);
  assert.equal(g.checkpointsPassed, 1);
  assert.equal(g.checkpointsFailed, 1);
  assert.deepEqual(g.missing, ["z"]);
  assert.equal(g.phaseFails.length, 1);
  assert.ok(g.reasons.length >= 3);
});

test("evaluateGate: passes when all required checkpoints pass and no FAIL phases", () => {
  const res = {
    checkpoints: [{ name: "x", pass: true }, { name: "y", pass: true }],
    summary: "### PHASE RESULTS\n- Phase 1: PASS — ok",
  };
  assert.equal(evaluateGate(res, ["x", "y"]).ok, true);
});

test("evaluateGate: agent error fails the gate", () => {
  assert.equal(evaluateGate({ checkpoints: [], error: "boom" }, []).ok, false);
});

test("evaluateGate: retry then pass — final verdict wins, gate passes", () => {
  // Agent recorded bpm_90 twice: a transient FAIL, then a PASS after blur-commit.
  const res = {
    checkpoints: [
      { name: "init", pass: true },
      { name: "bpm_90", pass: false, mismatches: [{ key: "bpm", expected: 90, actual: 120 }] },
      { name: "bpm_90", pass: true },
    ],
    summary: "### PHASE RESULTS\n- Phase 1: PASS — ok",
  };
  const g = evaluateGate(res, ["init", "bpm_90"]);
  assert.equal(g.ok, true, `expected pass, reasons: ${JSON.stringify(g.reasons)}`);
  assert.equal(g.checkpointsTotal, 2); // unique names
  assert.equal(g.checkpointsPassed, 2);
  assert.equal(g.checkpointsFailed, 0);
});

test("evaluateGate: pass then later FAIL — final verdict (FAIL) wins", () => {
  const res = {
    checkpoints: [
      { name: "x", pass: true },
      { name: "x", pass: false, mismatches: [{ key: "bpm", expected: 90, actual: 100 }] },
    ],
  };
  const g = evaluateGate(res, ["x"]);
  assert.equal(g.ok, false);
  assert.equal(g.checkpointsFailed, 1);
});
