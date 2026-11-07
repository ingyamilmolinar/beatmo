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

// No-op if absent.
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
	// Stamped as on desktop, or time-since-trigger UI never advances in the browser.
	RecordVoiceTrigger(id)
	if len(when) > 0 {
		fn.Invoke(string(id), 1.0, when[0])
		return
	}
	fn.Invoke(string(id), 1.0)
}

func PlayVol(id instrument.ID, vol float64, when ...float64) {
	// when, if given, is in AudioContext seconds; JS may ignore it.
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

// Forwarded to the WebAudio bridge, which builds the same played voice as desktop.
func PlayParams(id instrument.ID, vol, pitch, dur float64, when ...float64) {
	fn := js.Global().Get("playSoundParams")
	if !fn.Truthy() {
		// Volume only when the extended bridge is absent; PlayVol stamps the trigger.
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

// Synths read from the JS bridge, WAVs from the decoded buffer; 0 if unknown.
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
}

// 0 when the bridge is unavailable.
func Now() float64 {
	// audio.js may not be loaded yet; 0 tells callers to schedule immediately.
	fn := js.Global().Get("audioNow")
	if !fn.Truthy() {
		return 0
	}
	v := fn.Invoke()
	// Type(), not Truthy(): 0.0 is a valid currentTime for a new or suspended context.
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

// 48000 as fallback.
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

// Only for a uniform signature across build tags: WASM has no Go-owned mixer, and browser emission timing comes from JS.
type EmitReport struct {
	Row, Abs int
	DevPos   int64
}

// WebAudio owns timing; always unavailable.
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

// nil until EnableScopeExport, as on desktop.
func ExportService() *scopeexport.Service {
	wasmExportMu.Lock()
	defer wasmExportMu.Unlock()
	return wasmExportSvc
}

// Polls the JS analysers into the ring buffers and snapshots them so downloadScopeExport() can flush JSONL. Idempotent.
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

	// One goroutine owns both timers: ~10 polls/s and a snapshot every 2 s, desktop's cadence.
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

// Every stage maps to a JS analyser: Synth / AntiPop → channel ingress, InsertFX → pre-EQ, EQ → channel (post-EQ),
// Master → main channel.
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

// For the browser test harness, without waiting for the poll cycle; false when the service is not running.
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
