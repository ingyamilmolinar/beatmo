//go:build !js

package ui

// screenEdgesDefault is false on native builds; the world-space transform +
// edge-cache path remains the default for Ebiten desktop.
var screenEdgesDefault = false

const defaultEdgeCachePad = 64
const defaultGridCachePad = 64

// No throttling of timeline info on desktop; full fidelity is fine.
const timelineInfoThrottleMS = 0

// Full visuals on desktop.
const disableNodeGlow = false
const disableEdgeArrows = false

// Desktop can render full visuals; keep simple draw off by default.
const simpleDrawDefault = false
