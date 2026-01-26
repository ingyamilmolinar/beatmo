package ui

// registerGameForTest is a hook for tests to track created Game instances.
// It is a no-op in production builds unless overridden by test code.
var registerGameForTest = func(*Game) {}
