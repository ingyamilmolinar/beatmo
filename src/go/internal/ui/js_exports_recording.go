//go:build js && !test

package ui

import (
	"syscall/js"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func (g *Game) initJSRecording() {
	// lastResult caches the most recent StopRecording result so that
	// saveRecording() can use it even after stopRecording() was already called.
	var lastResult *audio.RecordingResult

	// isRecording() -> bool
	js.Global().Set("isRecording", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		return js.ValueOf(audio.IsRecording())
	}))

	// startRecording(format?) -> {error: string}
	// format is optional; defaults to "wav24"
	js.Global().Set("startRecording", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		format := audio.FormatWAV24
		if len(args) > 0 && args[0].Type() == js.TypeString {
			format = audio.AudioFormat(args[0].String())
		}

		instruments := make([]audio.InstrumentMeta, 0, len(g.drum.Rows))
		for _, row := range g.drum.Rows {
			if row.Instrument != "" {
				instruments = append(instruments, audio.InstrumentMeta{
					ID:   row.Instrument,
					Name: row.Name,
				})
			}
		}

		opts := audio.RecordingOptions{
			Format:      format,
			Instruments: instruments,
			BPM:         g.drum.BPM(),
		}

		if err := audio.StartRecording(opts); err != nil {
			return js.ValueOf(map[string]interface{}{"error": err.Error()})
		}
		lastResult = nil // clear stale cached result
		g.drum.SetRecording(true)
		return js.ValueOf(map[string]interface{}{"error": ""})
	}))

	// stopRecording() -> {error: string, channelCount: int, format: string, duration: float, channels: [{id, name, filename, size}]}
	js.Global().Set("stopRecording", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		result, err := audio.StopRecording()
		g.drum.SetRecording(false)
		if err != nil {
			return js.ValueOf(map[string]interface{}{"error": err.Error()})
		}

		lastResult = result

		channels := js.Global().Get("Array").New(len(result.Channels))
		for i, ch := range result.Channels {
			channels.SetIndex(i, js.ValueOf(map[string]interface{}{
				"id":       ch.ID,
				"name":     ch.Name,
				"filename": ch.Filename,
				"size":     len(ch.Data),
			}))
		}

		return js.ValueOf(map[string]interface{}{
			"error":        "",
			"channelCount": len(result.Channels),
			"format":       string(result.Metadata.Format),
			"duration":     result.Metadata.Duration,
			"bpm":          result.Metadata.BPM,
			"sampleRate":   result.Metadata.SampleRate,
			"channels":     channels,
		})
	}))

	// saveRecording() -> {error: string, path: string}
	// Triggers platform-specific save (filesystem on desktop, zip download on WASM).
	// Can be called while recording (stops first) or after stopRecording().
	js.Global().Set("saveRecording", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		var result *audio.RecordingResult

		if audio.IsRecording() {
			r, err := audio.StopRecording()
			g.drum.SetRecording(false)
			if err != nil {
				return js.ValueOf(map[string]interface{}{"error": err.Error(), "path": ""})
			}
			result = r
		} else if lastResult != nil {
			result = lastResult
		} else {
			return js.ValueOf(map[string]interface{}{"error": "no recording available to save", "path": ""})
		}

		savePath, saveErr := audio.SaveRecording(result)
		lastResult = nil // clear after save
		if saveErr != nil {
			return js.ValueOf(map[string]interface{}{"error": saveErr.Error(), "path": ""})
		}
		return js.ValueOf(map[string]interface{}{"error": "", "path": savePath})
	}))

	// recordingElapsedMs() -> float (milliseconds since recording start)
	js.Global().Set("recordingElapsedMs", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		return js.ValueOf(audio.RecordingElapsed().Seconds() * 1000)
	}))

	// availableFormats() -> ["wav16", "wav24", "wav32f", "flac", "ogg"]
	js.Global().Set("availableFormats", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		formats := audio.AvailableFormats()
		arr := js.Global().Get("Array").New(len(formats))
		for i, f := range formats {
			arr.SetIndex(i, js.ValueOf(string(f)))
		}
		return arr
	}))
}
