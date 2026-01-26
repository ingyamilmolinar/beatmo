package ui

import (
	"testing"

	"github.com/ingyamilmolinar/tunkul/core/model"
	"github.com/ingyamilmolinar/tunkul/internal/timeline"
)

// Regression test: importing a project must clear any previously committed
// timeline history so re-added nodes are not masked by stale immutable entries.
func TestImportClearsTimelineHistory(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Seed immutable playback history at abs=0.
	g.recordTimelineCommitKind(0, 0, true, model.NodeTypeRegular, timeline.CommitKindPlayback)
	if got := g.lastImmutableCommit(0); got != 0 {
		t.Fatalf("expected immutable commit before import, got %d", got)
	}

	// Import a minimal project that re-adds a single node/row.
	data := []byte(`{
		"version": 1,
		"subdiv": 8,
		"bpm": 120,
		"instruments": [{
			"name": "Kick",
			"id": "kick",
			"kind": "sample",
			"volume": 1,
			"origin": 0
		}],
		"nodes": [{
			"id": 0,
			"i": 0,
			"j": 0,
			"type": "regular",
			"inputs": [],
			"outputs": []
		}]
	}`)
	if err := g.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}

	if got := g.lastImmutableCommit(0); got != -1 {
		t.Fatalf("expected timeline cleared on import, got immutable abs=%d", got)
	}
	if val, typ, kind, ok := g.timelineService().CommittedKind(0, 0); ok || val {
		t.Fatalf("expected no committed entry at abs=0 after import, got val=%v ok=%v typ=%v kind=%v service=%p", val, ok, typ, kind, g.timelineService())
	}
}
