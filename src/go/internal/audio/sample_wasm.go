//go:build js && wasm && !test

package audio

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"syscall/js"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// RegisterWAV decodes a WAV the page can fetch (a file picker's object URL)
// at its OWN rate (JS decodeWavToPCM) and registers it through
// RegisterSamplePCM, which converts it to the engine rate with the same Go
// desktop uses — never with the browser's decodeAudioData resampler. The
// decode is asynchronous: the instrument exists now, its sound when decoded.
func RegisterWAV(id instrument.ID, path string) error {
	instrumentsMu.Lock()
	instruments = append(instruments, id)
	instrumentsMu.Unlock()
	go func() {
		pcm, sr, err := decodeWAVURL(path)
		if err != nil {
			log.Warnf("[audio] WAV %q for %s: %v", path, id, err)
			return
		}
		RegisterSamplePCM(id, pcm, sr)
	}()
	return nil
}

// decodeWAVURL decodes url to mono PCM at the file's own rate.
func decodeWAVURL(url string) ([]float32, int, error) {
	done := make(chan struct{})
	var pcm []float32
	var sr int
	var derr error
	then := js.FuncOf(func(this js.Value, args []js.Value) any {
		r := args[0]
		b := r.Get("bytes")
		raw := make([]byte, b.Get("byteLength").Int())
		js.CopyBytesToGo(raw, b)
		pcm = make([]float32, len(raw)/4)
		for i := range pcm {
			pcm[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[4*i:]))
		}
		sr = r.Get("sr").Int()
		close(done)
		return nil
	})
	catch := js.FuncOf(func(this js.Value, args []js.Value) any {
		derr = fmt.Errorf("decode failed: %s", args[0].Call("toString").String())
		close(done)
		return nil
	})
	defer then.Release()
	defer catch.Release()
	js.Global().Call("decodeWavToPCM", url).Call("then", then, catch)
	<-done
	return pcm, sr, derr
}

// RegisterAudio mirrors RegisterWAV for API parity.
func RegisterAudio(id instrument.ID, path string) error {
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
