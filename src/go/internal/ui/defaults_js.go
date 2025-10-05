//go:build js && !test

package ui

// screenEdgesDefault remains false by default; enable SCREEN_EDGES via logs or
// env-like switches for diagnostics. For web, the cache path generally performs
// better unless many rotations are present.
var screenEdgesDefault = false

// Default pads for caches on web builds (in pixels). Larger edge pad reduces
// rebuilds during pans at the cost of slightly larger offscreen images.
const defaultEdgeCachePad = 128
const defaultGridCachePad = 64

// Enable simple draw by default on web builds for better perf.
const simpleDrawDefault = true

// Throttle timeline info text updates in web builds to reduce per-frame
// text rendering overhead. Value in milliseconds; 0 disables throttling.
const timelineInfoThrottleMS = 200

// Reduce visual effects on web to keep frame time stable.
const disableNodeGlow = true
const disableEdgeArrows = true
