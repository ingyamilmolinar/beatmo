//go:build js && wasm && !test

package audio

import (
	"strings"
	"syscall/js"
)

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
	doc := js.Global().Get("document")
	input := doc.Call("createElement", "input")
	input.Set("type", "file")
	input.Set("accept", ".wav")

	var change js.Func
	change = js.FuncOf(func(this js.Value, args []js.Value) any {
		files := input.Get("files")
		if files.Length() == 0 {
			change.Release()
			return nil
		}
		file := files.Index(0)
		name := strings.ToLower(file.Get("name").String())
		if !strings.HasSuffix(name, ".wav") {
			js.Global().Get("console").Call("error", "Invalid file selected")
			change.Release()
			return nil
		}
		url := js.Global().Get("URL").Call("createObjectURL", file)
		js.Global().Call("loadWav", id, url)
		instrumentsMu.Lock()
		instruments = append(instruments, id)
		instrumentsMu.Unlock()
		change.Release()
		return nil
	})

	input.Call("addEventListener", "change", change)
	input.Call("click")
	return nil
}

// Sample type unused on wasm but kept for API parity.
type Sample struct{}
