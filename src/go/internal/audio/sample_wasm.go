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

// RegisterAudio mirrors RegisterWAV for API parity; decoding is delegated to JS.
func RegisterAudio(id, path string) error {
	return RegisterWAV(id, path)
}

// SelectWAV triggers a browser file picker and returns the chosen file as an object URL.
// On mobile, it first checks for a pending pick from the gesture-based rect system
// via openWAVFile(), which consumes the pending result. Falls back to the legacy
// direct file input approach if openWAVFile is not available.
func SelectWAV() (string, error) {
	g := js.Global()
	openWAV := g.Get("openWAVFile")
	if openWAV.Truthy() {
		done := make(chan struct{})
		var path string
		var retErr error
		then := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			if len(args) > 0 && args[0].Truthy() {
				url := args[0].Get("url")
				if url.Truthy() {
					path = url.String()
				} else {
					retErr = fmt.Errorf("no file selected")
				}
			} else {
				retErr = fmt.Errorf("no file selected")
			}
			close(done)
			return nil
		})
		catch := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			retErr = fmt.Errorf("file picker error")
			close(done)
			return nil
		})
		g.Set("__wavPickerThen", then)
		g.Set("__wavPickerCatch", catch)
		openWAV.Invoke().Call("then", then, catch)
		<-done
		if retErr != nil {
			return "", retErr
		}
		return path, nil
	}
	return selectWAVLegacy()
}

// selectWAVLegacy is the original file picker implementation using direct input.click().
// Used as fallback when openWAVFile is not available in the JS environment.
func selectWAVLegacy() (string, error) {
	doc := js.Global().Get("document")
	input := doc.Call("createElement", "input")
	input.Set("type", "file")
	input.Set("accept", ".wav")

	done := make(chan struct{})
	var path string
	var retErr error

	var change js.Func
	change = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
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
