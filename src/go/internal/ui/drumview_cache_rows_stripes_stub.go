package ui

// drumview_cache_rows_stripes_stub.go — temporary scaffolding while the
// rows-stripes refactor is in flight. The original stripes implementation
// (drumview_cache_rows_stripes.go) was removed; consumers in
// js_exports_*.go, image_metrics.go, and *_test.go still reference its
// fields/methods/constants. These stubs make the package build with
// no-op semantics: the layer-based render path runs, the stripes path
// never activates. Delete this file once the refactor either restores
// the implementation or removes all callers.

// wasmStripeTargetPx is the desired horizontal width per stripe (pixels).
// 0 effectively disables the stripes path (callers that compute
// "len/wasmStripeTargetPx" will divide by zero, but the tests gate
// stripes by checking dv.rowsStripingEnabled which stays false).
const wasmStripeTargetPx = 1024

// wasmStripeMaxCount caps the number of stripes — kept low while stubbed.
const wasmStripeMaxCount = 8

// rowsStripesMaybeRebuild is a no-op stub. Returns false (stripes never
// rebuilt) so the layer path remains the canonical render route.
func (dv *DrumView) rowsStripesMaybeRebuild() bool { return false }
