//go:build test

package ui

import "testing"

// The go-to wheel's beat labels are 1-BASED for the user (step 0 = "Beat 1",
// the same number the beat counter shows), with the subdivision as a reduced
// fraction when the position is off the beat.
func TestFormatGotoBeat(t *testing.T) {
	cases := []struct {
		steps, div int
		want       string
	}{
		{0, 8, "Beat 1"},
		{8, 8, "Beat 2"},
		{19, 8, "Beat 3 · 3/8"},
		{12, 8, "Beat 2 · 1/2"},
		{6, 4, "Beat 2 · 1/2"},
		{5, 1, "Beat 6"},
		{-3, 8, "Beat 1"},
	}
	for _, c := range cases {
		if got := formatGotoBeat(c.steps, c.div); got != c.want {
			t.Errorf("formatGotoBeat(%d, %d) = %q, want %q", c.steps, c.div, got, c.want)
		}
	}
}

func TestGotoBeatModelLadderAndRange(t *testing.T) {
	m := newGotoBeatModel(8, 64*8, 19)
	if m.Value != 19 || m.Min != 0 || m.Max != 512 {
		t.Fatalf("range: %+v", m)
	}
	want := []float64{1, 8, 32, 128}
	if len(m.Ladder) != len(want) {
		t.Fatalf("ladder %v, want %v", m.Ladder, want)
	}
	for i := range want {
		if m.Ladder[i] != want[i] {
			t.Fatalf("ladder %v, want %v", m.Ladder, want)
		}
	}
	if m.Ladder[m.Rung] != 8 {
		t.Errorf("default rung must be one beat, got step %v", m.Ladder[m.Rung])
	}
	if got := []string{m.LadderLabel(0), m.LadderLabel(1), m.LadderLabel(2), m.LadderLabel(3)}; got[0] != "1/8" || got[1] != "1 beat" || got[2] != "4 beats" || got[3] != "16 beats" {
		t.Errorf("ladder labels %v", got)
	}
	// One beat up from 3 3/8 snaps onto beat 4 (step 24).
	m.Step(1)
	if m.Value != 24 || m.CenterLabel() != "Beat 4" {
		t.Errorf("one beat up from 19 = %v (%s), want 24 (Beat 4)", m.Value, m.CenterLabel())
	}
	// Past the end clamps to the timeline end.
	m.SetLadderIndex(3)
	for i := 0; i < 10; i++ {
		m.Step(1)
	}
	if m.Value != 512 {
		t.Errorf("clamp at end: %v", m.Value)
	}
}

func TestGotoBeatModelHidesTheSubdivisionRungWhenDivIsOne(t *testing.T) {
	m := newGotoBeatModel(1, 32, 0)
	if len(m.Ladder) != 3 || m.Ladder[0] != 1 || m.LadderLabel(0) != "1 beat" {
		t.Fatalf("div=1 ladder %v (%q)", m.Ladder, m.LadderLabel(0))
	}
	if m.Ladder[m.Rung] != 1 {
		t.Errorf("default rung = one beat")
	}
}

func TestGotoTimeModel(t *testing.T) {
	m := newGotoTimeModel(200, 24)
	if m.CenterLabel() != "0:24" {
		t.Errorf("centre %q", m.CenterLabel())
	}
	if m.Ladder[m.Rung] != 1 {
		t.Errorf("default rung 1 s")
	}
	if got := []string{m.LadderLabel(0), m.LadderLabel(1), m.LadderLabel(2), m.LadderLabel(3)}; got[0] != "1s" || got[1] != "5s" || got[2] != "15s" || got[3] != "1m" {
		t.Errorf("ladder labels %v", got)
	}
	m.SetLadderIndex(3)
	m.Step(1)
	if m.CenterLabel() != "1:00" {
		t.Errorf("one minute up from 0:24 = %s", m.CenterLabel())
	}
}

func TestGotoConversionsRoundTrip(t *testing.T) {
	// 120 BPM, div 8: Beat 13 (step 96, 12 beats in) is 6 s in; 6 s resolves
	// back to Beat 13.
	if s := gotoStepsToSeconds(96, 8, 120); s != 6 {
		t.Errorf("steps→sec = %d", s)
	}
	if st := timeToSteps(6, 120, 8, 512); st != 96 {
		t.Errorf("sec→steps = %d", st)
	}
	// Mid-beat rounds DOWN to whole seconds.
	if s := gotoStepsToSeconds(103, 8, 120); s != 6 {
		t.Errorf("mid-beat steps→sec = %d", s)
	}
}
