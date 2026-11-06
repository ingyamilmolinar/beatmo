//go:build js && wasm && !test

package audio

import (
	"encoding/binary"
	"errors"
	"math"
	"syscall/js"
	"unsafe"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// RenderDocForGraph renders d the way a trigger of id plays it in the browser:
// window.renderDocPCM runs the playback render worker on d's flattened block,
// post-processes it like ensureRenderedSample (voice FX, gate tail,
// peak-normalise × amplitude, end fade, id's Sampler edit) and resolves with
// the PCM. Graph renders wait while any
// playback render is queued, so they never delay a note. BLOCKS until the
// promise settles — call it off the UI goroutine (a pool worker).
func RenderDocForGraph(id instrument.ID, d instrument.Doc, pitch float64) ([]float32, int, error) {
	fn := js.Global().Get("renderDocPCM")
	if fn.Type() != js.TypeFunction {
		return nil, 0, errors.New("renderDocPCM: audio.js not loaded")
	}
	block := instrument.Flatten(d, BuiltinModuleRegistry())
	if d.Render.Melodic && pitch != 0 {
		block[pitchFieldIndexJS()] = pitch
	}
	raw := make([]byte, 8*len(block))
	for i, x := range block {
		binary.LittleEndian.PutUint64(raw[8*i:], math.Float64bits(x))
	}
	f64 := js.Global().Get("Float64Array").New(len(block))
	js.CopyBytesToJS(js.Global().Get("Uint8Array").New(f64.Get("buffer")), raw)

	type result struct {
		v   js.Value
		err error
	}
	ch := make(chan result, 1)
	onOK := js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) > 0 {
			ch <- result{v: args[0]}
		} else {
			ch <- result{err: errors.New("renderDocPCM: no result")}
		}
		return nil
	})
	onErr := js.FuncOf(func(this js.Value, args []js.Value) any {
		msg := "renderDocPCM failed"
		if len(args) > 0 && args[0].Truthy() {
			msg += ": " + args[0].Call("toString").String()
		}
		ch <- result{err: errors.New(msg)}
		return nil
	})
	defer onOK.Release()
	defer onErr.Release()
	fn.Invoke(string(id), f64, d.Render.Seconds, d.Render.Amplitude, d.Render.GateTail, voiceFXForJS(d)).Call("then", onOK, onErr)
	res := <-ch
	if res.err != nil {
		return nil, 0, res.err
	}
	sr := SampleRate()
	if v := res.v.Get("sr"); v.Truthy() {
		sr = v.Int()
	}
	n := res.v.Get("length").Int()
	if n <= 0 {
		return nil, sr, errors.New("renderDocPCM: empty render")
	}
	scratch := make([]byte, n*4)
	js.CopyBytesToGo(scratch, res.v.Get("bytes"))
	out := make([]float32, n)
	copy(out, unsafe.Slice((*float32)(unsafe.Pointer(&scratch[0])), n))
	return out, sr, nil
}

// GraphVoiceSec is the note's played length: render.seconds, in whole frames.
func GraphVoiceSec(d instrument.Doc) float64 {
	return float64(VoiceFrames(d.Render.Seconds, SampleRate())) / float64(SampleRate())
}

func pitchFieldIndexJS() int {
	i, _ := BuiltinModuleRegistry().FieldIndex("pitch")
	return i
}
