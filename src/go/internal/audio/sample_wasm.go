//go:build js && wasm && !test

package audio

import "syscall/js"

// RegisterWAV loads a wav file via JavaScript and registers it.
func RegisterWAV(id, path string) error {
	js.Global().Call("loadWav", id, path)
	instruments = append(instruments, id)
	return nil
}

// Sample type unused on wasm but kept for API parity.
type Sample struct{}
