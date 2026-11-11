// scenarios/registry.js
//
// Tiny shared registry so a single in-page test runner can discover scenarios
// extracted from existing Playwright tests without each test file having to
// know about the runner.
//
// Each scenario module imports { register } from this file and calls it once
// at module top-level for every scenario it exports:
//
//   import { register } from "./registry.js";
//   export async function scenarioFoo() { ... return { pass, ... }; }
//   register({
//     suite: "webaudio_output_capture",
//     name:  "capture_roundtrip",
//     fn:    scenarioFoo,
//   });
//
// The Playwright shell imports the scenario function directly and calls it
// inside page.evaluate. The phone runner just imports the scenario module
// (any side-effect call to register() pushes the scenario into the registry)
// and walks the registry to render buttons + run-all controls.
//
// A scenario must return a plain JSON-serializable result of the shape:
//   { pass: boolean, reason?: string, ...details }
// — throw only for setup bugs the user cannot fix from the UI.

const _scenarios = [];

export function register(entry) {
  if (!entry || typeof entry.fn !== "function" || !entry.name) {
    throw new Error("scenarios/register: requires { suite, name, fn }");
  }
  // Replace any previous registration with the same (suite, name) so hot
  // reloads don't accumulate duplicates.
  const idx = _scenarios.findIndex((s) => s.suite === entry.suite && s.name === entry.name);
  const record = {
    suite: entry.suite || "uncategorized",
    name: entry.name,
    fn: entry.fn,
    description: entry.description || "",
  };
  if (idx >= 0) _scenarios[idx] = record;
  else _scenarios.push(record);
}

export function list() {
  return _scenarios.slice();
}

export function find(suite, name) {
  return _scenarios.find((s) => s.suite === suite && s.name === name) || null;
}

// Optional aliasing so a runner can do window.__scenarios.list() / .run() in
// a non-module context (e.g. devtools console on the phone).
if (typeof window !== "undefined") {
  window.__scenarios = { register, list, find };
}
