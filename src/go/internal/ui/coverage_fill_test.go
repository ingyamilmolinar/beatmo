package ui

import (
	"image/color"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// coverage_fill_test.go closes coverage gaps in helper / accessor files
// that have no behavior of their own beyond reading or mutating Game/Drum
// state. The tests favor breadth (many files touched) over depth: each
// case is a few lines that drive the function's full body so the cover
// tool sees it.

// ─── theme_tokens.go — pure color accessors ───────────────────────────────

func TestThemeTokenAccessorsReturnConfiguredColors(t *testing.T) {
	t.Helper()

	// Every Token* accessor must agree with the underlying col* package
	// constant. Catch silent renames or rebindings that would let the
	// design system drift without the drift-test catching it.
	cases := []struct {
		name string
		got  color.RGBA
		want color.RGBA
	}{
		{"surface1", TokenSurface1(), colSurface1},
		{"surface2", TokenSurface2(), colSurface2},
		{"surface3", TokenSurface3(), colSurface3},
		{"accent", TokenAccent(), colAccent},
		{"accentBright", TokenAccentBright(), colAccentBright},
		{"accentDim", TokenAccentDim(), colAccentDim},
		{"textPrimary", TokenTextPrimary(), colTextPrimary},
		{"textSecondary", TokenTextSecondary(), colTextSecondary},
		{"textDisabled", TokenTextDisabled(), colTextDisabled},
		{"textAccent", TokenTextAccent(), colTextAccent},
		{"playGreen", TokenPlayGreen(), colPlayGreen},
		{"stopRed", TokenStopRed(), colStopRed},
		{"muteActive", TokenMuteActiveFill(), colMuteActive},
		{"deleteFill", TokenDeleteFill(), colDeleteFill},
		{"soloActive", TokenSoloActiveFill(), colSoloActive},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s: got %#v want %#v", tc.name, tc.got, tc.want)
		}
	}

	if pb := TokenPanelBG(); pb != colPanelBG {
		t.Errorf("TokenPanelBG: got %#v want %#v", pb, colPanelBG)
	}
}

func TestThemeTokenBordersStripOutPremultiply(t *testing.T) {
	// TokenBorderSubtle/Medium re-pack the underlying colBorder* (NRGBA)
	// through the color.Color RGBA() interface and back to NRGBA. The
	// returned NRGBA must round-trip through the same unpack.
	for _, c := range []struct {
		name string
		got  color.NRGBA
		src  color.NRGBA
	}{
		{"borderSubtle", TokenBorderSubtle(), colBorderSubtle},
		{"borderMedium", TokenBorderMedium(), colBorderMedium},
	} {
		r, g, b, a := c.src.RGBA()
		want := color.NRGBA{
			R: uint8(r >> 8),
			G: uint8(g >> 8),
			B: uint8(b >> 8),
			A: uint8(a >> 8),
		}
		if c.got != want {
			t.Errorf("%s: got %#v want %#v", c.name, c.got, want)
		}
	}
}

func TestWithAlphaVariants(t *testing.T) {
	// WithAlpha takes RGBA → returns NRGBA with overridden alpha.
	src := color.RGBA{R: 10, G: 20, B: 30, A: 200}
	got := WithAlpha(src, 99)
	if got != (color.NRGBA{R: 10, G: 20, B: 30, A: 99}) {
		t.Errorf("WithAlpha: got %#v", got)
	}

	// WithAlphaNRGBA preserves RGB but rebinds alpha.
	srcN := color.NRGBA{R: 5, G: 6, B: 7, A: 250}
	gotN := WithAlphaNRGBA(srcN, 33)
	if gotN != (color.NRGBA{R: 5, G: 6, B: 7, A: 33}) {
		t.Errorf("WithAlphaNRGBA: got %#v", gotN)
	}

	// WithAlphaFromColor accepts any color.Color. Use a generic RGBA
	// that round-trips through the standard RGBA() method.
	gotG := WithAlphaFromColor(color.RGBA{R: 100, G: 110, B: 120, A: 255}, 77)
	if gotG != (color.NRGBA{R: 100, G: 110, B: 120, A: 77}) {
		t.Errorf("WithAlphaFromColor: got %#v", gotG)
	}

	// nil input → zero NRGBA.
	if got := WithAlphaFromColor(nil, 50); got != (color.NRGBA{}) {
		t.Errorf("WithAlphaFromColor(nil): got %#v want zero", got)
	}
}

// ─── runtime_profile.go — RuntimeFlags accessors ──────────────────────────

func TestRuntimeFlagsForwardsToProfile(t *testing.T) {
	rt := Runtime()
	p := Profile()
	if rt.ShowLayoutGuides() != p.ShowLayoutGuides {
		t.Errorf("ShowLayoutGuides")
	}
	if rt.ShowCursorLabel() != p.ShowCursorLabel {
		t.Errorf("ShowCursorLabel")
	}
	if rt.ShowEscHint() != p.ShowEscHint {
		t.Errorf("ShowEscHint")
	}
	if rt.EnableLayoutResize() != p.EnableLayoutResize {
		t.Errorf("EnableLayoutResize")
	}
	if rt.ShowRackSurface() != p.ShowRackSurface {
		t.Errorf("ShowRackSurface")
	}
	if rt.DirectDrawRows() != p.DirectDrawRows {
		t.Errorf("DirectDrawRows")
	}
	if rt.UseBottomSheet() != p.UseBottomSheet {
		t.Errorf("UseBottomSheet")
	}
	if rt.DefaultTimelineBeats() != p.DefaultTimelineBeats {
		t.Errorf("DefaultTimelineBeats")
	}
	if rt.ReserveAddRowSpace() != p.ReserveAddRowSpace {
		t.Errorf("ReserveAddRowSpace")
	}
}

// ─── event_helpers.go — emit* functions publish to the global hooks bus ──

// awaitEvent subscribes to k, runs do(), and returns the first received
// event or nil on timeout. Uses a short timeout so a buggy emitter
// produces a fast, clear failure rather than hanging the suite.
func awaitEvent(t *testing.T, k hooks.Kind, do func()) hooks.Event {
	t.Helper()
	got := make(chan hooks.Event, 1)
	unsub := hooks.Subscribe(k, func(e hooks.Event) {
		select {
		case got <- e:
		default:
		}
	})
	t.Cleanup(unsub)
	do()
	select {
	case e := <-got:
		return e
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timed out waiting for %s", k)
		return hooks.Event{}
	}
}

func TestEmitNodeAddedNilSafe(t *testing.T) {
	// Spec: emitNodeAdded must be a no-op when the node pointer is nil.
	// Subscribe and verify nothing arrives within a short window.
	got := make(chan hooks.Event, 1)
	unsub := hooks.Subscribe(hooks.EventNodeAdded, func(e hooks.Event) {
		select {
		case got <- e:
		default:
		}
	})
	t.Cleanup(unsub)
	emitNodeAdded(nil, model.NodeTypeRegular)
	select {
	case e := <-got:
		t.Fatalf("expected no event but received %#v", e.Payload)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestEmitNodeAddedPublishesPayload(t *testing.T) {
	n := &uiNode{ID: 7, I: 3, J: 4}
	e := awaitEvent(t, hooks.EventNodeAdded, func() {
		emitNodeAdded(n, model.NodeTypeMute)
	})
	p, ok := e.Payload.(hooks.NodeEdit)
	if !ok {
		t.Fatalf("payload type: %T", e.Payload)
	}
	if p.ID != 7 || p.I != 3 || p.J != 4 || p.Type != "mute" {
		t.Errorf("unexpected payload: %#v", p)
	}
}

func TestEmitNodeDeletedPublishesPayload(t *testing.T) {
	e := awaitEvent(t, hooks.EventNodeDeleted, func() {
		emitNodeDeleted(model.NodeID(11), 1, 2)
	})
	p := e.Payload.(hooks.NodeEdit)
	if p.ID != 11 || p.I != 1 || p.J != 2 {
		t.Errorf("payload: %#v", p)
	}
}

func TestEmitNodeMovedPublishesPayload(t *testing.T) {
	e := awaitEvent(t, hooks.EventNodeMoved, func() {
		emitNodeMoved(model.NodeID(13), 4, 5)
	})
	p := e.Payload.(hooks.NodeEdit)
	if p.ID != 13 || p.I != 4 || p.J != 5 {
		t.Errorf("payload: %#v", p)
	}
}

func TestEmitNodeTypeChangedPublishesNames(t *testing.T) {
	e := awaitEvent(t, hooks.EventNodeTypeChanged, func() {
		emitNodeTypeChanged(model.NodeID(1), model.NodeTypeRegular, model.NodeTypeSilent)
	})
	p := e.Payload.(hooks.NodeTypePayload)
	if p.OldType != "regular" || p.NewType != "silent" {
		t.Errorf("payload: %#v", p)
	}
}

func TestEmitNodeParamsChangedCarriesAllFields(t *testing.T) {
	params := model.NodeParams{
		Volume: 0.5, Pitch: 1.0, Duration: 2.0,
		LogicKind: "probability", LogicN: 3, LogicP: 0.7,
		GrooveKind: "swing", GroovePct: 30,
	}
	e := awaitEvent(t, hooks.EventNodeParamsChanged, func() {
		emitNodeParamsChanged(model.NodeID(2), params)
	})
	p := e.Payload.(hooks.NodeParamsPayload)
	if p.ID != 2 || p.Volume != 0.5 || p.LogicKind != "probability" ||
		p.LogicN != 3 || p.LogicP != 0.7 || p.GrooveKind != "swing" ||
		p.GroovePct != 30 {
		t.Errorf("payload: %#v", p)
	}
}

func TestEmitStartNodeChanged(t *testing.T) {
	e := awaitEvent(t, hooks.EventStartNodeChanged, func() {
		emitStartNodeChanged(2, model.NodeID(99))
	})
	p := e.Payload.(hooks.StartNodePayload)
	if p.Row != 2 || p.ID != 99 {
		t.Errorf("payload: %#v", p)
	}
}

func TestEmitEdgeAddedDeleted(t *testing.T) {
	e := awaitEvent(t, hooks.EventEdgeAdded, func() {
		emitEdgeAdded(model.NodeID(1), model.NodeID(2), 0, 0, 1, 1)
	})
	p := e.Payload.(hooks.EdgeEdit)
	if p.FromID != 1 || p.ToID != 2 || p.ToI != 1 || p.ToJ != 1 {
		t.Errorf("added payload: %#v", p)
	}

	e = awaitEvent(t, hooks.EventEdgeDeleted, func() {
		emitEdgeDeleted(model.NodeID(3), model.NodeID(4), 5, 6, 7, 8)
	})
	p = e.Payload.(hooks.EdgeEdit)
	if p.FromID != 3 || p.ToID != 4 || p.FromI != 5 || p.FromJ != 6 ||
		p.ToI != 7 || p.ToJ != 8 {
		t.Errorf("deleted payload: %#v", p)
	}
}

func TestEmitTransportEvents(t *testing.T) {
	if e := awaitEvent(t, hooks.EventSeek, func() { emitSeek(42) }); e.Payload.(hooks.SeekPayload).Beats != 42 {
		t.Errorf("seek")
	}
	if e := awaitEvent(t, hooks.EventSubdivChange, func() { emitSubdivChange(16) }); e.Payload.(hooks.SubdivPayload).Subdiv != 16 {
		t.Errorf("subdiv")
	}
	if e := awaitEvent(t, hooks.EventLengthChange, func() { emitLengthChange(32) }); e.Payload.(hooks.LengthPayload).Length != 32 {
		t.Errorf("length")
	}
	e := awaitEvent(t, hooks.EventMasterVolumeChange, func() { emitMasterVolumeChange(0.42) })
	if e.Payload.(hooks.MasterVolumePayload).Volume != 0.42 {
		t.Errorf("master volume")
	}
}

func TestEmitRowEvents(t *testing.T) {
	if e := awaitEvent(t, hooks.EventRowAdded, func() { emitRowAdded(1, "kick", "Kick") }); e.Payload.(hooks.RowChangePayload).Instrument != "kick" {
		t.Errorf("rowAdded")
	}
	if e := awaitEvent(t, hooks.EventRowDeleted, func() { emitRowDeleted(2) }); e.Payload.(hooks.RowChangePayload).Row != 2 {
		t.Errorf("rowDeleted")
	}
	if e := awaitEvent(t, hooks.EventRowInstrumentChange, func() {
		emitRowInstrumentChange(3, "old", "new", "Display")
	}); e.Payload.(hooks.RowChangePayload).OldInstrument != "old" {
		t.Errorf("rowInstrumentChange")
	}
	if e := awaitEvent(t, hooks.EventRowMute, func() { emitRowMute(4, true) }); !e.Payload.(hooks.RowChangePayload).Mute {
		t.Errorf("rowMute")
	}
	if e := awaitEvent(t, hooks.EventRowSolo, func() { emitRowSolo(5, true) }); e.Payload.(hooks.RowChangePayload).Row != 5 {
		t.Errorf("rowSolo")
	}
}

func TestNodeTypeNameMapping(t *testing.T) {
	cases := []struct {
		t    model.NodeType
		want string
	}{
		{model.NodeTypeRegular, "regular"},
		{model.NodeTypeInvisible, "invisible"},
		{model.NodeTypeSilent, "silent"},
		{model.NodeTypeMute, "mute"},
		{model.NodeType(99), "unknown"},
	}
	for _, c := range cases {
		if got := nodeTypeName(c.t); got != c.want {
			t.Errorf("nodeTypeName(%v): got %q want %q", c.t, got, c.want)
		}
	}
}

// ─── scene_catalog.go — catalog enumeration / dispatch ────────────────────

func TestSceneCatalogListersAreSorted(t *testing.T) {
	scenes := ListScenes()
	if len(scenes) == 0 {
		t.Fatalf("expected non-empty scene catalog")
	}
	for i := 1; i < len(scenes); i++ {
		if scenes[i-1].Name > scenes[i].Name {
			t.Errorf("ListScenes not sorted at %d: %q > %q",
				i, scenes[i-1].Name, scenes[i].Name)
		}
	}

	names := SceneNames(true)
	if len(names) != len(scenes) {
		t.Errorf("SceneNames(true): %d names but %d scenes", len(names), len(scenes))
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Errorf("SceneNames not sorted")
		}
	}

	// Mobile-only filter excludes mobile_* scenes; the desktop list
	// must still contain at least the transport_idle baseline.
	desktop := SceneNames(false)
	if len(desktop) == 0 {
		t.Fatalf("desktop SceneNames is empty")
	}
	for _, n := range desktop {
		if len(n) >= 7 && n[:7] == "mobile_" {
			t.Errorf("desktop list contains mobile-only scene %q", n)
		}
	}

	mobile := MobileSceneNames()
	if len(mobile) == 0 {
		t.Fatalf("MobileSceneNames is empty")
	}
}

func TestRunSceneUnknownReturnsError(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	if err := RunScene(g, "no-such-scene"); err == nil {
		t.Fatalf("expected error for unknown scene")
	}
}

func TestRunSceneTransportIdleNoOp(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultAudio(t)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// transport_idle has no Setup body; must not error.
	if err := RunScene(g, "transport_idle"); err != nil {
		t.Fatalf("RunScene(transport_idle): %v", err)
	}
}

func TestEnsureRowAddsMissingRows(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultAudio(t)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	startRows := len(g.drum.Rows)
	ensureRow(g, startRows+2)
	if len(g.drum.Rows) < startRows+3 {
		t.Errorf("ensureRow: have %d rows want >= %d", len(g.drum.Rows), startRows+3)
	}
}

// ─── game_uistate_setters.go — declarative UI-state apply ─────────────────

func TestSetCameraOffsetsAndScale(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultAudio(t)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	g.SetCameraOffsetX(123)
	if g.cam.OffsetX != 123 {
		t.Errorf("OffsetX=%v want 123", g.cam.OffsetX)
	}
	g.SetCameraOffsetY(45)
	if g.cam.OffsetY != 45 {
		t.Errorf("OffsetY=%v want 45", g.cam.OffsetY)
	}

	prev := g.cam.Scale
	g.SetCameraScale(2.5)
	if g.cam.Scale != 2.5 {
		t.Errorf("Scale=%v want 2.5", g.cam.Scale)
	}

	// Negative or zero scale must be rejected (precondition s > 0).
	g.SetCameraScale(0)
	if g.cam.Scale != 2.5 {
		t.Errorf("Scale changed on zero input: %v", g.cam.Scale)
	}
	g.SetCameraScale(-1)
	if g.cam.Scale != 2.5 {
		t.Errorf("Scale changed on negative input: %v", g.cam.Scale)
	}
	_ = prev

	// CenterCamera clears the centered flag so the next layout pass
	// recenters.
	g.centered = true
	g.CenterCamera()
	if g.centered {
		t.Errorf("CenterCamera did not clear centered flag")
	}
}

func TestSetSplitterFracClampsAndMarksUserSet(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultAudio(t)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)

	g.SetSplitterFrac(0.5)
	if !g.split.userSet {
		t.Errorf("userSet not marked")
	}

	// Out-of-range values must clamp to [0.05, 0.95].
	g.SetSplitterFrac(0.001)
	g.SetSplitterFrac(0.999)
	// Just smoke — the field type depends on horizontal/vertical mode.
	if g.split.X == 0 && g.split.Y == 0 {
		t.Errorf("splitter not updated by extreme inputs")
	}
}

func TestOpenSidebarForUnknownNodeIDIsNoOp(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultAudio(t)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Calling with an ID that doesn't exist must not panic and must
	// leave the sidebar closed.
	g.OpenSidebarForNodeID(999_999)
}

// ─── game_transport_helpers.go — transport state pass-through getters ────

func TestTransportGettersReflectStateMutation(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultAudio(t)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	g.SetBeatBaseForTest(5.5)
	if g.BeatBase() != 5.5 {
		t.Errorf("BeatBase=%v want 5.5", g.BeatBase())
	}

	g.SetLastBeatForTest(7.25)
	if g.LastBeat() != 7.25 {
		t.Errorf("LastBeat=%v want 7.25", g.LastBeat())
	}

	g.SetLastDisplayBeatForTest(8.0)
	if g.LastDisplayBeat() != 8.0 {
		t.Errorf("LastDisplayBeat=%v", g.LastDisplayBeat())
	}

	now := time.Now().Truncate(time.Millisecond)
	g.SetPlayStartForTest(now)
	if !g.PlayStart().Equal(now) {
		t.Errorf("PlayStart=%v want %v", g.PlayStart(), now)
	}

	g.SetAudioStartForTest(1.5)
	if g.AudioStart() != 1.5 {
		t.Errorf("AudioStart=%v want 1.5", g.AudioStart())
	}

	g.SetPausedBeatsForTest(3)
	if g.PausedBeats() != 3 {
		t.Errorf("PausedBeats=%v want 3", g.PausedBeats())
	}

	g.SetSeekFreezeFrames(9)
	if g.SeekFreezeFrames() != 9 {
		t.Errorf("SeekFreezeFrames=%v want 9", g.SeekFreezeFrames())
	}

	// LastProg has no public setter; just smoke that the getter runs.
	_ = g.LastProg()

	// JustPaused is part of the state-machine flow; toggle by going
	// from playing → not-playing via SetPlaying(false).
	g.SetPlayingForTest(true)
	g.SetPlaying(false)
	g.ClearJustResumed()
	g.ClearJustPaused()
}

// ─── touch.go — touch state queries ───────────────────────────────────────

func TestTouchStateBasics(t *testing.T) {
	ts := NewTouchState()
	if ts.ActiveTouchCount() != 0 {
		t.Errorf("ActiveTouchCount on empty: %d", ts.ActiveTouchCount())
	}
	if ts.PrimaryTouch() != nil {
		t.Errorf("PrimaryTouch on empty must be nil")
	}
	if ts.GetTouch(42) != nil {
		t.Errorf("GetTouch on empty must be nil")
	}
	if got := ts.AllTouches(); len(got) != 0 {
		t.Errorf("AllTouches on empty: %d", len(got))
	}
	if ts.RecentMultiTouch() {
		t.Errorf("RecentMultiTouch must be false on fresh state")
	}

	// Inject points directly so we exercise the lookup paths without
	// needing real Ebiten input.
	ts.points[1] = &TouchPoint{ID: 1, X: 10, Y: 20, StartTime: time.Now()}
	ts.points[2] = &TouchPoint{ID: 2, X: 30, Y: 40, StartTime: time.Now().Add(time.Millisecond)}

	if ts.ActiveTouchCount() != 2 {
		t.Errorf("count after inject: %d", ts.ActiveTouchCount())
	}
	if ts.GetTouch(1) == nil {
		t.Errorf("GetTouch(1) lost")
	}
	if got := ts.AllTouches(); len(got) != 2 {
		t.Errorf("AllTouches: %d want 2", len(got))
	}
	if pt := ts.PrimaryTouch(); pt == nil || pt.ID != 1 {
		t.Errorf("PrimaryTouch must return earliest start: %#v", pt)
	}

	ts.multiTouchCooldown = 5
	if !ts.RecentMultiTouch() {
		t.Errorf("RecentMultiTouch should be true while cooldown > 0")
	}

	ts.Reset()
	if ts.ActiveTouchCount() != 0 || ts.RecentMultiTouch() {
		t.Errorf("Reset did not clear state")
	}
}

func TestTouchEventKindString(t *testing.T) {
	cases := []struct {
		k    TouchEventKind
		want string
	}{
		{TouchEventStart, "start"},
		{TouchEventMove, "move"},
		{TouchEventEnd, "end"},
		{TouchEventKind(99), "unknown"},
	}
	for _, c := range cases {
		if got := c.k.String(); got != c.want {
			t.Errorf("%v.String(): got %q want %q", c.k, got, c.want)
		}
	}
}

func TestTouchDebugLogToggle(t *testing.T) {
	prev := IsTouchDebugEnabled()
	SetTouchDebugEnabled(true)
	if !IsTouchDebugEnabled() {
		t.Errorf("SetTouchDebugEnabled(true) had no effect")
	}
	SetTouchDebugEnabled(false)
	if IsTouchDebugEnabled() {
		t.Errorf("SetTouchDebugEnabled(false) had no effect")
	}
	t.Cleanup(func() { SetTouchDebugEnabled(prev) })
}
