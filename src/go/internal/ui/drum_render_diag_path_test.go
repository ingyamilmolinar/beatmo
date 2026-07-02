//go:build test

package ui

import "testing"

// Verifies the slim-bar diagnostic records which compositing path produced the
// drum-row frame: "windowed" during locked scrolled playback, "legacy" once
// free. This is the key correlate the JS canvas watcher logs when it catches an
// anomaly, so we can tell whether the artifact came from the windowed sub-image
// blit or the legacy shift-and-fill path.
func TestRowsRenderPathRecorded(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultAudio(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(900, 600)
	var nodes []*uiNode
	for k := 0; k < 5; k++ {
		nodes = append(nodes, g.tryAddNode(k, 0, 0))
	}
	g.start = nodes[0]
	g.graph.StartNodeID = nodes[0].ID
	for k := 0; k < len(nodes); k++ {
		g.addEdge(nodes[k], nodes[(k+1)%len(nodes)])
	}
	g.updateBeatInfos()
	g.SetPlayFunc(func(string, float64, ...float64) {})
	g.drum.changeLength(58)
	g.drum.SetFollow(true)
	defer SetTrackBeatForceRefreshForTest(false)()
	pressPlay(t, g.drum)
	scr := newTrackedImage("probe", 900, 600)
	for abs := 1; abs <= 160; abs++ {
		setPlayStartForAbs(g, abs)
		_ = g.Update()
		g.Draw(scr)
	}
	dv := g.drum
	if dv.Offset <= 0 {
		t.Fatalf("expected scrolled offset, got %d", dv.Offset)
	}
	if dv.lastRowsRenderPath != "windowed" {
		t.Fatalf("locked scrolled playback: lastRowsRenderPath=%q, want \"windowed\"", dv.lastRowsRenderPath)
	}
	dv.SetFollow(false)
	_ = g.Update()
	g.Draw(scr)
	if dv.lastRowsRenderPath != "legacy" {
		t.Fatalf("free playback: lastRowsRenderPath=%q, want \"legacy\"", dv.lastRowsRenderPath)
	}
	d := dv.RenderDiag()
	if d["follow"] != false {
		t.Fatalf("RenderDiag follow=%v, want false", d["follow"])
	}
	if d["renderPath"] != "legacy" {
		t.Fatalf("RenderDiag renderPath=%v, want legacy", d["renderPath"])
	}
	if _, ok := d["offset"]; !ok {
		t.Fatalf("RenderDiag missing offset")
	}
}
