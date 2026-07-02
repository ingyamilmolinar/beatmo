package ui

import (
	"image"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// TestPopupCloseNeverOpenedNoEvent asserts that Close on an ID that was never
// opened emits no popup.closed event (fixes phantom close log lines produced
// by hover-transition callers like the chain-tooltip handler).
func TestPopupCloseNeverOpenedNoEvent(t *testing.T) {
	ch := make(chan string, 4)
	u := hooks.GlobalBus().Subscribe(hooks.EventPopupClosed, func(e hooks.Event) {
		if p, ok := e.Payload.(hooks.PopupPayload); ok {
			ch <- p.ID
		}
	})
	defer u()

	portal := NewOverlayPortal(&HitIndex{})
	portal.Close("never-opened")

	select {
	case id := <-ch:
		t.Fatalf("Close of never-opened id emitted popup.closed for %q", id)
	case <-time.After(100 * time.Millisecond):
		// correct — no event
	}
}

// TestPopupReOpenEmitsReplacedThenOpened asserts that opening an overlay whose
// ID is already on the stack emits popup.closed(reason="replaced") before the
// new popup.opened — keeping open/close counts balanced on re-open.
func TestPopupReOpenEmitsReplacedThenOpened(t *testing.T) {
	opened := make(chan string, 4)
	type closedEv struct{ id, reason string }
	closedCh := make(chan closedEv, 4)

	uo := hooks.GlobalBus().Subscribe(hooks.EventPopupOpened, func(e hooks.Event) {
		if p, ok := e.Payload.(hooks.PopupPayload); ok {
			opened <- p.ID
		}
	})
	defer uo()
	uc := hooks.GlobalBus().Subscribe(hooks.EventPopupClosed, func(e hooks.Event) {
		if p, ok := e.Payload.(hooks.PopupPayload); ok {
			closedCh <- closedEv{p.ID, p.Reason}
		}
	})
	defer uc()

	portal := NewOverlayPortal(&HitIndex{})
	ov := &nopPortalOverlay{}

	// First open — produces one popup.opened.
	portal.Open(PortalEntry{ID: "X", Overlay: ov})
	select {
	case id := <-opened:
		if id != "X" {
			t.Fatalf("first open: got opened %q", id)
		}
	case <-time.After(time.Second):
		t.Fatal("first open: no popup.opened")
	}

	// Second open of same ID — must emit popup.closed("replaced") then popup.opened.
	portal.Open(PortalEntry{ID: "X", Overlay: ov})
	select {
	case ev := <-closedCh:
		if ev.id != "X" || ev.reason != "replaced" {
			t.Fatalf("re-open: got closed {%q, %q}, want {X, replaced}", ev.id, ev.reason)
		}
	case <-time.After(time.Second):
		t.Fatal("re-open: no popup.closed(replaced)")
	}
	select {
	case id := <-opened:
		if id != "X" {
			t.Fatalf("re-open: got opened %q, want X", id)
		}
	case <-time.After(time.Second):
		t.Fatal("re-open: no popup.opened after replacement")
	}
}

// nopPortalOverlay is a minimal PortalOverlay stub used by popup_log tests.
type nopPortalOverlay struct{}

func (o *nopPortalOverlay) Layout(_, _ image.Rectangle) {}
func (o *nopPortalOverlay) HitAreas() []HitArea         { return nil }
func (o *nopPortalOverlay) Draw(_ *ebiten.Image)        {}
func (o *nopPortalOverlay) ShouldClose() bool           { return false }

func TestPopupOpenCloseEmit(t *testing.T) {
	opened := make(chan string, 2)
	closed := make(chan string, 2)
	uo := hooks.GlobalBus().Subscribe(hooks.EventPopupOpened, func(e hooks.Event) {
		if p, ok := e.Payload.(hooks.PopupPayload); ok {
			opened <- p.ID
		}
	})
	defer uo()
	uc := hooks.GlobalBus().Subscribe(hooks.EventPopupClosed, func(e hooks.Event) {
		if p, ok := e.Payload.(hooks.PopupPayload); ok {
			closed <- p.ID
		}
	})
	defer uc()

	emitPopupOpened("inst-menu")
	emitPopupClosed("inst-menu", "explicit")

	select {
	case id := <-opened:
		if id != "inst-menu" {
			t.Fatalf("opened id %q", id)
		}
	case <-time.After(time.Second):
		t.Fatal("no open event")
	}
	select {
	case id := <-closed:
		if id != "inst-menu" {
			t.Fatalf("closed id %q", id)
		}
	case <-time.After(time.Second):
		t.Fatal("no close event")
	}
}
