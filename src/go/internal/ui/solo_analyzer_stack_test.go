package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
)

// Soloing only REMOVES scheduler work, so no toggle may add steady audio-thread cost. Each mute/solo re-selects
// the channel, and an EnableChannelAnalyzer that APPENDS stacks another FFT on the audio thread per toggle.

func TestSoloOrganDoesNotStackChannelAnalyzers(t *testing.T) {
	g := newMelodicCircuitGame(t, 0, 5, -2, 7)
	organ := melodicRow(t, g)

	base := audio.ChannelAnalyzerCount(channel.Of(testMelodicInst))
	t.Logf("%s analyzers before any solo: %d", testMelodicInst, base)

	for i := 1; i <= 5; i++ {
		setRowSolo(t, g.drum, organ, true)
		setRowSolo(t, g.drum, organ, false)
		t.Logf("after solo/unsolo cycle %d: %s analyzers=%d", i, testMelodicInst, audio.ChannelAnalyzerCount(channel.Of(testMelodicInst)))
	}

	got := audio.ChannelAnalyzerCount(channel.Of(testMelodicInst))
	if got > 1 {
		t.Errorf("solo stacked analyzers on the melodic channel: count=%d after 5 solo/unsolo cycles (want <=1).\n"+
			"Each one runs a 512-pt FFT on the audio thread every 512 samples and can never be disabled.", got)
	}
}

// reconcileChannelWithAudibleSet selects the same channel on every mute/solo toggle.
func TestSelectAudioChannelIsAnalyzerIdempotent(t *testing.T) {
	g := newMelodicCircuitGame(t, 0, 5, -2, 7)

	for i := 0; i < 8; i++ {
		g.drum.selectAudioChannel(channel.Of(testMelodicInst))
	}
	if got := audio.ChannelAnalyzerCount(channel.Of(testMelodicInst)); got > 1 {
		t.Errorf("8 selects of the same channel left %d live analyzers (want <=1)", got)
	}
}
