//go:build js && !test

package ui

import (
	"syscall/js"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

func (g *Game) initJSRecording() {
	// lastResult caches the most recent StopRecording result so that
	// saveRecording() can use it even after stopRecording() was already called.
	var lastResult *audio.RecordingResult

	// isRecording() -> bool
	js.Global().Set("isRecording", jsFn(func(args jsArgs) any {
		return js.ValueOf(audio.IsRecording())
	}))

	// startRecording(format?) -> Promise<{error: string}>
	// format is optional; defaults to "wav24".
	//
	// Returns a Promise rather than a synchronous result because the
	// off-thread pipeline blocks on awaitJSPromise to confirm the
	// AudioWorklet + Web Worker are wired before returning. awaitJSPromise
	// MUST run on a goroutine (not inside the JS callback) — otherwise the
	// JS event loop is held by this callback and can never dispatch the
	// Promise resolution, deadlocking the page.
	js.Global().Set("startRecording", jsFn(func(args jsArgs) any {
		format := audio.FormatWAV24
		if args.Len() > 0 && args.At(0).Type() == js.TypeString {
			format = audio.AudioFormat(args.Str(0))
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

		promiseCtor := js.Global().Get("Promise")
		executor := js.FuncOf(func(this js.Value, pargs []js.Value) interface{} {
			resolve := pargs[0]
			go func() {
				if err := audio.StartRecording(opts); err != nil {
					resolve.Invoke(js.ValueOf(map[string]interface{}{
						"error": err.Error(),
					}))
					return
				}
				lastResult = nil // clear stale cached result
				g.drum.SetRecording(true)
				resolve.Invoke(js.ValueOf(map[string]interface{}{
					"error": "",
				}))
			}()
			return nil
		})
		return promiseCtor.New(executor)
	}))

	// stopRecording() -> Promise<{error: string, channelCount: int, format: string,
	//   duration: float, channels: [{id, name, filename, size}], autoStopped: bool,
	//   filename: string}>
	//
	// On WASM the recording finalize runs off-thread (audio worklet → web
	// worker), so this returns a Promise that resolves once the worker has
	// emitted the final Blob and the EventRecordStop hook has fired. The
	// returned `filename` is the zip name ready in Downloads. JS callers
	// (browser tests, custom UIs) should `await` this.
	js.Global().Set("stopRecording", jsFn(func(args jsArgs) any {
		promiseCtor := js.Global().Get("Promise")
		executor := js.FuncOf(func(this js.Value, pargs []js.Value) interface{} {
			resolve := pargs[0]
			// Kick the in-process stop, then wait on EventRecordStop to
			// surface the finalized payload (filename, drops, etc).
			doneCh := make(chan audio.RecordStopPayload, 1)
			unsub := hooks.Subscribe(hooks.EventRecordStop, func(e hooks.Event) {
				p, ok := e.Payload.(audio.RecordStopPayload)
				if !ok {
					return
				}
				select {
				case doneCh <- p:
				default:
				}
			})

			placeholder, err := audio.StopRecording()
			g.drum.SetRecording(false)
			if err != nil {
				unsub()
				resolve.Invoke(js.ValueOf(map[string]interface{}{
					"error": err.Error(),
				}))
				return nil
			}
			lastResult = placeholder

			go func() {
				defer unsub()
				timeout := time.NewTimer(30 * time.Second)
				defer timeout.Stop()
				var payload audio.RecordStopPayload
				select {
				case payload = <-doneCh:
				case <-timeout.C:
					resolve.Invoke(js.ValueOf(map[string]interface{}{
						"error": "stopRecording timeout",
					}))
					return
				}
				if payload.Err != nil {
					resolve.Invoke(js.ValueOf(map[string]interface{}{
						"error": payload.Err.Error(),
					}))
					return
				}
				// lastResult was populated by the goroutine inside audio.StopRecording.
				result := lastResult
				channels := js.Global().Get("Array").New(0)
				if result != nil {
					channels = js.Global().Get("Array").New(len(result.Channels))
					for i, ch := range result.Channels {
						// Prefer ch.Bytes (set by streaming/worker pipelines)
						// over len(ch.Data) (legacy in-memory). Either path
						// reports the encoded payload size correctly.
						size := len(ch.Data)
						if size == 0 && ch.Bytes > 0 {
							size = ch.Bytes
						}
						channels.SetIndex(i, js.ValueOf(map[string]interface{}{
							"id":       ch.ID,
							"name":     ch.Name,
							"filename": ch.Filename,
							"size":     size,
						}))
					}
				}
				out := map[string]interface{}{
					"error":        "",
					"channelCount": payload.Channels,
					"duration":     payload.Duration,
					"drops":        int(payload.Drops),
					"filename":     payload.Dir,
					"channels":     channels,
				}
				if result != nil {
					out["format"] = string(result.Metadata.Format)
					out["bpm"] = result.Metadata.BPM
					out["sampleRate"] = result.Metadata.SampleRate
					out["autoStopped"] = audio.RecordingAutoStopped()
				}
				resolve.Invoke(js.ValueOf(out))
			}()
			return nil
		})
		return promiseCtor.New(executor)
	}))

	// saveRecording() -> Promise<{error: string, path: string}>
	//
	// Triggers the browser zip download. On WASM the underlying
	// audio.StopRecording auto-saves on completion (mirroring desktop's
	// async finalize), so this is normally redundant — it's kept to
	// support the "stop and save in one call" UX. Returns a Promise that
	// resolves to the saved filename (the zip in Downloads).
	//
	// Behavior:
	//   - If recording is active: stops it (which auto-saves), waits for
	//     EventRecordStop, returns the resulting path.
	//   - If not recording but a previous session is still pending the
	//     download anchor click: triggers it and returns.
	//   - Otherwise: returns {error: "no recording available to save"}.
	js.Global().Set("saveRecording", jsFn(func(args jsArgs) any {
		promiseCtor := js.Global().Get("Promise")
		executor := js.FuncOf(func(this js.Value, pargs []js.Value) interface{} {
			resolve := pargs[0]

			if audio.IsRecording() {
				doneCh := make(chan audio.RecordStopPayload, 1)
				unsub := hooks.Subscribe(hooks.EventRecordStop, func(e hooks.Event) {
					p, ok := e.Payload.(audio.RecordStopPayload)
					if !ok {
						return
					}
					select {
					case doneCh <- p:
					default:
					}
				})
				_, err := audio.StopRecording()
				g.drum.SetRecording(false)
				if err != nil {
					unsub()
					resolve.Invoke(js.ValueOf(map[string]interface{}{
						"error": err.Error(), "path": "",
					}))
					return nil
				}
				go func() {
					defer unsub()
					timeout := time.NewTimer(30 * time.Second)
					defer timeout.Stop()
					var payload audio.RecordStopPayload
					select {
					case payload = <-doneCh:
					case <-timeout.C:
						resolve.Invoke(js.ValueOf(map[string]interface{}{
							"error": "saveRecording timeout", "path": "",
						}))
						return
					}
					if payload.Err != nil {
						resolve.Invoke(js.ValueOf(map[string]interface{}{
							"error": payload.Err.Error(), "path": "",
						}))
						return
					}
					lastResult = nil
					resolve.Invoke(js.ValueOf(map[string]interface{}{
						"error": "", "path": payload.Dir,
					}))
				}()
				return nil
			}

			// Not recording: the last session may have already auto-saved
			// during its own stop. If lastResult exists, the save was
			// already triggered. If not, surface a friendly error.
			if lastResult != nil {
				path := lastResult.SessionDir
				lastResult = nil
				resolve.Invoke(js.ValueOf(map[string]interface{}{
					"error": "", "path": path,
				}))
				return nil
			}
			resolve.Invoke(js.ValueOf(map[string]interface{}{
				"error": "no recording available to save", "path": "",
			}))
			return nil
		})
		return promiseCtor.New(executor)
	}))

	// recordingElapsedMs() -> float (milliseconds since recording start)
	js.Global().Set("recordingElapsedMs", jsFn(func(args jsArgs) any {
		return js.ValueOf(audio.RecordingElapsed().Seconds() * 1000)
	}))

	// availableFormats() -> ["wav16", "wav24", "wav32f", "flac", "ogg"]
	js.Global().Set("availableFormats", jsFn(func(args jsArgs) any {
		formats := audio.AvailableFormats()
		arr := js.Global().Get("Array").New(len(formats))
		for i, f := range formats {
			arr.SetIndex(i, js.ValueOf(string(f)))
		}
		return arr
	}))
}
