package ui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

func TestGroupExportImportRoundTrip(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid, _ := g.graph.CreateGroup("melody", []model.NodeID{aID, bID})
	_ = g.graph.SetGroupRules(gid, []model.GroupRule{
		{Param: model.GroupParamPitch, Delta: 2, EveryN: 4},
	})
	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if !strings.Contains(string(data), "\"groups\"") {
		t.Fatalf("export missing groups array:\n%s", data)
	}
	if err := g.Import(data); err != nil {
		t.Fatalf("Import: %v", err)
	}
	groups := g.graph.AllGroups()
	if len(groups) != 1 {
		t.Fatalf("want 1 group after re-import, got %d", len(groups))
	}
	grp := groups[0]
	if grp.Name != "melody" || len(grp.NodeIDs) != 2 || len(grp.Rules) != 1 {
		t.Fatalf("round-trip mismatch: %+v", grp)
	}
	r := grp.Rules[0]
	if r.Param != model.GroupParamPitch || r.Delta != 2 || r.EveryN != 4 || r.Min != -24 || r.Max != 24 {
		t.Fatalf("rule mismatch: %+v", r)
	}
	// Node-ID remap: members must reference nodes that exist post-import.
	for _, n := range grp.NodeIDs {
		if _, ok := g.graph.GetNodeByID(n); !ok {
			t.Fatalf("group references missing node %d after import", n)
		}
	}
}

func TestGroupImportValidation(t *testing.T) {
	g, aID, _ := buildGroupLoopCircuit(t)
	data, _ := g.drum.exportBytes()
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	doc["groups"] = []any{
		// unknown node id 999 dropped; a survives
		map[string]any{"id": 1, "name": "ok", "node_ids": []any{float64(aID), float64(999)},
			"rules": []any{map[string]any{"param": "pitch", "delta": 2, "every_n": 0}}}, // every_n floored to 1
		// all-unknown members → group dropped entirely
		map[string]any{"id": 2, "name": "gone", "node_ids": []any{float64(777)}},
		// bad rule param → rule dropped, group kept
		map[string]any{"id": 3, "name": "norule", "node_ids": []any{float64(aID)},
			"rules": []any{map[string]any{"param": "sparkle", "delta": 1, "every_n": 1}}},
	}
	mutated, _ := json.Marshal(doc)
	if err := g.Import(mutated); err != nil {
		t.Fatalf("Import: %v", err)
	}
	groups := g.graph.AllGroups()
	if len(groups) != 2 {
		t.Fatalf("want 2 surviving groups, got %d: %+v", len(groups), groups)
	}
	if len(groups[0].NodeIDs) != 1 {
		t.Fatalf("unknown member should be dropped: %+v", groups[0])
	}
	if len(groups[0].Rules) != 1 || groups[0].Rules[0].EveryN != 1 {
		t.Fatalf("every_n must floor to 1: %+v", groups[0].Rules)
	}
	if len(groups[1].Rules) != 0 {
		t.Fatalf("bad rule must be dropped per-rule: %+v", groups[1])
	}
}

func TestLegacyFileWithoutGroupsImportsClean(t *testing.T) {
	g, _, _ := buildGroupLoopCircuit(t)
	gid, _ := g.graph.CreateGroup("", []model.NodeID{g.beatInfoAtRow(0, 0).NodeID})
	_ = gid
	data, _ := g.drum.exportBytes()
	var doc map[string]any
	_ = json.Unmarshal(data, &doc)
	delete(doc, "groups")
	legacy, _ := json.Marshal(doc)
	if err := g.Import(legacy); err != nil {
		t.Fatalf("Import legacy: %v", err)
	}
	// Import is replace-not-merge: no groups after importing a group-less file.
	if n := len(g.graph.AllGroups()); n != 0 {
		t.Fatalf("legacy import must clear groups, got %d", n)
	}
}

// TestImportCancelsActiveMarquee is the final-review follow-up: Import must
// cancel a stranded marquee drag (mirroring dismissLongPressPopup /
// cancelConnectMode / cancelMoveMode / groupMenu.Close(), all of which
// already clear transient gesture state before an import mutates the graph
// out from under it).
func TestImportCancelsActiveMarquee(t *testing.T) {
	g, _, _ := buildGroupLoopCircuit(t)
	data, _ := g.drum.exportBytes()

	g.marquee = marqueeDrag{active: true, startX: 10, startY: 10, curX: 50, curY: 50}
	if err := g.Import(data); err != nil {
		t.Fatalf("Import: %v", err)
	}
	if g.marquee.active {
		t.Fatalf("Import must cancel an active marquee, got %+v", g.marquee)
	}
}
