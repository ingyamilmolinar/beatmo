//go:build js && wasm

package audio

import "syscall/js"

// jsFloat32ToFloat64 converts a JS Float32Array to a Go []float64 slice.
func jsFloat32ToFloat64(jsArr js.Value) []float64 {
	if !jsArr.Truthy() {
		return nil
	}
	n := jsArr.Get("length").Int()
	if n == 0 {
		return nil
	}
	samples := make([]float64, n)
	for i := 0; i < n; i++ {
		samples[i] = jsArr.Index(i).Float()
	}
	return samples
}

func init() {
	platformRecordingStart = func(instruments []InstrumentMeta) {
		// Pass instrument IDs to JS so it creates per-channel capture nodes
		ids := make([]interface{}, len(instruments))
		for i, inst := range instruments {
			ids[i] = inst.ID
		}
		arr := js.Global().Get("Array").New(len(ids))
		for i, id := range ids {
			arr.SetIndex(i, js.ValueOf(id))
		}

		fn := js.Global().Get("startMultiChannelCapture")
		if fn.Truthy() {
			fn.Invoke(arr)
		}
	}

	platformRecordingStop = func() (map[string][]float64, []float64, int) {
		fn := js.Global().Get("stopMultiChannelCapture")
		if !fn.Truthy() {
			return nil, nil, 0
		}
		result := fn.Invoke()
		if !result.Truthy() {
			return nil, nil, 0
		}

		// Extract master samples
		masterArr := result.Get("master")
		master := jsFloat32ToFloat64(masterArr)

		// Extract per-instrument channels
		channelsObj := result.Get("channels")
		var perInst map[string][]float64
		if channelsObj.Truthy() {
			keys := js.Global().Get("Object").Call("keys", channelsObj)
			nKeys := keys.Get("length").Int()
			if nKeys > 0 {
				perInst = make(map[string][]float64, nKeys)
				for i := 0; i < nKeys; i++ {
					id := keys.Index(i).String()
					samples := jsFloat32ToFloat64(channelsObj.Get(id))
					if len(samples) > 0 {
						perInst[id] = samples
					}
				}
			}
		}

		return perInst, master, SampleRate()
	}
}
