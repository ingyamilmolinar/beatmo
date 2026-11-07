//go:build test

package ui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// The lane keeps a row's committed instrument until the new one's voice is rendered: no note late, dropped,
// silent or doubled, and the drum view knows nothing of it.

func heardByAbs(sink *fakeSink) map[int]laneNote {
	out := map[int]laneNote{}
	for k, n := range sink.heard() {
		out[k[1]] = n
	}
	return out
}

func TestLaneSwitchKeepsOldVoiceUntilReady(t *testing.T) {
	l, src, sink, now := newGridLane(t, 1)
	l.Tick() // commits the horizon on "i0"
	src.inst = []instrument.ID{"new"}
	src.notReady = map[instrument.ID]bool{"new": true}
	*now += 0.01
	l.Tick()
	for _, n := range sink.scheduled() {
		if n.Inst != "i0" {
			t.Fatalf("a note was committed with an unrendered voice: %+v", n)
		}
	}
	if len(src.warmed) == 0 || src.warmed[0] != "new" {
		t.Fatalf("the new instrument's voice was never requested: %v", src.warmed)
	}

	src.notReady["new"] = false // its render lands
	*now += 0.01
	l.Tick()
	guard := *now + l.cfg.Guard
	for _, n := range sink.scheduled() {
		if n.Time > guard && n.Inst != "new" {
			t.Fatalf("a not-started note kept the old instrument after the new voice was ready: %+v", n)
		}
	}
	*now += 1.2
	l.Tick()
	seen := map[int]int{}
	for _, n := range sink.scheduled() {
		seen[n.Abs]++
	}
	for abs, c := range seen {
		if c != 1 {
			t.Fatalf("cell %d sounds %d times", abs, c)
		}
	}
}

func TestLaneSwitchKeepsOldVoiceWhileNewNeverReady(t *testing.T) {
	l, src, sink, now := newGridLane(t, 1)
	l.Tick()
	start := *now
	src.inst = []instrument.ID{"broken"}
	src.notReady = map[instrument.ID]bool{"broken": true}
	for i := 0; i < 15; i++ { // 1.5 s: inside the bounded wait
		*now += 0.1
		l.Tick()
	}
	heard := heardByAbs(sink)
	first, last := int(start/0.125)+1, int(*now/0.125)
	for abs := first; abs <= last; abs++ {
		n, ok := heard[abs]
		if !ok {
			t.Fatalf("cell %d went silent while the new voice was not ready", abs)
		}
		if n.Inst != "i0" {
			t.Fatalf("cell %d played %s, whose voice never rendered", abs, n.Inst)
		}
	}
	if s := l.Stats(); s.Late != 0 || s.DroppedLate != 0 {
		t.Fatalf("the switch made notes late: %+v", s)
	}
	if len(src.warmed) < 3 {
		t.Fatalf("the render was requested %d times in 1.5 s; a dropped request must be re-sent", len(src.warmed))
	}
}

// After the bounded wait the user's choice takes effect.
func TestLaneSwitchGivesUpAfterTheBoundedWait(t *testing.T) {
	l, src, sink, now := newGridLane(t, 1)
	l.Tick()
	src.inst = []instrument.ID{"broken"}
	src.notReady = map[instrument.ID]bool{"broken": true}
	for i := 0; i < 30; i++ {
		*now += 0.1
		l.Tick()
	}
	for _, n := range sink.scheduled() {
		if n.Time > *now+0.2 && n.Inst != "broken" {
			t.Fatalf("3 s after the switch a far note still plays the old instrument: %+v", n)
		}
	}
}

// A melodic instrument renders each pitch apart, so a note whose own voice is not ready keeps the old instrument.
func TestLaneSwitchIsDecidedPerNote(t *testing.T) {
	l, src, sink, now := newGridLane(t, 1)
	l.Tick()
	src.inst = []instrument.ID{"new"}
	src.ready = func(n laneNote) bool { return n.Inst != "new" || n.Abs%2 == 0 }
	for i := 0; i < 5; i++ {
		*now += 0.05
		l.Tick()
	}
	for _, n := range sink.scheduled() {
		if n.Time <= *now+l.cfg.Guard {
			continue
		}
		if n.Abs%2 == 1 && n.Inst != "i0" {
			t.Fatalf("cell %d's voice is not rendered on the new instrument, yet it plays it", n.Abs)
		}
		if n.Abs%2 == 0 && n.Inst != "new" {
			t.Fatalf("cell %d's voice is ready, yet it still plays the old instrument", n.Abs)
		}
	}
}

func TestLaneSwitchStateResetsOnStop(t *testing.T) {
	l, src, sink, now := newGridLane(t, 1)
	l.Tick()
	src.inst = []instrument.ID{"new"}
	src.notReady = map[instrument.ID]bool{"new": true}
	*now += 0.01
	l.Tick()
	src.playing = false
	l.Tick()
	src.playing, src.gen = true, src.gen+1
	*now += 0.5
	l.Tick()
	for _, n := range sink.scheduled() {
		if n.Time > *now && n.Inst != "new" {
			t.Fatalf("after stop the lane still sounded the previous instrument: %+v", n)
		}
	}
}

func TestLaneFirstNoteIsNeverRetargeted(t *testing.T) {
	l, src, sink, _ := newGridLane(t, 1)
	src.inst = []instrument.ID{"cold"}
	src.notReady = map[instrument.ID]bool{"cold": true}
	l.Tick()
	if len(sink.scheduled()) == 0 {
		t.Fatal("nothing committed")
	}
	for _, n := range sink.scheduled() {
		if n.Inst != "cold" {
			t.Fatalf("a row with nothing heard yet played %s instead of its instrument", n.Inst)
		}
	}
}

// Switching belongs to the lane: the drum view holds only the instrument a row plays, with nothing for export,
// undo, row delete or import to keep in step.
func TestDrumViewHasNoSwitchState(t *testing.T) {
	for _, typ := range []reflect.Type{reflect.TypeOf(DrumRow{}), reflect.TypeOf(DrumView{})} {
		for i := range typ.NumField() {
			name := strings.ToLower(typ.Field(i).Name)
			if strings.Contains(name, "sounding") || strings.Contains(name, "pendingswitch") || strings.Contains(name, "switching") {
				t.Fatalf("%s.%s: the drum view must not carry audio switching state", typ.Name(), typ.Field(i).Name)
			}
		}
	}
}
