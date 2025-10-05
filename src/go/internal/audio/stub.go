//go:build test

package audio

type Voice interface{}

type Instrument interface{ NewVoice(int, int) Voice }

var insts = []string{"snare", "kick", "hihat", "tom", "clap"}

func Register(id string, inst Instrument) {
	insts = append(insts, id)
	InstrumentChannel(id)
}

func RegisterWAV(id, path string) error {
	insts = append(insts, id)
	InstrumentChannel(id)
	return nil
}

func SelectWAV() (string, error) { return "dummy.wav", nil }

// Play is a stub used during tests to avoid initializing audio devices.
func Play(id string, when ...float64) {}

// PlayVol is a stub used during tests for volume-controlled playback.
func PlayVol(id string, vol float64, when ...float64) {}

// PlayParams is a stub used during tests to accept extended playback params.
func PlayParams(id string, vol, pitch, dur float64, when ...float64) {}

// PlayParamsAt is a stub used during tests to avoid varargs allocation.
func PlayParamsAt(id string, vol, pitch, dur, when float64) {}

// SampleSeconds is a stub for tests.
func SampleSeconds(id string) float64 { return 0 }

var stopHook func(string)

func Stop(id string) {
	if stopHook != nil {
		stopHook(id)
	}
}

func SetStopHook(fn func(string)) { stopHook = fn }

// Now returns 0 during tests.
func Now() float64 { return 0 }

// Resume is a no-op in tests.
func Resume() {}

// Reset is a stub used during tests.
func Reset() { resetChannels() }

var SetBPMFunc = func(int) {}

func SetBPM(bpm int) { SetBPMFunc(bpm) }

// Instruments returns placeholder instrument IDs during tests.
func Instruments() []string { return insts }

func ResetInstruments() {
	insts = []string{"snare", "kick", "hihat", "tom", "clap"}
	resetInstrumentChannels(insts)
}

func RenameInstrument(oldID, newID string) {
	for i, id := range insts {
		if id == oldID {
			insts[i] = newID
			break
		}
	}
	renameInstrumentChannel(oldID, newID)
}
