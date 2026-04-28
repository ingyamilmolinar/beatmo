//go:build js && !test

package audio

import (
	"fmt"
	"log"
	"syscall/js"
)

// SaveRecording triggers the browser download for the most recent
// recording. The encoded zip Blob lives entirely in the JS encoder
// Worker; here we only fetch the cached object URL + filename and ask
// the JS download helper to dispatch the anchor.click. No bytes cross
// the Go-WASM boundary on this path — zero memcpy, zero re-encoding.
//
// Must be called from a goroutine that's reasonably close in time to
// the user-gesture event (the stop-button click) for the download to
// fire without browser policy gating. The full Stop → SaveRecording
// path completes in well under one second with worker-side encoding,
// well within the user-activation window.
//
// Returns the zip filename on success, error on failure.
func SaveRecording(result *RecordingResult) (string, error) {
	if result == nil {
		return "", fmt.Errorf("nil recording result")
	}
	handle := pendingDownload.Swap(nil)
	if handle == nil {
		return "", fmt.Errorf("no recording blob available to save")
	}
	if !triggerRecordingDownload(handle.BlobURL, handle.Filename) {
		// Restore the handle so a subsequent retry can pick it up.
		pendingDownload.Store(handle)
		return "", fmt.Errorf("download trigger failed")
	}
	log.Printf("[RECORDING] Triggered download: %s (%d bytes)", handle.Filename, handle.Size)
	return handle.Filename, nil
}

// triggerRecordingDownload dispatches the anchor.click in JS. Wraps the
// JS-side window.recordingTriggerDownload helper so the Go side never
// touches the DOM directly.
func triggerRecordingDownload(blobURL, filename string) bool {
	fn := js.Global().Get("recordingTriggerDownload")
	if !fn.Truthy() {
		log.Println("[RECORDING] recordingTriggerDownload not available")
		return false
	}
	v := fn.Invoke(blobURL, filename)
	return v.Truthy() && v.Bool()
}
