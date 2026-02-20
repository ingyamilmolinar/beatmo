//go:build js && wasm

package audio

import "syscall/js"

func SetChannelEQ(id string, sampleRate int, bands ...EQBand) {
	fn := js.Global().Get("setChannelEQ")
	if !fn.Truthy() {
		return
	}
	lastSetEQ = eqRecord{ID: id, SampleRate: sampleRate, Bands: append([]EQBand(nil), bands...)}
	arr := js.Global().Get("Array").New()
	for _, b := range bands {
		obj := js.Global().Get("Object").New()
		obj.Set("freq", b.Freq)
		obj.Set("q", b.Q)
		obj.Set("gainDB", b.GainDB)
		obj.Set("muted", b.Muted)
		switch b.Kind {
		case EQLowShelf:
			obj.Set("type", "lowshelf")
		case EQHighShelf:
			obj.Set("type", "highshelf")
		case EQLowpass:
			obj.Set("type", "lowpass")
		case EQHighpass:
			obj.Set("type", "highpass")
		default:
			obj.Set("type", "peaking")
		}
		arr.Call("push", obj)
	}
	fn.Invoke(id, arr)
}

func ClearChannelProcessors(id string) {
	SetChannelEQ(id, 0)
}

// NewEQProcessor is a no-op on WASM since EQ is handled by WebAudio.
func NewEQProcessor(sampleRate int, bands ...EQBand) Processor {
	return &passThrough{}
}
