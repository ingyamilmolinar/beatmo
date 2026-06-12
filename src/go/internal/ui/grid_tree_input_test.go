//go:build test

package ui

import (
	"image"
	"testing"
)

// recordingHandler returns a configurable result and logs calls.
type recordingHandler struct {
	result  InputResult
	presses *[]string
	tag     string
}

func (h *recordingHandler) OnPress(int, int) InputResult {
	*h.presses = append(*h.presses, h.tag)
	return h.result
}
func (h *recordingHandler) OnDrag(int, int)                   {}
func (h *recordingHandler) OnRelease(int, int)                {}
func (h *recordingHandler) OnWheel(int, int, int) InputResult { return InputIgnored }

func TestGridTreeDispatchHighestZFirst(t *testing.T) {
	tr := NewGridTree()
	var presses []string
	tr.HitIndexRef().Update("low", []HitArea{{
		Rect: image.Rect(0, 0, 100, 100), ZIndex: GZCanvas,
		Handler: &recordingHandler{result: InputIgnored, presses: &presses, tag: "canvas"},
	}})
	tr.HitIndexRef().Update("high", []HitArea{{
		Rect: image.Rect(0, 0, 100, 100), ZIndex: GZSidebar,
		Handler: &recordingHandler{result: InputConsumed, presses: &presses, tag: "sidebar"},
	}})

	tr.dispatchPressForTest(20, 20)

	if len(presses) != 1 || presses[0] != "sidebar" {
		t.Fatalf("expected only 'sidebar' to receive press, got %v", presses)
	}
	if !tr.inputHandled || !tr.suppress {
		t.Fatalf("consumed press must set inputHandled+suppress")
	}
}

func TestGridTreeIgnoredFallsThrough(t *testing.T) {
	tr := NewGridTree()
	var presses []string
	tr.HitIndexRef().Update("low", []HitArea{{
		Rect: image.Rect(0, 0, 100, 100), ZIndex: GZCanvas,
		Handler: &recordingHandler{result: InputConsumed, presses: &presses, tag: "canvas"},
	}})
	tr.HitIndexRef().Update("high", []HitArea{{
		Rect: image.Rect(0, 0, 100, 100), ZIndex: GZSidebar,
		Handler: &recordingHandler{result: InputIgnored, presses: &presses, tag: "sidebar"},
	}})

	tr.dispatchPressForTest(20, 20)

	if len(presses) != 2 || presses[0] != "sidebar" || presses[1] != "canvas" {
		t.Fatalf("expected sidebar then canvas, got %v", presses)
	}
}
