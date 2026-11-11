package audio

import (
	"math"
	"testing"
)

func TestFreqResponseUnityFlat(t *testing.T) {
	// All bands at 0 dB should produce a flat 0 dB response everywhere.
	bands := []EQBand{
		{Kind: EQPeaking, Freq: 1000, Q: 1.414, GainDB: 0},
		{Kind: EQPeaking, Freq: 4000, Q: 1.414, GainDB: 0},
	}
	points := ComputeFreqResponse(48000, bands, 64, 20, 20000)
	if len(points) != 64 {
		t.Fatalf("expected 64 points, got %d", len(points))
	}
	for _, p := range points {
		if math.Abs(p.GainDB) > 0.01 {
			t.Errorf("expected ~0 dB at %.1f Hz, got %.4f dB", p.FreqHz, p.GainDB)
		}
	}
}

func TestFreqResponsePeakingBoost(t *testing.T) {
	// +6 dB peak at 1 kHz should show a peak near 1 kHz.
	bands := []EQBand{
		{Kind: EQPeaking, Freq: 1000, Q: 1.414, GainDB: 6},
	}
	points := ComputeFreqResponse(48000, bands, 200, 20, 20000)
	if len(points) == 0 {
		t.Fatal("no points returned")
	}
	// Find the point closest to 1 kHz and verify it's boosted.
	var maxDB float64
	var maxFreq float64
	for _, p := range points {
		if p.GainDB > maxDB {
			maxDB = p.GainDB
			maxFreq = p.FreqHz
		}
	}
	if maxDB < 5.0 {
		t.Errorf("expected peak >= 5 dB, got %.2f dB", maxDB)
	}
	if maxFreq < 500 || maxFreq > 2000 {
		t.Errorf("expected peak near 1 kHz, got %.1f Hz", maxFreq)
	}
	// Verify edges are near 0 dB.
	if math.Abs(points[0].GainDB) > 1.0 {
		t.Errorf("expected ~0 dB at %.1f Hz, got %.2f dB", points[0].FreqHz, points[0].GainDB)
	}
	last := points[len(points)-1]
	if math.Abs(last.GainDB) > 1.0 {
		t.Errorf("expected ~0 dB at %.1f Hz, got %.2f dB", last.FreqHz, last.GainDB)
	}
}

func TestFreqResponseMutedBandIgnored(t *testing.T) {
	// A muted band should have no effect on the response.
	bands := []EQBand{
		{Kind: EQPeaking, Freq: 1000, Q: 1.414, GainDB: 12, Muted: true},
	}
	points := ComputeFreqResponse(48000, bands, 64, 20, 20000)
	if len(points) == 0 {
		t.Fatal("no points returned")
	}
	for _, p := range points {
		if math.Abs(p.GainDB) > 0.01 {
			t.Errorf("muted band should produce flat response, got %.4f dB at %.1f Hz", p.GainDB, p.FreqHz)
		}
	}
}

func TestFreqResponseMultipleBands(t *testing.T) {
	// Two bands boosted: their contributions should be additive in dB.
	single1 := ComputeFreqResponse(48000, []EQBand{
		{Kind: EQPeaking, Freq: 200, Q: 1.414, GainDB: 6},
	}, 64, 20, 20000)
	single2 := ComputeFreqResponse(48000, []EQBand{
		{Kind: EQPeaking, Freq: 8000, Q: 1.414, GainDB: 3},
	}, 64, 20, 20000)
	both := ComputeFreqResponse(48000, []EQBand{
		{Kind: EQPeaking, Freq: 200, Q: 1.414, GainDB: 6},
		{Kind: EQPeaking, Freq: 8000, Q: 1.414, GainDB: 3},
	}, 64, 20, 20000)

	if len(single1) != len(both) || len(single2) != len(both) {
		t.Fatal("point count mismatch")
	}
	for i := range both {
		expected := single1[i].GainDB + single2[i].GainDB
		if math.Abs(both[i].GainDB-expected) > 0.01 {
			t.Errorf("at %.1f Hz: expected %.4f dB (sum), got %.4f dB",
				both[i].FreqHz, expected, both[i].GainDB)
		}
	}
}

func TestFreqResponseLowShelfCut(t *testing.T) {
	// Low shelf cut at 200 Hz should attenuate low frequencies.
	bands := []EQBand{
		{Kind: EQLowShelf, Freq: 200, Q: 0.707, GainDB: -6},
	}
	points := ComputeFreqResponse(48000, bands, 100, 20, 20000)
	if len(points) == 0 {
		t.Fatal("no points returned")
	}
	// Low end should be cut.
	if points[0].GainDB > -3 {
		t.Errorf("expected low freq cut, got %.2f dB at %.1f Hz", points[0].GainDB, points[0].FreqHz)
	}
	// High end should be near unity.
	last := points[len(points)-1]
	if math.Abs(last.GainDB) > 0.5 {
		t.Errorf("expected ~0 dB at high freq, got %.2f dB at %.1f Hz", last.GainDB, last.FreqHz)
	}
}

func TestFreqResponseHighpass(t *testing.T) {
	// HPF at 200 Hz should attenuate below 200 Hz, flat above.
	bands := []EQBand{
		{Kind: EQHighpass, Freq: 200, Q: 0.707},
	}
	points := ComputeFreqResponse(48000, bands, 200, 20, 20000)
	if len(points) == 0 {
		t.Fatal("no points returned")
	}
	// Low end (20-50 Hz) should be significantly cut.
	if points[0].GainDB > -6 {
		t.Errorf("expected strong cut at %.0f Hz, got %.2f dB", points[0].FreqHz, points[0].GainDB)
	}
	// High end (well above cutoff) should be near 0 dB.
	last := points[len(points)-1]
	if math.Abs(last.GainDB) > 0.5 {
		t.Errorf("expected ~0 dB at %.0f Hz, got %.2f dB", last.FreqHz, last.GainDB)
	}
}

func TestFreqResponseLowpass(t *testing.T) {
	// LPF at 5 kHz should attenuate above 5 kHz, flat below.
	bands := []EQBand{
		{Kind: EQLowpass, Freq: 5000, Q: 0.707},
	}
	points := ComputeFreqResponse(48000, bands, 200, 20, 20000)
	if len(points) == 0 {
		t.Fatal("no points returned")
	}
	// Low end should be near 0 dB.
	if math.Abs(points[0].GainDB) > 0.5 {
		t.Errorf("expected ~0 dB at %.0f Hz, got %.2f dB", points[0].FreqHz, points[0].GainDB)
	}
	// High end (20 kHz) should be strongly cut.
	last := points[len(points)-1]
	if last.GainDB > -6 {
		t.Errorf("expected strong cut at %.0f Hz, got %.2f dB", last.FreqHz, last.GainDB)
	}
}

func TestFreqResponseHPFLPFCombo(t *testing.T) {
	// HPF at 100 Hz + LPF at 10 kHz + peaking band. Both extremes attenuated.
	bands := []EQBand{
		{Kind: EQHighpass, Freq: 100, Q: 0.707},
		{Kind: EQPeaking, Freq: 1000, Q: 1.414, GainDB: 3},
		{Kind: EQLowpass, Freq: 10000, Q: 0.707},
	}
	points := ComputeFreqResponse(48000, bands, 200, 20, 20000)
	if len(points) == 0 {
		t.Fatal("no points returned")
	}
	// Low end should be cut by HPF.
	if points[0].GainDB > -3 {
		t.Errorf("expected HPF cut at %.0f Hz, got %.2f dB", points[0].FreqHz, points[0].GainDB)
	}
	// High end should be cut by LPF.
	last := points[len(points)-1]
	if last.GainDB > -3 {
		t.Errorf("expected LPF cut at %.0f Hz, got %.2f dB", last.FreqHz, last.GainDB)
	}
	// Mid range (near 1 kHz) should show the peaking boost.
	var midDB float64
	for _, p := range points {
		if p.FreqHz > 800 && p.FreqHz < 1200 {
			if p.GainDB > midDB {
				midDB = p.GainDB
			}
		}
	}
	if midDB < 2.0 {
		t.Errorf("expected peaking boost in mid range, max was %.2f dB", midDB)
	}
}

func TestComputeBiquadCoeffsInvalid(t *testing.T) {
	// Invalid parameters should return zero coefficients.
	c := ComputeBiquadCoeffs(EQPeaking, 0, 1000, 1, 0)
	if c != (BiquadCoeffs{}) {
		t.Error("expected zero coeffs for sr=0")
	}
	c = ComputeBiquadCoeffs(EQPeaking, 48000, 0, 1, 0)
	if c != (BiquadCoeffs{}) {
		t.Error("expected zero coeffs for freq=0")
	}
	c = ComputeBiquadCoeffs(EQPeaking, 48000, 1000, 0, 0)
	if c != (BiquadCoeffs{}) {
		t.Error("expected zero coeffs for q=0")
	}
}
