package main

import (
	"encoding/json"
	"testing"
)

// TestGrooveEmit verifies that a Hit carrying Groove/GroovePct emits
// groove_kind/groove_pct on its regular node, and that hits without groove
// omit both keys entirely (omitempty contract the importer relies on).
func TestGrooveEmit(t *testing.T) {
	sc := Showcase{
		Stem: "groove-test", BPM: 120, Subdiv: 16, Bars: 1,
		Insts: []InstSpec{{ID: "kick", Name: "Kick", Volume: 0.8}},
		Rows: []RowSpec{{Inst: "kick", Hits: []Hit{
			{Step: 0, Vol: 0.8},
			{Step: 4, Vol: 0.8, Groove: "delay", GroovePct: 0.25},
		}}},
	}
	b, err := build(sc)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	var d struct {
		Nodes []map[string]any `json:"nodes"`
	}
	if err := json.Unmarshal(b, &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Locate the two regular nodes by their pitch-less regular type + volume.
	var plain, grooved map[string]any
	for _, n := range d.Nodes {
		if n["type"] != "regular" {
			continue
		}
		if _, has := n["groove_kind"]; has {
			grooved = n
		} else {
			plain = n
		}
	}
	if grooved == nil {
		t.Fatal("no regular node carries groove_kind")
	}
	if got := grooved["groove_kind"]; got != "delay" {
		t.Errorf("groove_kind = %v, want delay", got)
	}
	if got := grooved["groove_pct"]; got != 0.25 {
		t.Errorf("groove_pct = %v, want 0.25", got)
	}
	if plain == nil {
		t.Fatal("expected a groove-less regular node")
	}
	if _, has := plain["groove_pct"]; has {
		t.Error("groove-less node must omit groove_pct")
	}
}

// TestGrooveValidation verifies build() rejects bad groove data at generation
// time rather than shipping it into an embedded template.
func TestGrooveValidation(t *testing.T) {
	for _, bad := range []Hit{
		{Step: 0, Groove: "swing", GroovePct: 0.2}, // unknown kind
		{Step: 0, Groove: "delay", GroovePct: 1.5}, // pct out of range
		{Step: 0, Groove: "rush", GroovePct: -0.1}, // pct out of range
	} {
		sc := Showcase{
			Stem: "groove-bad", BPM: 120, Subdiv: 16, Bars: 1,
			Insts: []InstSpec{{ID: "kick", Name: "Kick", Volume: 0.8}},
			Rows:  []RowSpec{{Inst: "kick", Hits: []Hit{bad}}},
		}
		if _, err := build(sc); err == nil {
			t.Errorf("build accepted invalid groove %+v", bad)
		}
	}
}

// TestGrooveHelpers pins the swing/feel helpers' step selection.
func TestGrooveHelpers(t *testing.T) {
	in := []Hit{{Step: 0}, {Step: 1}, {Step: 2}, {Step: 4}, {Step: 6}}
	s8 := swing8(in, 0.15)
	for _, h := range s8 {
		wantDelay := h.Step == 2 || h.Step == 6
		if (h.Groove == "delay") != wantDelay {
			t.Errorf("swing8 step %d groove=%q", h.Step, h.Groove)
		}
	}
	s16 := swing16(in, 0.2)
	for _, h := range s16 {
		if (h.Groove == "delay") != (h.Step%2 == 1) {
			t.Errorf("swing16 step %d groove=%q", h.Step, h.Groove)
		}
	}
	for _, h := range laidBack(in, 0.1) {
		if h.Groove != "delay" || h.GroovePct != 0.1 {
			t.Errorf("laidBack step %d %+v", h.Step, h)
		}
	}
	for _, h := range pushed(in, 0.1) {
		if h.Groove != "rush" {
			t.Errorf("pushed step %d %+v", h.Step, h)
		}
	}
	// pre-existing groove survives untouched
	pre := swing16([]Hit{{Step: 1, Groove: "rush", GroovePct: 0.3}}, 0.2)
	if pre[0].Groove != "rush" || pre[0].GroovePct != 0.3 {
		t.Errorf("existing groove clobbered: %+v", pre[0])
	}
}
