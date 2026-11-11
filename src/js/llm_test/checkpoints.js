/**
 * Objective checkpoint evaluation for the agent sanity tests.
 *
 * A "checkpoint" compares an `expect` object against the live engine state read
 * from fullLayoutSnapshot(). This converts the agent's subjective prose ("looks
 * like it played") into machine-verifiable facts ("state.isPlaying === true").
 *
 * The agent records checkpoints via the `checkpoint` tool (see agent.js); the
 * gate (evaluateGate) decides overall PASS/FAIL from the recorded ledger plus
 * the parsed "### PHASE RESULTS" block. cli.js turns the gate verdict into a
 * process exit code so a real regression fails CI instead of silently passing.
 */

/** Loose equality: numbers compare numerically, booleans as booleans, else as strings. */
function valuesEqual(actual, expected) {
  if (typeof expected === "number") return Number(actual) === expected;
  if (typeof expected === "boolean") return Boolean(actual) === expected;
  return String(actual) === String(expected);
}

/**
 * Match `actual` against `expected`. `expected` is either a literal (exact match
 * via valuesEqual) or a relational operator object — `{gt|lt|gte|lte|eq|ne: value}` —
 * used by gesture/stress tests whose result is directional, not exact
 * (e.g. {camScale:{gt:1.0}}, {camOffsetX:{ne:12.7}}, {totalNodes:{gt:4}}).
 * Multiple operators on one key are AND-ed. Unknown operators fail closed.
 */
export function matchesExpectation(actual, expected) {
  if (expected !== null && typeof expected === "object" && !Array.isArray(expected)) {
    const entries = Object.entries(expected);
    if (entries.length === 0) return false;
    return entries.every(([op, v]) => {
      const a = Number(actual);
      const b = Number(v);
      switch (op) {
        case "gt": return a > b;
        case "lt": return a < b;
        case "gte": return a >= b;
        case "lte": return a <= b;
        case "eq": return valuesEqual(actual, v);
        case "ne": return !valuesEqual(actual, v);
        default: return false;
      }
    });
  }
  return valuesEqual(actual, expected);
}

/**
 * Compare an `expect` object against a fullLayoutSnapshot() result.
 *
 * Supported scalar keys (compared against snap.state.<key>):
 *   bpm, subdiv, totalRows, isPlaying, activeTab, viewMode, channel
 * Per-row keys (compared against snap.rows[row].<key>):
 *   { row: N, muted?: bool, soloed?: bool }
 *
 * @param {Object|null} snap - result of fullLayoutSnapshot()
 * @param {Object} expect
 * @returns {{pass: boolean, mismatches: Array<{key:string, expected:any, actual:any}>}}
 */
export function compareCheckpoint(snap, expect) {
  const mismatches = [];
  if (!snap || !snap.state) {
    return { pass: false, mismatches: [{ key: "snapshot", expected: "present", actual: snap ? "no .state" : "null" }] };
  }
  const st = snap.state;
  const scalarKeys = [
    "bpm", "subdiv", "totalRows", "totalNodes", "isPlaying", "activeTab", "viewMode",
    "channel", "camScale", "camOffsetX", "camOffsetY",
  ];
  for (const k of scalarKeys) {
    if (expect[k] === undefined || expect[k] === null) continue;
    if (!matchesExpectation(st[k], expect[k])) {
      mismatches.push({ key: k, expected: expect[k], actual: st[k] });
    }
  }

  if (expect.row !== undefined && expect.row !== null) {
    const rows = snap.rows || [];
    const r = rows[expect.row];
    if (!r) {
      mismatches.push({ key: `row[${expect.row}]`, expected: "present", actual: "missing" });
    } else {
      if (expect.muted !== undefined && Boolean(r.muted) !== Boolean(expect.muted)) {
        mismatches.push({ key: `row[${expect.row}].muted`, expected: expect.muted, actual: r.muted });
      }
      if (expect.soloed !== undefined && Boolean(r.soloed) !== Boolean(expect.soloed)) {
        mismatches.push({ key: `row[${expect.row}].soloed`, expected: expect.soloed, actual: r.soloed });
      }
    }
  }

  return { pass: mismatches.length === 0, mismatches };
}

/** Render a mismatch list to a single human-readable line. */
export function formatMismatches(mismatches) {
  if (!mismatches || mismatches.length === 0) return "(no detail)";
  return mismatches
    .map((m) => `${m.key}: expected ${JSON.stringify(m.expected)}, got ${JSON.stringify(m.actual)}`)
    .join("; ");
}

/**
 * Parse the agent's "### PHASE RESULTS" block from its final summary.
 * Returns [{phase:number, result:"PASS"|"PARTIAL"|"FAIL", note:string}].
 */
export function extractPhaseResults(summary) {
  if (!summary) return [];
  const results = [];
  for (const line of summary.split("\n")) {
    const m = line.match(/Phase\s+(\d+)\s*:\s*(PASS|PARTIAL|FAIL)\s*[-–—]?\s*(.*)/i);
    if (m) {
      results.push({ phase: parseInt(m[1], 10), result: m[2].toUpperCase(), note: m[3].trim() });
    }
  }
  return results;
}

/**
 * Evaluate the overall pass/fail gate for an agent run.
 *
 * The gate fails if ANY of:
 *   - the agent threw a fatal error,
 *   - any recorded checkpoint failed,
 *   - a required checkpoint was never recorded,
 *   - a phase self-reported FAIL.
 *
 * @param {Object} result - agent result { checkpoints?, summary?, error? }
 * @param {string[]} requiredCheckpoints - names that MUST be present and passing
 * @returns {{ok:boolean, reasons:string[], checkpointsPassed:number,
 *            checkpointsFailed:number, missing:string[], phaseFails:Array}}
 */
export function evaluateGate(result, requiredCheckpoints = []) {
  const reasons = [];
  const ledger = Array.isArray(result?.checkpoints) ? result.checkpoints : [];
  // Collapse by name, LAST verdict wins. A checkpoint may be recorded more than
  // once when the agent retries after a transient FAIL (e.g. a value that takes
  // an extra frame to commit) and then self-corrects to PASS. Only the final
  // verdict counts — otherwise legitimate recovery would fail the gate.
  const finalByName = new Map();
  for (const c of ledger) finalByName.set(c.name, c);

  let checkpointsPassed = 0;
  let checkpointsFailed = 0;
  for (const [name, c] of finalByName) {
    if (c.pass) {
      checkpointsPassed++;
    } else {
      checkpointsFailed++;
      reasons.push(`checkpoint "${name}" failed: ${formatMismatches(c.mismatches)}`);
    }
  }
  const checkpointsTotal = finalByName.size;

  const missing = [];
  for (const name of requiredCheckpoints) {
    if (!finalByName.has(name)) {
      missing.push(name);
      reasons.push(`required checkpoint "${name}" was never recorded`);
    }
  }

  if (result?.error) reasons.push(`agent error: ${result.error}`);

  const phases = extractPhaseResults(result?.summary ?? "");
  const phaseFails = phases.filter((p) => p.result === "FAIL");
  for (const p of phaseFails) reasons.push(`Phase ${p.phase} reported FAIL — ${p.note}`);

  const ok = reasons.length === 0;
  return { ok, reasons, checkpointsPassed, checkpointsFailed, checkpointsTotal, missing, phaseFails };
}
