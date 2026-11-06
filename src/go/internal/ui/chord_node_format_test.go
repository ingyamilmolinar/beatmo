package ui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// setNodeTypeAndChord edits the start node in place, the way the sidebar
// does (type on the graph node, spec through SetNodeParams).
func setNodeTypeAndChord(t *testing.T, g *Game, id model.NodeID, typ model.NodeType, c model.ChordSpec) {
	t.Helper()
	n, ok := g.graph.GetNodeByID(id)
	if !ok {
		t.Fatalf("node %d missing", id)
	}
	n.Type = typ
	g.graph.Nodes[id] = n
	p := n.Params
	p.Chord = c
	g.graph.SetNodeParams(id, p)
	g.notifyPredictorNode(id)
}

func nodeOf(t *testing.T, g *Game, id model.NodeID) model.Node {
	t.Helper()
	n, ok := g.graph.GetNodeByID(id)
	if !ok {
		t.Fatalf("node %d missing", id)
	}
	return n
}

// fileNodes decodes an exported project generically, so tests can read and
// rewrite node fields without depending on the exportNode struct.
func fileNodes(t *testing.T, file []byte) (map[string]any, []map[string]any) {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(file, &doc); err != nil {
		t.Fatalf("decode export: %v", err)
	}
	raw, _ := doc["nodes"].([]any)
	var nodes []map[string]any
	for _, r := range raw {
		nodes = append(nodes, r.(map[string]any))
	}
	return doc, nodes
}

func fileNodeOfType(t *testing.T, file []byte, typ string) map[string]any {
	t.Helper()
	_, nodes := fileNodes(t, file)
	for _, n := range nodes {
		if n["type"] == typ {
			return n
		}
	}
	t.Fatalf("export has no %s node: %s", typ, file)
	return nil
}

// rewriteChordNode applies edit to the exported chord node and re-encodes.
func rewriteChordNode(t *testing.T, file []byte, edit func(n map[string]any)) []byte {
	t.Helper()
	doc, _ := fileNodes(t, file)
	for _, r := range doc["nodes"].([]any) {
		if n := r.(map[string]any); n["type"] == "chord" {
			edit(n)
		}
	}
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func chordBlockOf(n map[string]any) string {
	b, _ := json.Marshal(n["chord"])
	return string(b)
}

// onlyNode returns the single visible node of a freshly imported circuit.
func startNodeID(t *testing.T, g *Game) model.NodeID {
	t.Helper()
	if g.start == nil {
		t.Fatal("no start node")
	}
	return g.start.ID
}

func TestChordNodeExportImportRoundTrip(t *testing.T) {
	g := newTestGameForUndo(t)
	id := startNodeID(t, g)
	setNodeTypeAndChord(t, g, id, model.NodeTypeChord, model.ChordSpec{Quality: "maj7", Inversion: 1, Spread: 2})
	file := mustExport(t, g)
	if got := chordBlockOf(fileNodeOfType(t, file, "chord")); got != `{"inversion":1,"quality":"maj7","spread":2}` {
		t.Fatalf("export chord block = %s", got)
	}

	g2 := newTestGameForUndo(t)
	if err := g2.Import(file); err != nil {
		t.Fatalf("import: %v", err)
	}
	n := nodeOf(t, g2, startNodeID(t, g2))
	if n.Type != model.NodeTypeChord || n.Params.Chord != (model.ChordSpec{Quality: "maj7", Inversion: 1, Spread: 2}) {
		t.Fatalf("imported node type=%d chord=%+v", n.Type, n.Params.Chord)
	}
	if again := mustExport(t, g2); string(again) != string(file) {
		t.Fatalf("re-export is not byte-identical\nfirst=%s\nagain=%s", file, again)
	}
}

// A chord collapsed to a single note keeps its spec, and so must the file:
// undo replays the export, so a spec the file dropped would vanish on undo.
func TestDormantChordSpecSurvivesExport(t *testing.T) {
	g := newTestGameForUndo(t)
	id := startNodeID(t, g)
	setNodeTypeAndChord(t, g, id, model.NodeTypeRegular, model.ChordSpec{Quality: "m7"})
	file := mustExport(t, g)
	if got := chordBlockOf(fileNodeOfType(t, file, "regular")); got != `{"quality":"m7"}` {
		t.Fatalf("a dormant spec must export with its node, got chord=%s", got)
	}
	g2 := newTestGameForUndo(t)
	if err := g2.Import(file); err != nil {
		t.Fatal(err)
	}
	n := nodeOf(t, g2, startNodeID(t, g2))
	if n.Type != model.NodeTypeRegular || n.Params.Chord.Quality != "m7" {
		t.Fatalf("dormant spec lost: type=%d chord=%+v", n.Type, n.Params.Chord)
	}
}

func TestImportRejectsInvalidChordBlocks(t *testing.T) {
	cases := map[string]map[string]any{
		"unknown quality":    {"quality": "bogus"},
		"inversion too high": {"quality": "maj", "inversion": 3},
		"negative inversion": {"quality": "maj", "inversion": -1},
		"spread too high":    {"quality": "maj", "spread": 3},
		"no quality":         {"inversion": 1},
	}
	for name, block := range cases {
		t.Run(name, func(t *testing.T) {
			src := newTestGameForUndo(t)
			setNodeTypeAndChord(t, src, startNodeID(t, src), model.NodeTypeChord, model.ChordSpec{Quality: "maj"})
			file := rewriteChordNode(t, mustExport(t, src), func(n map[string]any) { n["chord"] = block })

			g := newTestGameForUndo(t)
			setNodeTypeAndChord(t, g, startNodeID(t, g), model.NodeTypeChord, model.ChordSpec{Quality: "m7"})
			before := mustExport(t, g)
			if err := g.Import(file); err == nil {
				t.Fatalf("import accepted chord block %v", block)
			}
			if after := mustExport(t, g); string(after) != string(before) {
				t.Fatal("a rejected import must leave the current circuit untouched")
			}
		})
	}
}

// Review Focus 5: a chord node with no spec block is a major triad (what the
// type cycle seeds), never a silent or regular node.
func TestImportChordTypeWithoutSpecSeedsMajor(t *testing.T) {
	src := newTestGameForUndo(t)
	setNodeTypeAndChord(t, src, startNodeID(t, src), model.NodeTypeChord, model.ChordSpec{Quality: "maj"})
	file := rewriteChordNode(t, mustExport(t, src), func(n map[string]any) { delete(n, "chord") })
	g := newTestGameForUndo(t)
	if err := g.Import(file); err != nil {
		t.Fatal(err)
	}
	n := nodeOf(t, g, startNodeID(t, g))
	if n.Type != model.NodeTypeChord || n.Params.Chord.Quality != model.ChordDefaultQuality {
		t.Fatalf("type=%d chord=%+v, want a seeded major chord", n.Type, n.Params.Chord)
	}
}

// Review Focus 3: undo/redo across a type change restores the chord, and a
// redo onto the collapsed node keeps the dormant spec.
func TestUndoRedoRestoresChordAcrossTypeChange(t *testing.T) {
	g := newTestGameForUndo(t)
	id := startNodeID(t, g)
	setNodeTypeAndChord(t, g, id, model.NodeTypeChord, model.ChordSpec{Quality: "m7", Inversion: 2})
	g.undoManager.record("chord")
	setNodeTypeAndChord(t, g, id, model.NodeTypeSilent, model.ChordSpec{Quality: "m7", Inversion: 2})
	g.undoManager.record("silent")

	g.undoManager.Undo()
	n := nodeOf(t, g, startNodeID(t, g))
	if n.Type != model.NodeTypeChord || n.Params.Chord != (model.ChordSpec{Quality: "m7", Inversion: 2}) {
		t.Fatalf("undo: type=%d chord=%+v", n.Type, n.Params.Chord)
	}
	g.undoManager.Redo()
	n = nodeOf(t, g, startNodeID(t, g))
	if n.Type != model.NodeTypeSilent || n.Params.Chord.Quality != "m7" {
		t.Fatalf("redo: type=%d chord=%+v (dormant spec must survive)", n.Type, n.Params.Chord)
	}
}

// The chord block is additive: a plain node exports no chord key at all.
func TestPlainNodeExportsNoChordKey(t *testing.T) {
	g := newTestGameForUndo(t)
	if strings.Contains(string(mustExport(t, g)), `"chord"`) {
		t.Fatal("a node without a spec must not write a chord key")
	}
}
