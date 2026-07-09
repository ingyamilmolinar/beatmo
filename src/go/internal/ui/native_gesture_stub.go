//go:build !js || test

package ui

// testCapturedNativeRects, when non-nil, receives every platformSyncNativeRects
// call (last write wins the slice contents) for test inspection.
var testCapturedNativeRects *[]NativeRect

func platformSyncNativeRects(rects []NativeRect) {
	if testCapturedNativeRects != nil {
		*testCapturedNativeRects = append((*testCapturedNativeRects)[:0], rects...)
	}
}
