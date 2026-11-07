/** The bridge smoke verifies plumbing (registered, callable, right shape), never Go logic. A new export gets an entry
 * in the catalogue, not a new test file. */

export async function assertExportExists(page, name) {
  const exists = await page.evaluate((n) => typeof globalThis[n] === "function", name);
  if (!exists) {
    throw new Error(`bridge-smoke: export '${name}' is not registered as a global function`);
  }
}

/** The error is attributed to the export name. */
export async function callExport(page, name, args = []) {
  const result = await page.evaluate(
    ({ n, a }) => {
      try {
        const fn = globalThis[n];
        if (typeof fn !== "function") {
          return { __error: `not a function: ${n}` };
        }
        const value = fn(...a);
        return { __ok: true, value: serializeForBridge(value) };
      } catch (err) {
        return { __error: String((err && err.message) || err) };
      }

      function serializeForBridge(v) {
        if (v === null || v === undefined) return v;
        const t = typeof v;
        if (t === "boolean" || t === "number" || t === "string") return v;
        if (Array.isArray(v)) {
          return { __kind: "array", length: v.length, sample: v.slice(0, 3).map(serializeForBridge) };
        }
        if (t === "object") {
          const keys = Object.keys(v);
          return { __kind: "object", keys: keys.slice(0, 32) };
        }
        return { __kind: t };
      }
    },
    { n: name, a: args },
  );
  if (result && result.__error) {
    throw new Error(`bridge-smoke: ${name}(${JSON.stringify(args)}) threw: ${result.__error}`);
  }
  return result.value;
}

/** Shapes: "boolean" | "number" | "string" | "undefined" | "any" | "array" | "object" | "nullable" (void exports) |
 * {kind:"object", keys:[...]} (required subset). "any" matches anything; use sparingly. */
export function assertReturnShape(name, value, shape) {
  if (shape === "any") return;
  if (shape === "nullable") {
    if (value === null || value === undefined) return;
    throw new Error(`bridge-smoke: ${name} expected null/undefined, got ${JSON.stringify(value)}`);
  }
  if (typeof shape === "string") {
    if (shape === "array") {
      if (!value || value.__kind !== "array") {
        throw new Error(`bridge-smoke: ${name} expected array, got ${JSON.stringify(value)}`);
      }
      return;
    }
    if (shape === "object") {
      if (!value || (value.__kind !== "object" && typeof value !== "object")) {
        throw new Error(`bridge-smoke: ${name} expected object, got ${JSON.stringify(value)}`);
      }
      return;
    }
    if (shape === "undefined") {
      if (value !== undefined && value !== null) {
        throw new Error(`bridge-smoke: ${name} expected undefined/null, got ${JSON.stringify(value)}`);
      }
      return;
    }
    if (typeof value !== shape) {
      throw new Error(`bridge-smoke: ${name} expected typeof ${shape}, got ${typeof value} (${JSON.stringify(value)})`);
    }
    return;
  }
  if (shape && shape.kind === "object") {
    if (!value || value.__kind !== "object") {
      throw new Error(`bridge-smoke: ${name} expected object, got ${JSON.stringify(value)}`);
    }
    const keys = value.keys || [];
    for (const required of shape.keys || []) {
      if (!keys.includes(required)) {
        throw new Error(
          `bridge-smoke: ${name} expected object key '${required}', got keys ${JSON.stringify(keys)}`,
        );
      }
    }
    return;
  }
  throw new Error(`bridge-smoke: unknown shape descriptor ${JSON.stringify(shape)} for ${name}`);
}

export async function smokeExport(page, entry) {
  const { name, args = [], returns = "any", skipCall = false } = entry;
  await assertExportExists(page, name);
  if (skipCall) return;
  const value = await callExport(page, name, args);
  assertReturnShape(name, value, returns);
}
