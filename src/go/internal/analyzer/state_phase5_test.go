package analyzer

import (
	"testing"
)

// TestPhase5_StereoFieldsDefaultEmpty — fresh ChannelMetrics has zero
// values for stereo fields, meaning renderers can use the existing mono
// fields as a fallback. Phase 5 schema lands first; data wiring follows.
func TestPhase5_StereoFieldsDefaultEmpty(t *testing.T) {
	var ch ChannelMetrics
	if ch.HasStereo() {
		t.Fatalf("fresh ChannelMetrics: HasStereo() must be false")
	}
	if got := ch.PeakL(); got != 0 {
		t.Errorf("PeakL fallback: got %v, want 0", got)
	}
	if got := ch.PeakR(); got != 0 {
		t.Errorf("PeakR fallback: got %v, want 0", got)
	}
}

// TestPhase5_HasStereoWhenAnyFieldSet — once any stereo field is
// populated, HasStereo() returns true so renderers can switch to the
// L/R paint path.
func TestPhase5_HasStereoWhenAnyFieldSet(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*ChannelMetrics)
	}{
		{"WaveformL", func(c *ChannelMetrics) { c.WaveformL = []float64{0.5} }},
		{"WaveformR", func(c *ChannelMetrics) { c.WaveformR = []float64{0.5} }},
		{"FFTBinsL", func(c *ChannelMetrics) { c.FFTBinsL = []float64{-20} }},
		{"FFTBinsR", func(c *ChannelMetrics) { c.FFTBinsR = []float64{-20} }},
		{"PeakDBL", func(c *ChannelMetrics) { c.PeakDBL = -6 }},
		{"PeakDBR", func(c *ChannelMetrics) { c.PeakDBR = -6 }},
		{"RMSDBL", func(c *ChannelMetrics) { c.RMSDBL = -12 }},
		{"RMSDBR", func(c *ChannelMetrics) { c.RMSDBR = -12 }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var ch ChannelMetrics
			c.mut(&ch)
			if !ch.HasStereo() {
				t.Fatalf("HasStereo() after setting %s: got false, want true", c.name)
			}
		})
	}
}

// TestPhase5_FallbackHelpersReturnMonoWhenNoStereo — when no stereo
// data is set, PeakL/PeakR/RMS L/R fall back to the mono PeakDB/RMSDB
// fields. Renderers can call PeakL/PeakR unconditionally and get a
// reasonable value either way.
func TestPhase5_FallbackHelpersReturnMonoWhenNoStereo(t *testing.T) {
	ch := ChannelMetrics{PeakDB: -3, RMSDB: -10}
	if got := ch.PeakL(); got != -3 {
		t.Errorf("PeakL with mono-only: got %v, want -3", got)
	}
	if got := ch.PeakR(); got != -3 {
		t.Errorf("PeakR with mono-only: got %v, want -3", got)
	}
	if got := ch.RMSL(); got != -10 {
		t.Errorf("RMSL with mono-only: got %v, want -10", got)
	}
	if got := ch.RMSR(); got != -10 {
		t.Errorf("RMSR with mono-only: got %v, want -10", got)
	}
}
