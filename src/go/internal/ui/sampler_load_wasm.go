//go:build js && wasm && !test

package ui

import (
	"syscall/js"
	"unsafe"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func init() { samplerWAVLoadFn = browserSamplerLoadWAV }

// browserSamplerLoadWAV opens the browser file picker (object URL) and decodes
// it to PCM via the decodeWavToPCM JS bridge. It blocks the calling goroutine
// on the decode Promise — safe under Go's cooperative wasm scheduler, mirroring
// audio.SelectWAV's own channel-blocking pattern.
func browserSamplerLoadWAV() ([]float32, int, bool) {
	url, err := audio.SelectWAV()
	if err != nil || url == "" {
		return nil, 0, false
	}
	fn := js.Global().Get("decodeWavToPCM")
	if !fn.Truthy() {
		return nil, 0, false
	}
	done := make(chan struct{})
	var pcm []float32
	var sr int
	then := js.FuncOf(func(_ js.Value, args []js.Value) interface{} {
		if len(args) > 0 && args[0].Truthy() {
			res := args[0]
			n := res.Get("length").Int()
			u8 := res.Get("bytes")
			if n > 0 && u8.Truthy() {
				scratch := make([]byte, n*4)
				js.CopyBytesToGo(scratch, u8)
				f := unsafe.Slice((*float32)(unsafe.Pointer(&scratch[0])), n)
				pcm = make([]float32, n)
				copy(pcm, f)
				sr = res.Get("sr").Int()
			}
		}
		close(done)
		return nil
	})
	catch := js.FuncOf(func(_ js.Value, _ []js.Value) interface{} {
		close(done)
		return nil
	})
	defer then.Release()
	defer catch.Release()
	fn.Invoke(url).Call("then", then, catch)
	<-done
	if len(pcm) == 0 || sr <= 0 {
		return nil, 0, false
	}
	return pcm, sr, true
}
