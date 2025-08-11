//go:build js && wasm && !test

package audio

import (
	"fmt"
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

// RegisterWAVDialog triggers a browser file picker, asks for a name, and registers the selected WAV.
func RegisterWAVDialog() (string, error) {
	doc := js.Global().Get("document")
	input := doc.Call("createElement", "input")
	input.Set("type", "file")
	input.Set("accept", ".wav")

	done := make(chan struct{})
	var result string
	var retErr error

	var change js.Func
	change = js.FuncOf(func(this js.Value, args []js.Value) any {
		files := input.Get("files")
		if files.Length() == 0 {
			retErr = fmt.Errorf("no file selected")
			change.Release()
			close(done)
			return nil
		}
		file := files.Index(0)
		fname := strings.ToLower(file.Get("name").String())
		if !strings.HasSuffix(fname, ".wav") {
			retErr = fmt.Errorf("invalid file selected")
			change.Release()
			close(done)
			return nil
		}
		prompt := js.Global().Call("prompt", "Instrument name?")
		if !prompt.Truthy() {
			retErr = fmt.Errorf("instrument name is required")
			change.Release()
			close(done)
			return nil
		}
		id := strings.TrimSpace(prompt.String())
		if id == "" {
			retErr = fmt.Errorf("instrument name is required")
			change.Release()
			close(done)
			return nil
		}
		url := js.Global().Get("URL").Call("createObjectURL", file)
		js.Global().Call("loadWav", id, url)
		instrumentsMu.Lock()
		instruments = append(instruments, id)
		instrumentsMu.Unlock()
		result = id
		change.Release()
		close(done)
		return nil
	})

	input.Call("addEventListener", "change", change)
	input.Call("click")
	<-done
	return result, retErr
}

// Sample type unused on wasm but kept for API parity.
type Sample struct{}
