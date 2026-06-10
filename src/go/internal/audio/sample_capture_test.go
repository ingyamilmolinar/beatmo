package audio

import "testing"

func TestRenderInstrumentOneShotDeterministic(t *testing.T) {
	a, sr := RenderInstrumentOneShot("kick")
	if len(a) == 0 {
		t.Fatal("RenderInstrumentOneShot returned empty buffer")
	}
	if sr <= 0 {
		t.Fatalf("sample rate = %d, want > 0", sr)
	}
	b, _ := RenderInstrumentOneShot("kick")
	if len(a) != len(b) {
		t.Fatalf("non-deterministic length: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("non-deterministic sample at %d: %v vs %v", i, a[i], b[i])
		}
	}
}

func TestRegisterSamplePCMMakesInstrumentAvailable(t *testing.T) {
	id := "user.sample.testpcm"
	RegisterSamplePCM(id, []float32{0.1, 0.2, 0.3}, 44100)
	found := false
	for _, x := range Instruments() {
		if x == id {
			found = true
		}
	}
	if !found {
		t.Fatalf("%q not registered (not in Instruments())", id)
	}
}
