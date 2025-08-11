//go:build test

package audio

type Voice interface{}

type Instrument interface{ NewVoice(int, int) Voice }

var insts = []string{"snare", "kick", "hihat", "tom", "clap"}

func Register(id string, inst Instrument) { insts = append(insts, id) }

func RegisterWAV(id, path string) error { insts = append(insts, id); return nil }

func RegisterWAVDialog() (string, error) {
	id := "user"
	insts = append(insts, id)
	return id, nil
}

// Play is a stub used during tests to avoid initializing audio devices.
func Play(id string, when ...float64) {}

// Now returns 0 during tests.
func Now() float64 { return 0 }

// Resume is a no-op in tests.
func Resume() {}

// Reset is a stub used during tests.
func Reset() {}

var SetBPMFunc = func(int) {}

func SetBPM(bpm int) { SetBPMFunc(bpm) }

// Instruments returns placeholder instrument IDs during tests.
func Instruments() []string { return insts }

func ResetInstruments() { insts = []string{"snare", "kick", "hihat", "tom", "clap"} }
