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

// 48 kHz, the browser's rate too, so a circuit is the same samples on both; the env override is for diagnostics only.
var sampleRate = func() int {
	rate := env.AudioSampleRate.Get()
	if env.AudioSampleRate.IsSet() {
		log.Infof("[audio] using custom sample rate: %d Hz", rate)
	}
	// So effects built before the device opens run at the engine rate too.
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

// The mixer then uses SampleBlock instead of calling Sample() in a loop.
type BlockVoice interface {
	Voice
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

// Removing an unregistered id is a no-op.
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

// nil before audio init (test / WASM builds).
func AnalyzerService() *analyzer.Service { return analyzerSvc }

// nil before audio init (test / WASM builds).
func ScopeService() *scope.Service { return scopeSvc }

// nil when export is disabled.
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
	// Before the analyzer service, so MasterLUFSGetter has something to read on the first tick.
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

// Safe when audio was never initialised.
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
