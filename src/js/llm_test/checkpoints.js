/** Turns the agent's prose into machine-checked facts: a checkpoint compares `expect` with fullLayoutSnapshot(), and the
 * gate makes the run PASS/FAIL so a real regression fails CI instead of silently passing. */

/** Loose equality: numbers compare numerically, booleans as booleans, else as strings. */
function valuesEqual(actual, expected) {
  if (typeof expected === "number") return Number(actual) === expected;
  if (typeof expected === "boolean") return Boolean(actual) === expected;
  return String(actual) === String(expected);
}

/** A literal, or {gt|lt|gte|lte|eq|ne: value} for directional results; several operators AND together, unknown ones
 * fail closed. */
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

/** Scalar keys (bpm, subdiv, totalRows, isPlaying, activeTab, viewMode, channel) against snap.state; {row, muted?,
 * soloed?} against snap.rows. */
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

/** [{phase, result: "PASS"|"PARTIAL"|"FAIL", note}]. */
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

/** Fails on a fatal error, any failed checkpoint, a required one never recorded, or a phase that self-reported FAIL. */
export function evaluateGate(result, requiredCheckpoints = []) {
  const reasons = [];
  const ledger = Array.isArray(result?.checkpoints) ? result.checkpoints : [];
  // LAST verdict wins: an agent that retries after a transient FAIL and recovers must pass.
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
