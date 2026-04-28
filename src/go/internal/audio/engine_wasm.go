//go:build js && wasm && !test

package audio

import (
	"sync"
	"syscall/js"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/scope"
	"github.com/ingyamilmolinar/beatmo/internal/scopeexport"
)

type Voice interface{}

type Instrument interface{}

var (
	instruments   = append([]string(nil), BuiltinInstrumentIDs...)
	instrumentsMu sync.RWMutex
)

func Register(id string, inst Instrument) {
	instrumentsMu.Lock()
	for _, existing := range instruments {
		if existing == id {
			instrumentsMu.Unlock()
			InstrumentChannel(id)
			return
		}
	}
	instruments = append(instruments, id)
	instrumentsMu.Unlock()
	bumpInstrumentsVersion()
	InstrumentChannel(id)
}

func Play(id string, when ...float64) {
	fn := js.Global().Get("playSound")
	if !fn.Truthy() {
		return
	}
	if len(when) > 0 {
		fn.Invoke(id, 1.0, when[0])
		return
	}
	fn.Invoke(id, 1.0)
}

// PlayVol plays an instrument at the given volume. Volume is forwarded
// to the WebAudio bridge which applies a GainNode before the destination.
func PlayVol(id string, vol float64, when ...float64) {
	// Pass id and volume to JS. Optionally pass the first 'when' timestamp
	// if provided (seconds in AudioContext time). The JS side may ignore it.
	fn := js.Global().Get("playSound")
	if !fn.Truthy() {
		return
	}
	if len(when) > 0 {
		fn.Invoke(id, vol, when[0])
		return
	}
	fn.Invoke(id, vol)
}

// PlayParams forwards volume, pitch (semitones) and duration multiplier to the
// WebAudio bridge. The JS side applies playbackRate = 2^(pitch/12)/dur.
func PlayParams(id string, vol, pitch, dur float64, when ...float64) {
	fn := js.Global().Get("playSoundParams")
	if !fn.Truthy() {
		// Fallback to volume-only if extended bridge is not present.
		PlayVol(id, vol, when...)
		return
	}
	if len(when) > 0 {
		fn.Invoke(id, vol, pitch, dur, when[0])
		return
	}
	fn.Invoke(id, vol, pitch, dur)
}

// PlayParamsAt schedules a sound with explicit 'when'. When when<=0 starts immediately.
func PlayParamsAt(id string, vol, pitch, dur, when float64) {
	if when > 0 {
		PlayParams(id, vol, pitch, dur, when)
	} else {
		PlayParams(id, vol, pitch, dur)
	}
}

// SampleSeconds returns the base sample duration in seconds for an instrument ID.
// For synthesized instruments, this reads from the JS audio bridge; for WAVs,
// it reflects the decoded buffer duration. Returns 0 if unknown.
func SampleSeconds(id string) float64 {
	fn := js.Global().Get("sampleDurationSec")
	if !fn.Truthy() {
		return 0
	}
	v := fn.Invoke(id)
	if v.Truthy() {
		return v.Float()
	}
	return 0
}

func Stop(id string) {
	if fn := js.Global().Get("stopSound"); fn.Truthy() {
		fn.Invoke(id)
	}
}

// ResetInstruments restores the default instrument ID list.
func ResetInstruments() {
	instrumentsMu.Lock()
	instruments = append([]string(nil), BuiltinInstrumentIDs...)
	instrumentsMu.Unlock()
	bumpInstrumentsVersion()
	ClearAllInsertEffects()
	resetInstrumentChannels(instruments)
}

// Now returns the current WebAudio time in seconds as reported by
// AudioContext.currentTime via the JS bridge. Falls back to 0 when unavailable.
func Now() float64 {
	// Guard against missing bridge function to avoid panics when the
	// page hasn't loaded audio.js yet. Returning 0 signals callers to
	// schedule immediately using the current AudioContext time.
	fn := js.Global().Get("audioNow")
	if !fn.Truthy() {
		return 0
	}
	v := fn.Invoke()
	// Use Type() check instead of Truthy() because Truthy() returns false
	// for 0.0, which is a valid AudioContext.currentTime when the context
	// is newly created or suspended.
	if v.Type() == js.TypeNumber {
		return v.Float()
	}
	return 0
}

// Close is a no-op on WASM (browser handles its own audio cleanup via pagehide).
func Close() {}

func Reset() { resetChannels() }

func Resume() {
	// Ask JS to resume the AudioContext, typically after a user gesture.
	if fn := js.Global().Get("resumeAudio"); fn.Truthy() {
		fn.Invoke()
	}
}

func SetBPM(b int) {}

// SampleRate returns the WebAudio context sample rate.
// Returns 48000 as fallback (common browser default).
func SampleRate() int {
	sr := js.Global().Get("__audioCtxSR")
	if sr.Truthy() {
		return int(sr.Float())
	}
	return 48000
}

func Instruments() []string {
	instrumentsMu.RLock()
	ids := append([]string(nil), instruments...)
	instrumentsMu.RUnlock()
	return ids
}

// RenameInstrument updates an instrument ID in the list.
func RenameInstrument(oldID, newID string) {
	changed := false
	instrumentsMu.Lock()
	for i, id := range instruments {
		if id == oldID {
			instruments[i] = newID
			changed = true
			break
		}
	}
	instrumentsMu.Unlock()
	if changed {
		bumpInstrumentsVersion()
	}
	renameInstrumentChannel(oldID, newID)
}

// AnalyzerService returns nil on WASM (analyzer runs via JS AudioWorklet, not Go).
func AnalyzerService() *analyzer.Service { return nil }

// ScopeService returns nil on WASM (scope runs via JS AudioWorklet, not Go).
func ScopeService() *scope.Service { return nil }

var (
	wasmExportSvc  *scopeexport.Service
	wasmExportMu   sync.Mutex
	wasmExportStop chan struct{}
)

// ExportService returns the WASM flight-recorder service once it has been
// started via EnableScopeExport; otherwise nil (matching the desktop contract).
func ExportService() *scopeexport.Service {
	wasmExportMu.Lock()
	defer wasmExportMu.Unlock()
	return wasmExportSvc
}

// EnableScopeExport starts the WASM flight recorder. It periodically polls the
// JS AudioWorklet analyzers (pre-EQ and post-EQ snapshots), pushes their rolling
// waveforms into scopeexport.Service ring buffers, and invokes BufferSnapshot
// so downloadScopeExport() can flush the accumulated JSONL. Idempotent.
func EnableScopeExport() {
	wasmExportMu.Lock()
	if wasmExportSvc != nil {
		wasmExportMu.Unlock()
		return
	}
	sr := SampleRate()
	wasmExportSvc = scopeexport.NewService(scopeexport.Config{
		SampleRate:      sr,
		Interval:        2 * time.Second,
		InstrumentsFunc: func() []string { return Instruments() },
	})
	wasmExportStop = make(chan struct{})
	stop := wasmExportStop
	svc := wasmExportSvc
	wasmExportMu.Unlock()

	// Poll the analyzers ~10x per second to keep the ring buffers fed with
	// rolling-window samples, and take a full snapshot every 2s to mirror the
	// desktop cadence. A single goroutine owns both timers.
	go func() {
		pollTicker := time.NewTicker(100 * time.Millisecond)
		defer pollTicker.Stop()
		snapTicker := time.NewTicker(2 * time.Second)
		defer snapTicker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-pollTicker.C:
				pollExportSamples(svc)
			case <-snapTicker.C:
				svc.BufferSnapshot()
			}
		}
	}()
}

// pollExportSamples pushes the current JS analyzer snapshots for every
// instrument into the export service ring buffers. Shared by the background
// goroutine ticker and ForceScopeExportSnapshot.
func pollExportSamples(svc *scopeexport.Service) {
	for _, id := range Instruments() {
		pre := PreEQAnalyzerSnapshot(id)
		if len(pre.Waveform) > 0 {
			svc.PushSamples(scope.StageSynth, id, pre.Waveform)
		}
		post := ChannelAnalyzerSnapshot(id)
		if len(post.Waveform) > 0 {
			svc.PushSamples(scope.StageEQ, id, post.Waveform)
		}
	}
	master := ChannelAnalyzerSnapshot("main")
	if len(master.Waveform) > 0 {
		svc.PushSamples(scope.StageMaster, "master", master.Waveform)
	}
}

// ForceScopeExportSnapshot immediately polls all instrument analyzers, pushes
// the samples into the ring buffers, and buffers one snapshot. Used by the
// browser test harness to produce a snapshot without waiting for the
// background goroutine's 100ms poll cycle. Returns false if the export
// service is not running.
func ForceScopeExportSnapshot() bool {
	wasmExportMu.Lock()
	svc := wasmExportSvc
	wasmExportMu.Unlock()
	if svc == nil {
		return false
	}
	pollExportSamples(svc)
	svc.BufferSnapshot()
	return true
}
