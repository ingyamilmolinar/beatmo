package ui

var registerGameForTest = func(*Game) {}

// Without it the registry keeps every closed Game (and its caches) until TestMain, and a full ui
// run runs out of memory.
var unregisterGameForTest = func(*Game) {}
