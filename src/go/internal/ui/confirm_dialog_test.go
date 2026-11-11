package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

func newTestConfirm(t *testing.T) (*ConfirmDialog, *int, *int) {
	t.Helper()
	confirms, cancels := 0, 0
	d := newConfirmDialog(confirmConfig{
		id:          "test-confirm",
		titleKey:    i18n.KeyConfirmReplaceNode,
		confirmKey:  i18n.KeyCapOverwrite,
		destructive: true,
		onConfirm:   func() { confirms++ },
		onCancel:    func() { cancels++ },
	})
	d.Layout(image.Rectangle{}, image.Rect(0, 0, 1280, 720))
	return d, &confirms, &cancels
}

func TestConfirmDialog_ConfirmFires(t *testing.T) {
	d, confirms, cancels := newTestConfirm(t)
	d.confirm()
	if *confirms != 1 || *cancels != 0 {
		t.Fatalf("confirm(): confirms=%d cancels=%d, want 1/0", *confirms, *cancels)
	}
	if !d.ShouldClose() {
		t.Fatal("confirm() must mark the dialog closed")
	}
}

func TestConfirmDialog_CancelFires(t *testing.T) {
	d, confirms, cancels := newTestConfirm(t)
	d.cancel()
	if *confirms != 0 || *cancels != 1 {
		t.Fatalf("cancel(): confirms=%d cancels=%d, want 0/1", *confirms, *cancels)
	}
	if !d.ShouldClose() {
		t.Fatal("cancel() must mark the dialog closed")
	}
}

func TestConfirmDialog_EnterConfirmsEscCancels(t *testing.T) {
	d, confirms, cancels := newTestConfirm(t)
	if !d.HandleEnter() {
		t.Fatal("HandleEnter must consume the key")
	}
	if *confirms != 1 {
		t.Fatalf("HandleEnter → confirm: got %d", *confirms)
	}
	d2, _, cancels2 := newTestConfirm(t)
	if d2.HandleEscape() {
		t.Fatal("HandleEscape must return false so CloseTop removes the entry")
	}
	if *cancels2 != 1 {
		t.Fatalf("HandleEscape → cancel: got %d", *cancels2)
	}
	_ = cancels
}

func TestConfirmDialog_Idempotent(t *testing.T) {
	// confirm() then cancel(): only onConfirm fires, exactly once.
	d, confirms, cancels := newTestConfirm(t)
	d.confirm()
	d.cancel()  // simulates the portal-cleanup OnClose after a self-close
	d.confirm() // repeated calls are no-ops
	if *confirms != 1 || *cancels != 0 {
		t.Fatalf("idempotency: confirms=%d cancels=%d, want 1/0", *confirms, *cancels)
	}
	// Fresh dialog: cancel() then confirm(): only onCancel fires, once.
	d2, confirms2, cancels2 := newTestConfirm(t)
	d2.cancel()
	d2.confirm()
	if *confirms2 != 0 || *cancels2 != 1 {
		t.Fatalf("idempotency: confirms=%d cancels=%d, want 0/1", *confirms2, *cancels2)
	}
}

func TestConfirmDialog_PublishesButtonHitAreas(t *testing.T) {
	d, _, _ := newTestConfirm(t)
	areas := d.HitAreas()
	// confirm + cancel + close + panel catch-all == 4 areas; no full-screen backdrop.
	if len(areas) != 4 {
		t.Fatalf("HitAreas len = %d, want 4 (confirm/cancel/close/panel)", len(areas))
	}
	for _, a := range areas {
		if a.Rect.Dx() >= 1280 && a.Rect.Dy() >= 720 {
			t.Fatal("no full-screen backdrop allowed — it would swallow outside taps")
		}
	}
}
