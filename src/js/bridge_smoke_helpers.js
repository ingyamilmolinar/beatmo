/**
 * Bridge-smoke helpers
 *
 * Shared assertion utilities for wasm_bridge_smoke.browser.test.js. The point
 * of the bridge smoke test is to verify the WASM↔JS plumbing — every export
 * is registered, callable, and returns a value of the expected shape — WITHOUT
 * re-asserting anything about the underlying Go logic. Logic correctness lives
 * in the Go test suite.
 *
 * If you're adding a new JS export in Go, add a smoke entry HERE (in the
 * catalogue used by wasm_bridge_smoke.browser.test.js) — do NOT create a
 * dedicated `*.browser.test.js` file just to verify the export exists.
 */

/**
 * Assert that `name` is a function in the page's global scope after WASM init.
 */
export async function assertExportExists(page, name) {
  const exists = await page.evaluate((n) => typeof globalThis[n] === "function", name);
  if (!exists) {
    throw new Error(`bridge-smoke: export '${name}' is not registered as a global function`);
  }
}

/**
 * Call `name(...args)` in the page and return its result. Throws if the call
 * throws, with the error message attributed to the export name.
 */
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

/**
 * Verify that a serialized return value matches an expected shape descriptor.
 *
 * Shape forms:
 *   "boolean" | "number" | "string" | "undefined" | "any"
 *   "array"                       — Array.isArray(v)
 *   "object"                      — typeof v === "object" && !Array.isArray(v)
 *   { kind: "object", keys: [...] } — required keys subset present
 *   "nullable"                    — null | undefined accepted (used for void exports)
 *
 * "any" matches anything (including undefined). Use sparingly.
 */
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

/**
 * Run a smoke entry: assert export exists, call it with args, verify return shape.
 */
export async function smokeExport(page, entry) {
  const { name, args = [], returns = "any", skipCall = false } = entry;
  await assertExportExists(page, name);
  if (skipCall) return;
  const value = await callExport(page, name, args);
  assertReturnShape(name, value, returns);
}
