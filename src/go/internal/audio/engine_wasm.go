//go:build js && wasm && !test

package audio

import (
	"sync"
	"syscall/js"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/scope"
	"github.com/ingyamilmolinar/beatmo/internal/scopeexport"
)

type Voice interface{}

type Instrument interface{}

var (
	instruments   = append([]instrument.ID(nil), roster.All...)
	instrumentsMu sync.RWMutex
)

func Register(id instrument.ID, inst Instrument) {
	instrumentsMu.Lock()
	for _, existing := range instruments {
		if existing == id {
			instrumentsMu.Unlock()
			return
		}
	}
	instruments = append(instruments, id)
	instrumentsMu.Unlock()
	bumpInstrumentsVersion()
}

// Unregister removes a runtime-registered instrument id from the playable set
// (the inverse of Register). No-op if absent.
func Unregister(id instrument.ID) {
	instrumentsMu.Lock()
	found := false
	out := instruments[:0]
	for _, existing := range instruments {
		if existing == id {
			found = true
			continue
		}
		out = append(out, existing)
	}
	instruments = out
	instrumentsMu.Unlock()
	if found {
		bumpInstrumentsVersion()
	}
}

func Play(id instrument.ID, when ...float64) {
	fn := js.Global().Get("playSound")
	if !fn.Truthy() {
		return
	}
	// Stamp the trigger clock so the UI's time-since-trigger reads (synth-tab
	// pulse glow, sampler preview playhead) work in the browser, mirroring the
	// desktop Play path. Without this, audio.SinceLastTrigger never advances on
	// WASM and those signals stay invisible.
	RecordVoiceTrigger(id)
	if len(when) > 0 {
		fn.Invoke(string(id), 1.0, when[0])
		return
	}
	fn.Invoke(string(id), 1.0)
}

// PlayVol plays an instrument at the given volume. Volume is forwarded
// to the WebAudio bridge which applies a GainNode before the destination.
func PlayVol(id instrument.ID, vol float64, when ...float64) {
	// Pass id and volume to JS. Optionally pass the first 'when' timestamp
	// if provided (seconds in AudioContext time). The JS side may ignore it.
	fn := js.Global().Get("playSound")
	if !fn.Truthy() {
		return
	}
	RecordVoiceTrigger(id)
	if len(when) > 0 {
		fn.Invoke(string(id), vol, when[0])
		return
	}
	fn.Invoke(string(id), vol)
}

// PlayParams forwards volume, pitch (semitones) and duration multiplier to the
// WebAudio bridge. The JS side applies playbackRate = 2^(pitch/12)/dur.
func PlayParams(id instrument.ID, vol, pitch, dur float64, when ...float64) {
	fn := js.Global().Get("playSoundParams")
	if !fn.Truthy() {
		// Fallback to volume-only if extended bridge is not present
		// (PlayVol stamps the trigger clock itself).
		PlayVol(id, vol, when...)
		return
	}
	RecordVoiceTrigger(id)
	if len(when) > 0 {
		fn.Invoke(string(id), vol, pitch, dur, when[0])
		return
	}
	fn.Invoke(string(id), vol, pitch, dur)
}

// PlayParamsAt schedules a sound with explicit 'when'. When when<=0 starts immediately.
func PlayParamsAt(id instrument.ID, vol, pitch, dur, when float64) {
	if when > 0 {
		PlayParams(id, vol, pitch, dur, when)
	} else {
		PlayParams(id, vol, pitch, dur)
	}
}

// SampleSeconds returns the base sample duration in seconds for an instrument ID.
// For synthesized instruments, this reads from the JS audio bridge; for WAVs,
// it reflects the decoded buffer duration. Returns 0 if unknown.
func SampleSeconds(id instrument.ID) float64 {
	fn := js.Global().Get("sampleDurationSec")
	if !fn.Truthy() {
		return 0
	}
	v := fn.Invoke(string(id))
	if v.Truthy() {
		return v.Float()
	}
	return 0
}

func Stop(id instrument.ID) {
	if fn := js.Global().Get("stopSound"); fn.Truthy() {
		fn.Invoke(string(id))
	}
}

// ResetInstruments restores the default instrument ID list.
func ResetInstruments() {
	ResetInstrumentDocs()
	instrumentsMu.Lock()
	instruments = append(append([]instrument.ID(nil), roster.All...), userOnlyLibraryIDs()...)
	instrumentsMu.Unlock()
	bumpInstrumentsVersion()
	ClearAllInsertEffects()
	resetInstrumentChannels()
	clearInstrumentDisplayNames()
	// Phase 5: wire each shipped instrument id to its SynthRecipe so the
	// browser audio pipeline can pick up user-edited params at render time
	// (mirrors the desktop + stub build paths).
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

func Instruments() []instrument.ID {
	instrumentsMu.RLock()
	ids := append([]instrument.ID(nil), instruments...)
	instrumentsMu.RUnlock()
	return ids
}

// EmitReport identifies a voice's first-emitted block. Exported so the UI
// package can drain into a []audio.EmitReport. WASM has no Go-owned mixer
// loop (playback happens in WebAudio/JS), so this type exists only to keep
// the signature uniform across build tags; browser Stage-D emission comes
// from JS in Phase 4, not this ring.
type EmitReport struct {
	Row, Abs int
	DevPos   int64
}

// DeviceClockSnapshot is unavailable on WASM: there is no Go-owned mixer
// device clock (WebAudio owns timing). Always reports unavailable.
func DeviceClockSnapshot() (int64, float64, bool) { return 0, 0, false }

// DrainEmitReports is unavailable on WASM: no Go-owned emission ring. Always empty.
func DrainEmitReports(dst []EmitReport) int { return 0 }

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
		SampleRate: sr,
		Interval:   2 * time.Second,
		InstrumentsFunc: func() []channel.ID {
			ids := Instruments()
			out := make([]channel.ID, len(ids))
			for i, id := range ids {
				out[i] = channel.Of(id)
			}
			return out
		},
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
//
// All five pipeline stages are bridged via the matching JS analyser nodes:
//   - StageSynth / StageAntiPop  → channel-ingress (pre-FX) analyser
//   - StageInsertFX              → pre-EQ analyser (post-inserts, pre-EQ)
//   - StageEQ                    → channel analyser (post-EQ)
//   - StageMaster                → main-channel analyser
func pollExportSamples(svc *scopeexport.Service) {
	for _, id := range Instruments() {
		synth := SynthAnalyzerSnapshot(channel.Of(id))
		if len(synth.Waveform) > 0 {
			svc.PushSamples(scope.StageSynth, channel.Of(id), synth.Waveform)
			// AntiPop shares the synth ingress in WASM — see ScopeStageSnapshots.
			svc.PushSamples(scope.StageAntiPop, channel.Of(id), synth.Waveform)
		}
		pre := PreEQAnalyzerSnapshot(channel.Of(id))
		if len(pre.Waveform) > 0 {
			svc.PushSamples(scope.StageInsertFX, channel.Of(id), pre.Waveform)
		}
		post := ChannelAnalyzerSnapshot(channel.Of(id))
		if len(post.Waveform) > 0 {
			svc.PushSamples(scope.StageEQ, channel.Of(id), post.Waveform)
		}
	}
	master := ChannelAnalyzerSnapshot(channel.Master)
	if len(master.Waveform) > 0 {
		svc.PushSamples(scope.StageMaster, channel.Master, master.Waveform)
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
