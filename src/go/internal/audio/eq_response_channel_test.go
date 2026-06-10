package audio

import (
	"math"
	"testing"
)

func TestGetChannelEQResponse_BadInputs(t *testing.T) {
	const channelID = "test.eqresp.bad"
	t.Cleanup(func() { ClearChannelProcessors(channelID) })

	if r := GetChannelEQResponse(channelID, 0, 20, 20000); r != nil {
		t.Errorf("numPoints=0: got non-nil response of length %d", len(r))
	}
	if r := GetChannelEQResponse(channelID, -10, 20, 20000); r != nil {
		t.Errorf("numPoints=-10: got non-nil response of length %d", len(r))
	}
	if r := GetChannelEQResponse(channelID, 32, 20, 20000); r != nil {
		t.Errorf("unset channel: got non-nil response of length %d", len(r))
	}
}

func TestGetChannelEQResponse_BasicShape(t *testing.T) {
	const channelID = "test.eqresp.shape"
	t.Cleanup(func() { ClearChannelProcessors(channelID) })

	SetChannelEQ(channelID, 48000,
		EQBand{Kind: EQPeaking, Freq: 1000, Q: 1.0, GainDB: 6},
	)

	r := GetChannelEQResponse(channelID, 32, 20, 20000)
	if len(r) != 32 {
		t.Fatalf("response length = %d; want 32", len(r))
	}

	for i := 1; i < len(r); i++ {
		if r[i].FreqHz <= r[i-1].FreqHz {
			t.Errorf("freq not strictly increasing at i=%d: %.3f <= %.3f", i, r[i].FreqHz, r[i-1].FreqHz)
		}
	}

	if math.Abs(r[0].FreqHz-20) > 1e-6 {
		t.Errorf("first freq = %.6f; want 20", r[0].FreqHz)
	}
	if math.Abs(r[len(r)-1].FreqHz-20000) > 1e-6 {
		t.Errorf("last freq = %.6f; want 20000", r[len(r)-1].FreqHz)
	}

	bestIdx, bestDist := 0, math.Inf(1)
	for i, p := range r {
		d := math.Abs(math.Log10(p.FreqHz) - math.Log10(1000))
		if d < bestDist {
			bestDist, bestIdx = d, i
		}
	}
	if r[bestIdx].GainDB <= 0 {
		t.Errorf("peaking +6dB at 1kHz: mag at nearest bin (%.1fHz) = %.2fdB; want > 0", r[bestIdx].FreqHz, r[bestIdx].GainDB)
	}
}

func TestGetChannelEQResponse_DefaultSampleRate(t *testing.T) {
	const channelID = "test.eqresp.defaultsr"
	t.Cleanup(func() { ClearChannelProcessors(channelID) })

	SetChannelEQ(channelID, 0,
		EQBand{Kind: EQPeaking, Freq: 1000, Q: 1.0, GainDB: 6},
	)
	r := GetChannelEQResponse(channelID, 16, 100, 10000)
	if r == nil {
		t.Fatalf("response is nil; expected fallback to 48k sample rate")
	}
	if len(r) != 16 {
		t.Errorf("response length = %d; want 16", len(r))
	}
}

func TestChannelEQSnapshot(t *testing.T) {
	const channelID = "test.eqsnap"
	t.Cleanup(func() { ClearChannelProcessors(channelID) })

	got := ChannelEQSnapshot(channelID)
	if got.ID != "" || got.SampleRate != 0 || len(got.Bands) != 0 {
		t.Errorf("empty snapshot: got %+v; want zero value", got)
	}

	SetChannelEQ(channelID, 48000,
		EQBand{Kind: EQPeaking, Freq: 1000, Q: 1.0, GainDB: 3},
		EQBand{Kind: EQHighShelf, Freq: 8000, Q: 0.7, GainDB: -2},
	)
	got = ChannelEQSnapshot(channelID)
	if got.ID != channelID {
		t.Errorf("ID = %q; want %q", got.ID, channelID)
	}
	if got.SampleRate != 48000 {
		t.Errorf("SampleRate = %d; want 48000", got.SampleRate)
	}
	if len(got.Bands) != 2 {
		t.Fatalf("Bands length = %d; want 2", len(got.Bands))
	}
	if got.Bands[0].GainDB != 3 || got.Bands[1].GainDB != -2 {
		t.Errorf("Bands gains = %+v; want [3, -2]", got.Bands)
	}

	got.Bands[0].GainDB = 99
	again := ChannelEQSnapshot(channelID)
	if again.Bands[0].GainDB != 3 {
		t.Errorf("snapshot aliased internal slice: re-read gain = %v; want 3", again.Bands[0].GainDB)
	}
}
