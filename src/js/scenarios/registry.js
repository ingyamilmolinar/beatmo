// One in-page runner discovers scenarios without test files knowing about it: each module calls register({suite, name,
// fn}) at top level. A scenario returns a JSON-serializable {pass, reason?, …} and throws only for setup bugs the user
// cannot fix from the UI.

const _scenarios = [];

export function register(entry) {
  if (!entry || typeof entry.fn !== "function" || !entry.name) {
    throw new Error("scenarios/register: requires { suite, name, fn }");
  }
  // So hot reloads do not accumulate duplicates.
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

// For a non-module context such as the phone's devtools console.
if (typeof window !== "undefined") {
  window.__scenarios = { register, list, find };
}
