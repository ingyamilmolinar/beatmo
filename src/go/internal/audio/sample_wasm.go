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

// SelectWAV triggers a browser file picker and returns the chosen file as an object URL.
func SelectWAV() (string, error) {
	doc := js.Global().Get("document")
	input := doc.Call("createElement", "input")
	input.Set("type", "file")
	input.Set("accept", ".wav")

	done := make(chan struct{})
	var path string
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
		url := js.Global().Get("URL").Call("createObjectURL", file)
		path = url.String()
		change.Release()
		close(done)
		return nil
	})

	input.Call("addEventListener", "change", change)
	input.Call("click")
	<-done
	if retErr != nil {
		return "", retErr
	}
	return path, nil
}

// Sample type unused on wasm but kept for API parity.
type Sample struct{}
