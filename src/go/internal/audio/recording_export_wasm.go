//go:build js && !test

package audio

import (
	"fmt"
	"syscall/js"

	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// The zip Blob lives in the JS encoder Worker; only its URL and filename are fetched, so no bytes cross the Go-WASM
// boundary. Call it close to the stop click, or browser policy may gate the download.
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
	log.Debugf("[record] triggered download: %s (%d bytes)", handle.Filename, handle.Size)
	return handle.Filename, nil
}

// triggerRecordingDownload dispatches the anchor.click in JS. Wraps the
// JS-side window.recordingTriggerDownload helper so the Go side never
// touches the DOM directly.
func triggerRecordingDownload(blobURL, filename string) bool {
	fn := js.Global().Get("recordingTriggerDownload")
	if !fn.Truthy() {
		log.Warnf("[record] recordingTriggerDownload not available")
		return false
	}
	v := fn.Invoke(blobURL, filename)
	return v.Truthy() && v.Bool()
}
