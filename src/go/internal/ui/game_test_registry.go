package ui

// registerGameForTest is a hook for tests to track created Game instances.
// It is a no-op in production builds unless overridden by test code.
var registerGameForTest = func(*Game) {}

// unregisterGameForTest is the matching hook Close calls so a closed Game
// leaves the test registry and becomes collectable. Without it the registry
// retained EVERY Game a package's tests ever built until TestMain's final
// drain — a full internal/ui run held thousands of closed Games (their layer
// caches included) and ran out of memory once enough scene-sweep tests
// existed. No-op outside tests.
var unregisterGameForTest = func(*Game) {}
