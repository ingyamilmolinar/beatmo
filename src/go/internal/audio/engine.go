//go:build !test && !js

package audio

import (
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/scope"
	"github.com/ingyamilmolinar/beatmo/internal/scopeexport"
)

// sampleRate is the audio output sample rate.
// Default is 44100 Hz for maximum compatibility.
// Override with AUDIO_SAMPLE_RATE environment variable.
var sampleRate = func() int {
	if s := os.Getenv("AUDIO_SAMPLE_RATE"); s != "" {
		if rate, err := strconv.Atoi(s); err == nil && rate > 0 {
			log.Printf("[AUDIO] Using custom sample rate: %d Hz", rate)
			return rate
		}
	}
	return 44100 // Default to 44100 for maximum compatibility
}()

// bufferSizeBytes10ms is 10ms of 16-bit mono audio
var bufferSizeBytes10ms = sampleRate / 100 * 2

var (
	ctx   *oto.Context
	once  sync.Once
	mix   *mixer
	start = time.Now()
	bpm   = 120

	instruments = map[string]Instrument{}
	instOrder   []string
	instMu      sync.RWMutex

	stopHookMu sync.RWMutex
	stopHook   func(string)

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
	NewVoice(bpm, sampleRate int) Voice
}

// Register makes an instrument available for playback by ID.
func Register(id string, inst Instrument) {
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
	InstrumentChannel(id)
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
	// Initialize send effects (delay + reverb).
	initSendEffects(sampleRate)
	// Initialize insert effect chains with the correct sample rate.
	InitInsertChains(sampleRate)
	// Install master compressor for automatic gain management.
	SetupMasterCompressor(sampleRate)
	// Phase 2 audio-panel redesign: stand up the master LUFS integrator
	// before the analyzer service so MasterLUFSGetter has something to
	// read on the first tick. The integrator is fed from the mixer
	// master push site (engine_stop.go) via FeedMasterLUFS.
	EnsureMasterLUFS(float64(sampleRate))
	// Start the analyzer service for real-time metering and FFT.
	analyzerSvc = analyzer.NewService(analyzer.Config{
		FFTSize:               1024,
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
	if scopeExportFlag || os.Getenv("SCOPE_EXPORT") == "1" {
		interval := 2 * time.Second
		if v := os.Getenv("SCOPE_EXPORT_INTERVAL"); v != "" {
			if secs, err := strconv.ParseFloat(v, 64); err == nil && secs > 0 {
				interval = time.Duration(secs * float64(time.Second))
			}
		}
		path := "scope_export.jsonl"
		if v := os.Getenv("SCOPE_EXPORT_PATH"); v != "" {
			path = v
		}
		exportSvc = scopeexport.NewService(scopeexport.Config{
			SampleRate: sampleRate,
			Interval:   interval,
			OutputPath: path,
			BPMFunc:    func() int { return bpm },
			InstrumentsFunc: func() []string {
				instMu.RLock()
				defer instMu.RUnlock()
				return append([]string{}, instOrder...)
			},
			LookupMeta: func(id string) (string, string, bool) {
				m, ok := CatalogLookup(id)
				if !ok {
					return "", "", false
				}
				return m.Name, m.Source, ok
			},
			ChannelVolume: ChannelVolume,
			ChannelPan:    ChannelPan,
			MainVolume:    MainVolume,
		})
		go exportSvc.Run()
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
