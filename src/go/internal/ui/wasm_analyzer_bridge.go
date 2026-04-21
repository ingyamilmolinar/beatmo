//go:build js && wasm && !test

package ui

import (
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	scope "github.com/ingyamilmolinar/beatmo/internal/scope"
)

// BuildAnalyzerStateFromSnapshots synthesizes an analyzer.State on WASM by
// calling the JS-backed ChannelAnalyzerSnapshot for master plus every drum row,
// and promoting the active row (if distinct from master) to the detail slot.
func BuildAnalyzerStateFromSnapshots(activeID string, rows []*DrumRow, sampleRate int) *analyzer.State {
	masterSnap := audio.ChannelAnalyzerSnapshot("main")

	rowSnaps := make([]RowSnapshot, 0, len(rows))
	for _, r := range rows {
		if r == nil || r.Instrument == "" {
			continue
		}
		rowSnaps = append(rowSnaps, RowSnapshot{
			ID:   r.Instrument,
			Name: r.Name,
			Snap: audio.ChannelAnalyzerSnapshot(r.Instrument),
		})
	}

	var detail *audio.AnalyzerSnapshot
	var detailID, detailName string
	if activeID != "" && activeID != "main" {
		d := audio.ChannelAnalyzerSnapshot(activeID)
		detail = &d
		detailID = activeID
		for _, r := range rows {
			if r != nil && r.Instrument == activeID {
				detailName = r.Name
				break
			}
		}
	}

	return SynthesizeAnalyzerState("main", "Master", masterSnap, rowSnaps, detail, detailID, detailName, sampleRate)
}

// BuildScopeStateFromSnapshots synthesizes a scope.State on WASM using pre-EQ
// (StageSynth tap) and post-EQ (StageEQ tap) analyzer snapshots. Stages other
// than Synth/EQ render inactive — bridging those would need new JS tap points.
func BuildScopeStateFromSnapshots(instID string, tapA, tapB scope.Stage) *scope.State {
	id := instID
	if id == "" {
		id = "main"
	}
	pre := audio.PreEQAnalyzerSnapshot(id)
	post := audio.ChannelAnalyzerSnapshot(id)
	return SynthesizeScopeState(id, tapA, tapB, pre, post)
}
