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
		// a rule naming no param → rule dropped, group kept
		map[string]any{"id": 3, "name": "norule", "node_ids": []any{float64(aID)},
			"rules": []any{map[string]any{"param": "", "delta": 1, "every_n": 1}}},
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

// A disabled group with configured rules must come back disabled, never switched on by its rules.
func TestEnabledFlagRoundTripsExportImport(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gidOn, _ := g.graph.CreateGroup("on", []model.NodeID{aID, bID})
	if err := g.graph.SetGroupRules(gidOn, []model.GroupRule{
		{Param: model.GroupParamPitch, Delta: 2, EveryN: 4},
	}); err != nil {
		t.Fatalf("SetGroupRules(on): %v", err)
	}
	if err := g.graph.SetGroupRulesEnabled(gidOn, true); err != nil {
		t.Fatalf("SetGroupRulesEnabled(on): %v", err)
	}

	c := g.tryAddNode(0, 5, model.NodeTypeRegular)
	gidOff, _ := g.graph.CreateGroup("off", []model.NodeID{c.ID})
	if err := g.graph.SetGroupRules(gidOff, []model.GroupRule{
		{Param: model.GroupParamVolume, Delta: -0.1, EveryN: 2},
	}); err != nil {
		t.Fatalf("SetGroupRules(off): %v", err)
	}

	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	g2 := New(testLogger)
	t.Cleanup(g2.CloseForTest)
	g2.Layout(800, 600)
	if err := g2.Import(data); err != nil {
		t.Fatalf("Import: %v", err)
	}

	groups := g2.graph.AllGroups()
	if len(groups) != 2 {
		t.Fatalf("want 2 imported groups, got %d: %+v", len(groups), groups)
	}
	var on, off model.NodeGroup
	for _, grp := range groups {
		switch grp.Name {
		case "on":
			on = grp
		case "off":
			off = grp
		}
	}
	if !on.RulesEnabled {
		t.Fatalf("enabled group must import as RulesEnabled=true, got %+v", on)
	}
	if off.RulesEnabled {
		t.Fatalf("disabled group with configured rules must import as RulesEnabled=false, got %+v", off)
	}

	data2, err := g2.drum.exportBytes()
	if err != nil {
		t.Fatalf("re-Export: %v", err)
	}
	if string(data) != string(data2) {
		t.Fatalf("export -> Import -> export is not byte-faithful:\nfirst:  %s\nsecond: %s", data, data2)
	}
}

// Import must cancel a stranded marquee drag, like every other transient gesture, before it rewrites the graph.
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

// A rule param has one spelling; an unknown one rejects the file before anything resets.
func TestGroupImportRejectsUnknownRuleParam(t *testing.T) {
	g, aID, _ := buildGroupLoopCircuit(t)
	data, _ := g.drum.exportBytes()
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	doc["groups"] = []any{map[string]any{"id": 1, "name": "g", "node_ids": []any{float64(aID)},
		"rules": []any{map[string]any{"param": "sparkle", "delta": 1, "every_n": 1}}}}
	mutated, _ := json.Marshal(doc)
	if err := g.Import(mutated); err == nil {
		t.Fatal(`import accepted rule param "sparkle"`)
	}
}
