package audio

import (
	"math"
	"testing"
)

func TestPanDefaultCenter(t *testing.T) {
	withDefaultAudio(t)
	ch := newChannel("pan-test", nil)
	if p := ch.Pan(); p != 0 {
		t.Errorf("new channel Pan() = %f, want 0", p)
	}
}

func TestSetPanClampMin(t *testing.T) {
	ch := newChannel("pan-min", nil)
	ch.SetPan(-5)
	if p := ch.Pan(); p != -1 {
		t.Errorf("SetPan(-5) → Pan() = %f, want -1", p)
	}
}

func TestSetPanClampMax(t *testing.T) {
	ch := newChannel("pan-max", nil)
	ch.SetPan(5)
	if p := ch.Pan(); p != 1 {
		t.Errorf("SetPan(5) → Pan() = %f, want 1", p)
	}
}

func TestSetPanNaN(t *testing.T) {
	ch := newChannel("pan-nan", nil)
	ch.SetPan(math.NaN())
	if p := ch.Pan(); p != 0 {
		t.Errorf("SetPan(NaN) → Pan() = %f, want 0", p)
	}
}

func TestSetPanInf(t *testing.T) {
	ch := newChannel("pan-inf-pos", nil)
	ch.SetPan(math.Inf(1))
	if p := ch.Pan(); p != 0 {
		t.Errorf("SetPan(+Inf) → Pan() = %f, want 0", p)
	}

	ch.SetPan(math.Inf(-1))
	if p := ch.Pan(); p != 0 {
		t.Errorf("SetPan(-Inf) → Pan() = %f, want 0", p)
	}
}

func TestPanGainsCenter(t *testing.T) {
	ch := newChannel("pan-center", nil)
	ch.SetPan(0)
	gainL, gainR := ch.PanGains()
	// At center: angle = π/4, cos=sin ≈ 0.707
	if math.Abs(gainL-math.Sqrt2/2) > 0.001 {
		t.Errorf("center gainL = %f, want ≈ 0.707", gainL)
	}
	if math.Abs(gainR-math.Sqrt2/2) > 0.001 {
		t.Errorf("center gainR = %f, want ≈ 0.707", gainR)
	}
}

func TestPanGainsFullLeft(t *testing.T) {
	ch := newChannel("pan-left", nil)
	ch.SetPan(-1)
	gainL, gainR := ch.PanGains()
	// At full left: angle = 0, cos=1, sin=0
	if math.Abs(gainL-1.0) > 0.001 {
		t.Errorf("full-left gainL = %f, want ≈ 1.0", gainL)
	}
	if math.Abs(gainR) > 0.001 {
		t.Errorf("full-left gainR = %f, want ≈ 0.0", gainR)
	}
}

func TestPanGainsFullRight(t *testing.T) {
	ch := newChannel("pan-right", nil)
	ch.SetPan(1)
	gainL, gainR := ch.PanGains()
	// At full right: angle = π/2, cos=0, sin=1
	if math.Abs(gainL) > 0.001 {
		t.Errorf("full-right gainL = %f, want ≈ 0.0", gainL)
	}
	if math.Abs(gainR-1.0) > 0.001 {
		t.Errorf("full-right gainR = %f, want ≈ 1.0", gainR)
	}
}

func TestPanGainsEqualPowerConservation(t *testing.T) {
	ch := newChannel("pan-power", nil)
	for _, pan := range []float64{-1, -0.5, 0, 0.5, 1} {
		ch.SetPan(pan)
		gL, gR := ch.PanGains()
		power := gL*gL + gR*gR
		if math.Abs(power-1.0) > 0.01 {
			t.Errorf("pan=%f: gainL²+gainR² = %f, want ≈ 1.0", pan, power)
		}
	}
}

func TestSetChannelPanAPI(t *testing.T) {
	withDefaultAudio(t)
	id := "pan-api-test"
	_ = InstrumentChannel(id)

	SetChannelPan(id, 0.75)
	got := ChannelPan(id)
	if math.Abs(got-0.75) > 0.001 {
		t.Errorf("ChannelPan(%q) = %f, want 0.75", id, got)
	}
}
