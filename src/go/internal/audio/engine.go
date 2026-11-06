//go:build !test && !js

package audio

import (
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/env"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/log"
	"github.com/ingyamilmolinar/beatmo/internal/scope"
	"github.com/ingyamilmolinar/beatmo/internal/scopeexport"
)

// sampleRate is the audio output sample rate.
// Default is EngineSampleRate (48 kHz), the rate the browser runs at too, so
// a circuit is the same samples on both platforms. BEATMO_AUDIO_SAMPLE_RATE
// (env.AudioSampleRate) overrides it for diagnostics only.
var sampleRate = func() int {
	rate := env.AudioSampleRate.Get()
	if env.AudioSampleRate.IsSet() {
		log.Infof("[audio] using custom sample rate: %d Hz", rate)
	}
	// Effects built before the device opens (an offline render) run at the
	// engine rate too.
	InitInsertChains(rate)
	return rate
}()

// bufferSizeBytes10ms is 10ms of 16-bit mono audio
var bufferSizeBytes10ms = sampleRate / 100 * 2

var (
	ctx   *oto.Context
	once  sync.Once
	mix   *mixer
	start = time.Now()
	bpm   = 120

	instruments = map[instrument.ID]Instrument{}
	instOrder   []instrument.ID
	instMu      sync.RWMutex

	stopHookMu sync.RWMutex
	stopHook   func(instrument.ID)

	analyzerSvc *analyzer.Service
	scopeSvc    *scope.Service

	exportSvc       *scopeexport.Service
	scopeExportFlag bool
)

// Voice generates PCM samples in the range [-1,1].
type Voice interface {
	// Sample returns the next sample and whether the voice has finished.
	Sample() (float64, bool)
}

// BlockVoice is an optional interface for voices that support bulk sample
// rendering. When a voice implements BlockVoice, the mixer uses SampleBlock
// instead of calling Sample() in a tight loop, reducing per-sample function
// call overhead.
type BlockVoice interface {
	Voice
	// SampleBlock fills dst with up to len(dst) samples and returns the
	// number of samples written and whether the voice is done.
	SampleBlock(dst []float64) (int, bool)
}

// Instrument constructs a new Voice instance when triggered.
type Instrument interface {
	NewVoice(sampleRate int) Voice
}

// Register makes an instrument available for playback by ID.
func Register(id instrument.ID, inst Instrument) {
	created := false
	instMu.Lock()
	if _, exists := instruments[id]; !exists {
		instOrder = append(instOrder, id)
		created = true
	}
	instruments[id] = inst
	instMu.Unlock()
	if created {
		bumpInstrumentsVersion()
	}
}

// Unregister removes a runtime-registered instrument from the playable set. It
// is the inverse of Register, used by DeleteInstrument to drop a cloned/user
// instrument. Removing an id that isn't registered is a no-op.
func Unregister(id instrument.ID) {
	instMu.Lock()
	_, existed := instruments[id]
	if existed {
		delete(instruments, id)
		out := instOrder[:0]
		for _, x := range instOrder {
			if x != id {
				out = append(out, x)
			}
		}
		instOrder = out
	}
	instMu.Unlock()
	if existed {
		bumpInstrumentsVersion()
	}
}

// AnalyzerService returns the global analyzer service, or nil if audio
// has not been initialized (e.g. in test/WASM builds).
func AnalyzerService() *analyzer.Service { return analyzerSvc }

// ScopeService returns the global scope service, or nil if audio
// has not been initialized (e.g. in test/WASM builds).
func ScopeService() *scope.Service { return scopeSvc }

// ExportService returns the global scope export service, or nil if
// export is not enabled.
func ExportService() *scopeexport.Service { return exportSvc }

// EnableScopeExport enables scope export on the next audio init.
func EnableScopeExport() { scopeExportFlag = true }

func init() {
	ResetInstruments()
}

func initContext() {
	c := platformInitContext(sampleRate)
	if c == nil {
		return
	}
	ctx = c
	mix = newMixer(c)
	// Initialize insert effect chains with the correct sample rate.
	InitInsertChains(sampleRate)
	// Phase 2 audio-panel redesign: stand up the master LUFS integrator
	// before the analyzer service so MasterLUFSGetter has something to
	// read on the first tick. The integrator is fed from the mixer
	// master push site (engine_stop.go) via FeedMasterLUFS.
	EnsureMasterLUFS(float64(sampleRate))
	// Start the analyzer service for real-time metering and FFT.
	analyzerSvc = analyzer.NewService(analyzer.Config{
		FFTSize:               analyzer.SpectrumFFTSize,
		WindowSize:            2048,
		MaxInstruments:        32,
		SampleRate:            sampleRate,
		MasterLUFSGetter:      MasterLUFSShortTerm,
		ClipsLastWindowGetter: ClipsLastWindow,
	})
	go analyzerSvc.Run()
	// Start the scope service for real-time A/B pipeline comparison.
	scopeSvc = scope.NewService(scope.Config{
		MaxWindowMs: 500,
		SampleRate:  sampleRate,
	})
	go scopeSvc.Run()

	// Optionally start the scope export flight recorder.
	if scopeExportFlag || env.ScopeExport.Get() {
		interval := 2 * time.Second
		if secs := env.ScopeExportInterval.Get(); secs > 0 {
			interval = time.Duration(secs * float64(time.Second))
		}
		path := env.ScopeExportPath.Get()
		exportSvc = scopeexport.NewService(scopeexport.Config{
			SampleRate: sampleRate,
			Interval:   interval,
			OutputPath: path,
			BPMFunc:    func() int { return bpm },
			InstrumentsFunc: func() []channel.ID {
				instMu.RLock()
				defer instMu.RUnlock()
				out := make([]channel.ID, len(instOrder))
				for i, id := range instOrder {
					out[i] = channel.Of(id)
				}
				return out
			},
			LookupMeta: func(ch channel.ID) (string, string, bool) {
				id, ok := ch.Instrument()
				if !ok {
					return "", "", false
				}
				m, ok := CatalogLookup(id)
				if !ok {
					return "", "", false
				}
				return m.Name, m.Source.String(), ok
			},
			ChannelVolume: ChannelVolume,
			MainVolume:    MainVolume,
		})
		exportSvc.Start()
	}
}

// Close stops the audio player, clears all voices, and releases the audio
// device. Safe to call when mix is nil (e.g. audio was never initialized).
func Close() {
	if mix == nil {
		return
	}
	mix.mu.Lock()
	mix.voices = nil
	mix.mu.Unlock()
	if mix.player != nil {
		mix.player.Pause()
		mix.player.Close()
	}
}
