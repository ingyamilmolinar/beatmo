//go:build js && !test

package audio

// In the browser every effect runs in the C engine in the beatmo-dsp worklet; the Go processor list is bookkeeping
// only, so no Go DSP ships in main.wasm.
func newDistortion(int, map[string]float64) *passThrough   { return &passThrough{} }
func newDelay(int, map[string]float64) *passThrough        { return &passThrough{} }
func newReverb(int, map[string]float64) *passThrough       { return &passThrough{} }
func newChorus(int, map[string]float64) *passThrough       { return &passThrough{} }
func newBitcrusher(int, map[string]float64) *passThrough   { return &passThrough{} }
func newFilter(int, map[string]float64) *passThrough       { return &passThrough{} }
func newWaveshaper(int, map[string]float64) *passThrough   { return &passThrough{} }
func newRingMod(int, map[string]float64) *passThrough      { return &passThrough{} }
func newTremolo(int, map[string]float64) *passThrough      { return &passThrough{} }
func newGate(int, map[string]float64) *passThrough         { return &passThrough{} }
func newLimiter(int, map[string]float64) *passThrough      { return &passThrough{} }
func newFlanger(int, map[string]float64) *passThrough      { return &passThrough{} }
func newPhaser(int, map[string]float64) *passThrough       { return &passThrough{} }
func newAutoWah(int, map[string]float64) *passThrough      { return &passThrough{} }
func newCompressorFX(int, map[string]float64) *passThrough { return &passThrough{} }
func newTransient(int, map[string]float64) *passThrough    { return &passThrough{} }
func newTape(int, map[string]float64) *passThrough         { return &passThrough{} }
func newPitchShift(int, map[string]float64) *passThrough   { return &passThrough{} }
