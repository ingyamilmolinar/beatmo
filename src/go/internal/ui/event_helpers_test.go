package ui

import (
	"context"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// awaitEmit subscribes to k on a private bus, runs fn, and returns the first
// matching event delivered to the subscriber. Times out at 2s so a missing
// publish fails loudly. Sources from the helper under test ARE the hot path
// being verified — both payload + Source are inspected by the caller.
//
// Implementation note: we route the helpers through the global bus (that's
// what hooks.PublishWithSource does), so the subscription is on GlobalBus().
// The 2s timeout is generous because the global bus's worker pool is shared
// with other tests in this package.
func awaitEmit(t *testing.T, k hooks.Kind, fn func()) hooks.Event {
	t.Helper()
	got := make(chan hooks.Event, 1)
	unsub := hooks.Subscribe(k, func(e hooks.Event) {
		select {
		case got <- e:
		default:
		}
	})
	t.Cleanup(unsub)
	fn()
	select {
	case e := <-got:
		return e
	case <-time.After(2 * time.Second):
		t.Fatalf("emit helper for %s did not publish within timeout", k)
		return hooks.Event{}
	}
}

// ── Source attribution: every emit must carry the user-code call site,
//    not the emit helper itself. This is the contract round-2 leans on.

// TestEmitSourceIsUserCallerNotHelper verifies a representative emit (Node
// added) records the test file's site, not event_helpers.go.
func TestEmitSourceIsUserCallerNotHelper(t *testing.T) {
	n := &uiNode{ID: 42, I: 3, J: 4}
	_, wantFile, wantLine, _ := runtime.Caller(0)
	e := awaitEmit(t, hooks.EventNodeAdded, func() {
		emitNodeAdded(n, model.NodeTypeRegular) // expected source: this line
	})
	wantLine += 2 // the emit call sits two lines below the runtime.Caller probe

	if !strings.HasSuffix(wantFile, e.Source.File) {
		t.Errorf("source file = %q; want suffix-match against %q", e.Source.File, wantFile)
	}
	if e.Source.Line != wantLine {
		t.Errorf("source line = %d; want %d (the emit call's line)", e.Source.Line, wantLine)
	}
	if !strings.HasPrefix(e.Source.Pkg, "internal/ui") {
		t.Errorf("source pkg = %q; want internal/ui prefix", e.Source.Pkg)
	}
	// Sanity: payload also reached the subscriber.
	p, _ := e.Payload.(hooks.NodeEdit)
	if p.ID != 42 || p.I != 3 || p.J != 4 || p.Type != "regular" {
		t.Errorf("payload = %+v; want id=42 i=3 j=4 type=regular", p)
	}
}

// ── Per-helper publish + payload tests ──────────────────────────────────

func TestEmitNodeDeletedPayload(t *testing.T) {
	e := awaitEmit(t, hooks.EventNodeDeleted, func() { emitNodeDeleted(7, 1, 2) })
	p, _ := e.Payload.(hooks.NodeEdit)
	if p.ID != 7 || p.I != 1 || p.J != 2 {
		t.Errorf("payload = %+v", p)
	}
}

func TestEmitNodeMovedPayload(t *testing.T) {
	e := awaitEmit(t, hooks.EventNodeMoved, func() { emitNodeMoved(9, 5, 6) })
	p, _ := e.Payload.(hooks.NodeEdit)
	if p.ID != 9 || p.I != 5 || p.J != 6 {
		t.Errorf("payload = %+v", p)
	}
}

func TestEmitNodeTypeChangedPayload(t *testing.T) {
	e := awaitEmit(t, hooks.EventNodeTypeChanged, func() {
		emitNodeTypeChanged(11, model.NodeTypeRegular, model.NodeTypeMute)
	})
	p, _ := e.Payload.(hooks.NodeTypePayload)
	if p.ID != 11 || p.OldType != "regular" || p.NewType != "mute" {
		t.Errorf("payload = %+v", p)
	}
}

func TestEmitNodeParamsChangedPayload(t *testing.T) {
	params := model.NodeParams{Volume: 0.7, Pitch: 1.2, LogicKind: "probability", LogicP: 0.5}
	e := awaitEmit(t, hooks.EventNodeParamsChanged, func() {
		emitNodeParamsChanged(13, params)
	})
	p, _ := e.Payload.(hooks.NodeParamsPayload)
	if p.ID != 13 || p.Volume != 0.7 || p.LogicKind != "probability" {
		t.Errorf("payload = %+v", p)
	}
}

func TestEmitStartNodeChangedPayload(t *testing.T) {
	e := awaitEmit(t, hooks.EventStartNodeChanged, func() { emitStartNodeChanged(2, 17) })
	p, _ := e.Payload.(hooks.StartNodePayload)
	if p.Row != 2 || p.ID != 17 {
		t.Errorf("payload = %+v", p)
	}
}

func TestEmitRowAddedPayload(t *testing.T) {
	e := awaitEmit(t, hooks.EventRowAdded, func() { emitRowAdded(0, "kick", "Kick") })
	p, _ := e.Payload.(hooks.RowChangePayload)
	if p.Row != 0 || p.Instrument != "kick" || p.Name != "Kick" {
		t.Errorf("payload = %+v", p)
	}
}

func TestEmitRowDeletedPayload(t *testing.T) {
	e := awaitEmit(t, hooks.EventRowDeleted, func() { emitRowDeleted(3) })
	p, _ := e.Payload.(hooks.RowChangePayload)
	if p.Row != 3 {
		t.Errorf("payload = %+v", p)
	}
}

func TestEmitRowMutePayload(t *testing.T) {
	e := awaitEmit(t, hooks.EventRowMute, func() { emitRowMute(1, true) })
	p, _ := e.Payload.(hooks.RowChangePayload)
	if p.Row != 1 || !p.Mute {
		t.Errorf("payload = %+v", p)
	}
}

func TestEmitRowSoloPayload(t *testing.T) {
	e := awaitEmit(t, hooks.EventRowSolo, func() { emitRowSolo(2, true) })
	p, _ := e.Payload.(hooks.RowChangePayload)
	if p.Row != 2 || !p.Solo {
		t.Errorf("payload = %+v", p)
	}
}

func TestEmitEQBandChange(t *testing.T) {
	e := awaitEmit(t, hooks.EventEQBandChange, func() { emitEQBandChange("snare", 4, -2.5) })
	p, _ := e.Payload.(hooks.EQBandPayload)
	if p.Channel != "snare" || p.Band != 4 || p.GainDB != -2.5 {
		t.Errorf("payload = %+v", p)
	}
}

func TestEmitInsertEffectAdded(t *testing.T) {
	e := awaitEmit(t, hooks.EventInsertEffectAdded, func() {
		emitInsertEffectAdded("kick", 1, "delay")
	})
	p, _ := e.Payload.(hooks.InsertEffectPayload)
	if p.Channel != "kick" || p.Slot != 1 || p.Type != "delay" {
		t.Errorf("payload = %+v", p)
	}
}

func TestEmitInsertEffectRemoved(t *testing.T) {
	e := awaitEmit(t, hooks.EventInsertEffectRemoved, func() {
		emitInsertEffectRemoved("kick", 0)
	})
	p, _ := e.Payload.(hooks.InsertEffectPayload)
	if p.Channel != "kick" || p.Slot != 0 {
		t.Errorf("payload = %+v", p)
	}
}

func TestEmitInsertEffectParam(t *testing.T) {
	e := awaitEmit(t, hooks.EventInsertEffectParam, func() {
		emitInsertEffectParam("snare", 0, "mix", 0.42)
	})
	p, _ := e.Payload.(hooks.InsertEffectPayload)
	if p.Channel != "snare" || p.Param != "mix" || p.Value != 0.42 {
		t.Errorf("payload = %+v", p)
	}
}

func TestEmitBPMChange(t *testing.T) {
	e := awaitEmit(t, hooks.EventBPMChange, func() { emitBPMChange(132) })
	p, _ := e.Payload.(hooks.BPMPayload)
	if p.BPM != 132 {
		t.Errorf("payload = %+v", p)
	}
}

func TestEmitImport(t *testing.T) {
	e := awaitEmit(t, hooks.EventImport, func() { emitImport(1024, 12, 3) })
	p, _ := e.Payload.(hooks.ImportPayload)
	if p.Bytes != 1024 || p.Nodes != 12 || p.Rows != 3 {
		t.Errorf("payload = %+v", p)
	}
}

func TestEmitRowColorChanged(t *testing.T) {
	e := awaitEmit(t, hooks.EventRowColorChanged, func() { emitRowColorChanged(2, 0xFF8040FF) })
	p, _ := e.Payload.(hooks.RowColorPayload)
	if p.Row != 2 || p.Color != 0xFF8040FF {
		t.Errorf("payload = %+v", p)
	}
}

func TestEmitCustomWAVLoaded(t *testing.T) {
	e := awaitEmit(t, hooks.EventCustomWAVLoaded, func() { emitCustomWAVLoaded("snare2", true) })
	p, _ := e.Payload.(hooks.CustomWAVPayload)
	if p.InstrumentID != "snare2" || !p.IsUpdate {
		t.Errorf("payload = %+v", p)
	}
}

func TestEmitInstrumentRenamed(t *testing.T) {
	e := awaitEmit(t, hooks.EventInstrumentRenamed, func() {
		emitInstrumentRenamed("kick", "kick_v2")
	})
	p, _ := e.Payload.(hooks.InstrumentRenamePayload)
	if p.OldID != "kick" || p.NewID != "kick_v2" {
		t.Errorf("payload = %+v", p)
	}
}

func TestEmitSceneApplied(t *testing.T) {
	e := awaitEmit(t, hooks.EventSceneApplied, func() { emitSceneApplied("transport_idle") })
	p, _ := e.Payload.(hooks.ScenePayload)
	if p.Name != "transport_idle" {
		t.Errorf("payload = %+v", p)
	}
}

func TestEmitUIStateApplied(t *testing.T) {
	e := awaitEmit(t, hooks.EventUIStateApplied, func() { emitUIStateApplied("/tmp/ui.json") })
	p, _ := e.Payload.(hooks.UIStatePayload)
	if p.Path != "/tmp/ui.json" {
		t.Errorf("payload = %+v", p)
	}
}

func TestEmitFavoriteToggled(t *testing.T) {
	e := awaitEmit(t, hooks.EventFavoriteToggled, func() { emitFavoriteToggled("kick", true) })
	p, _ := e.Payload.(hooks.FavoritePayload)
	if p.InstrumentID != "kick" || !p.IsFavorite {
		t.Errorf("payload = %+v", p)
	}
}

// ── Sanity: every emit passes through the source-aware bus (i.e. its
//    Source is non-zero). Catches a future regression where someone routes
//    a helper through the legacy PublishKind by accident.

func TestAllEmitsCarrySource(t *testing.T) {
	type emit struct {
		name string
		kind hooks.Kind
		fn   func()
	}
	cases := []emit{
		{"NodeAdded", hooks.EventNodeAdded, func() {
			emitNodeAdded(&uiNode{ID: 1, I: 0, J: 0}, model.NodeTypeRegular)
		}},
		{"RowAdded", hooks.EventRowAdded, func() { emitRowAdded(0, "kick", "Kick") }},
		{"RowMute", hooks.EventRowMute, func() { emitRowMute(0, true) }},
		{"BPMChange", hooks.EventBPMChange, func() { emitBPMChange(120) }},
		{"EQBandChange", hooks.EventEQBandChange, func() { emitEQBandChange("kick", 0, 0) }},
		{"InsertEffectAdded", hooks.EventInsertEffectAdded, func() { emitInsertEffectAdded("kick", 0, "reverb") }},
		{"RowColorChanged", hooks.EventRowColorChanged, func() { emitRowColorChanged(0, 0) }},
		{"FavoriteToggled", hooks.EventFavoriteToggled, func() { emitFavoriteToggled("kick", false) }},
	}
	for _, tc := range cases {
		e := awaitEmit(t, tc.kind, tc.fn)
		if e.Source.IsZero() {
			t.Errorf("%s: Source missing — helper bypassed PublishWithSource", tc.name)
		}
		if !strings.HasPrefix(e.Source.Pkg, "internal/ui") {
			t.Errorf("%s: Source.Pkg = %q; want internal/ui prefix", tc.name, e.Source.Pkg)
		}
	}
}

// Suppress unused-import warning when the file lives without other usage.
var _ = context.TODO
var _ sync.Mutex
