//go:build js && !test

package ui

import (
	"syscall/js"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

func (g *Game) initJSRecording() {
	// So saveRecording() works after stopRecording().
	var lastResult *audio.RecordingResult

	// isRecording() -> bool
	js.Global().Set("isRecording", jsFn(func(args jsArgs) any {
		return js.ValueOf(audio.IsRecording())
	}))

	// startRecording(format?="wav24") -> Promise<{error}>. awaitJSPromise MUST run on a goroutine: inside the
	// callback the JS event loop could never resolve the promise and the page deadlocks.
	js.Global().Set("startRecording", jsFn(func(args jsArgs) any {
		format := audio.FormatWAV24
		if args.Len() > 0 && args.At(0).Type() == js.TypeString {
			format = audio.AudioFormat(args.Str(0))
		}

		instruments := make([]audio.InstrumentMeta, 0, len(g.drum.Rows))
		for _, row := range g.drum.Rows {
			if row.Instrument != "" {
				instruments = append(instruments, audio.InstrumentMeta{
					ID:   channel.Of(row.Instrument),
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

	// stopRecording() -> Promise<{error,channelCount,format,duration,channels,autoStopped,filename}>; resolves once
	// the off-thread worker emits the Blob and EventRecordStop fires.
	js.Global().Set("stopRecording", jsFn(func(args jsArgs) any {
		promiseCtor := js.Global().Get("Promise")
		executor := js.FuncOf(func(this js.Value, pargs []js.Value) interface{} {
			resolve := pargs[0]
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
						// Bytes is set by the streaming pipelines; Data by the in-memory one.
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

	// saveRecording() -> Promise<{error,path}>. A stop already auto-saves; this keeps the one-call "stop and save":
	// it stops an active recording, re-triggers a pending download, or reports nothing to save.
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

			// The last stop already triggered the save.
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
