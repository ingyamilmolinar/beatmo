//go:build !js || test

package ui

// Last write wins.
var testCapturedNativeRects *[]NativeRect

func platformSyncNativeRects(rects []NativeRect) {
	if testCapturedNativeRects != nil {
		*testCapturedNativeRects = append((*testCapturedNativeRects)[:0], rects...)
	}
}
