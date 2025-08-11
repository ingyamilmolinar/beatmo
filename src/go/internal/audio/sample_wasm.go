//go:build js && wasm && !test

package audio

import "syscall/js"

// RegisterWAV loads a wav file via JavaScript and registers it.
func RegisterWAV(id, path string) error {
	js.Global().Call("loadWav", id, path)
	instrumentsMu.Lock()
	instruments = append(instruments, id)
	instrumentsMu.Unlock()
	return nil
}

// RegisterWAVDialog triggers a browser file picker and registers the selected WAV.
func RegisterWAVDialog(id string) error {
	js.Global().Call("selectWav", id)
	instrumentsMu.Lock()
	instruments = append(instruments, id)
	instrumentsMu.Unlock()
	return nil
}

// Sample type unused on wasm but kept for API parity.
type Sample struct{}
