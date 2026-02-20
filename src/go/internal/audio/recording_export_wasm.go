//go:build js && !test

package audio

import (
	"archive/zip"
	"bytes"
	"fmt"
	"log"
	"syscall/js"
)

// SaveRecording bundles all encoded channels into a zip and triggers a browser download.
// Returns the zip filename.
func SaveRecording(result *RecordingResult) (string, error) {
	if result == nil || len(result.Channels) == 0 {
		return "", fmt.Errorf("no channels to save")
	}

	zipName := "beatmo-recording-" + result.Metadata.Timestamp + ".zip"

	data, err := bundleRecordingZip(result)
	if err != nil {
		return "", fmt.Errorf("failed to create zip: %w", err)
	}

	triggerZipDownload(zipName, data)
	log.Printf("[RECORDING] Triggered download: %s (%d bytes)", zipName, len(data))
	return zipName, nil
}

// bundleRecordingZip creates a zip archive containing all channel files + metadata.
func bundleRecordingZip(result *RecordingResult) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	for _, ch := range result.Channels {
		w, err := zw.Create(ch.Filename)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(ch.Data); err != nil {
			return nil, err
		}
	}

	// Add session metadata
	metaJSON, err := result.MetadataJSON()
	if err == nil {
		w, err := zw.Create("session.json")
		if err == nil {
			w.Write(metaJSON)
		}
	}

	if err := zw.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// triggerZipDownload triggers a browser download of the zip data.
func triggerZipDownload(name string, data []byte) {
	g := js.Global()

	uint8Array := g.Get("Uint8Array")
	blobCtor := g.Get("Blob")
	urlObj := g.Get("URL")
	doc := g.Get("document")

	if !uint8Array.Truthy() || !blobCtor.Truthy() || !urlObj.Truthy() || !doc.Truthy() {
		log.Println("[RECORDING] Warning: browser APIs not available for download")
		return
	}

	buf := uint8Array.New(len(data))
	js.CopyBytesToJS(buf, data)

	opts := g.Get("Object").New()
	opts.Set("type", "application/zip")
	blob := blobCtor.New([]any{buf}, opts)
	href := urlObj.Call("createObjectURL", blob)

	a := doc.Call("createElement", "a")
	a.Set("href", href)
	a.Set("download", name)
	doc.Get("body").Call("appendChild", a)
	a.Call("click")

	// Cleanup after short delay
	var revoke js.Func
	revoke = js.FuncOf(func(this js.Value, args []js.Value) any {
		urlObj.Call("revokeObjectURL", href)
		a.Call("remove")
		revoke.Release()
		return nil
	})
	if st := g.Get("setTimeout"); st.Truthy() {
		st.Invoke(revoke, 100)
	} else {
		revoke.Invoke(js.Undefined(), nil)
	}
}
