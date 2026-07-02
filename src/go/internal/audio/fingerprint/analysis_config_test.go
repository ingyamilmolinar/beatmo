package fingerprint

import "testing"

func TestDefaultAnalysisConfig_SaneValues(t *testing.T) {
	c := DefaultAnalysisConfig()
	if c.HopSec <= 0 || c.FFTSize <= 0 || len(c.Bands) == 0 {
		t.Fatalf("defaults not populated: %+v", c)
	}
	if c.Seed == 0 {
		t.Errorf("Seed default must be non-zero for determinism")
	}
	if c.TempoMinBPM <= 0 || c.TempoMaxBPM <= c.TempoMinBPM {
		t.Errorf("tempo range invalid: %v..%v", c.TempoMinBPM, c.TempoMaxBPM)
	}
}

func TestLoadAnalysisConfig_MergesOverDefaults(t *testing.T) {
	def := DefaultAnalysisConfig()
	got, err := LoadAnalysisConfig([]byte(`{"hop_sec":0.25,"tempo_max_bpm":300}`))
	if err != nil {
		t.Fatal(err)
	}
	if got.HopSec != 0.25 {
		t.Errorf("HopSec=%v want 0.25 (overridden)", got.HopSec)
	}
	if got.TempoMaxBPM != 300 {
		t.Errorf("TempoMaxBPM=%v want 300 (overridden)", got.TempoMaxBPM)
	}
	// Unset fields keep defaults.
	if got.FFTSize != def.FFTSize {
		t.Errorf("FFTSize=%v want default %v (not overridden)", got.FFTSize, def.FFTSize)
	}
	if len(got.Bands) != len(def.Bands) {
		t.Errorf("Bands replaced unexpectedly: %d vs default %d", len(got.Bands), len(def.Bands))
	}
}

func TestLoadAnalysisConfig_BandEdgeRoundTrips(t *testing.T) {
	got, err := LoadAnalysisConfig([]byte(`{"bands":[{"name":"x","lo_hz":100,"hi_hz":900}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Bands) != 1 {
		t.Fatalf("bands=%d want 1", len(got.Bands))
	}
	b := got.Bands[0]
	if b.Name != "x" || b.LoHz != 100 || b.HiHz != 900 {
		t.Errorf("band did not round-trip: %+v (want name=x lo=100 hi=900)", b)
	}
}

func TestDefaultAnalysisConfig_CompareDefaults(t *testing.T) {
	c := DefaultAnalysisConfig()
	if c.CompareWeights.Tempo <= 0 || c.CompareWeights.Key <= 0 || c.CompareWeights.Mix <= 0 || c.CompareWeights.Rhythm <= 0 {
		t.Errorf("compare weights not populated: %+v", c.CompareWeights)
	}
	if !(c.DriftTol > 0 && c.MismatchTol > c.DriftTol) {
		t.Errorf("tolerances invalid: drift=%v mismatch=%v", c.DriftTol, c.MismatchTol)
	}
}
