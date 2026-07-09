package audio

import "testing"

func TestConfigForInstrumentUsesLoudnessAmp(t *testing.T) {
	// Pick any instrument that has a generated loudness amplitude.
	var id string
	var want float32
	for k, v := range instrumentLoudnessAmp {
		id, want = k, v
		break
	}
	if id == "" {
		t.Fatal("instrumentLoudnessAmp is empty — run `make measure-loudness`")
	}
	got := ConfigForInstrument(id).Amplitude
	if got != want {
		t.Errorf("ConfigForInstrument(%q).Amplitude = %v, want loudness amp %v", id, got, want)
	}
}

func TestLoudnessAmpFallsBackWhenAbsent(t *testing.T) {
	if _, ok := loudnessAmp("definitely-not-an-instrument"); ok {
		t.Error("loudnessAmp returned ok for an unknown id")
	}
}
