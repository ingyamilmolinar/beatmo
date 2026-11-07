//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// The pair was too narrow to discover on a phone.
func TestLenButtons_HiddenOnMobile(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	if r := g.drum.lenIncBtn.Rect(); !r.Empty() {
		t.Fatalf("lenIncBtn should be empty on mobile, got %v", r)
	}
	if r := g.drum.lenDecBtn.Rect(); !r.Empty() {
		t.Fatalf("lenDecBtn should be empty on mobile, got %v", r)
	}
}

func TestLenButtons_VisibleOnDesktop(t *testing.T) {
	setupMobileTest(t, false)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)

	if r := g.drum.lenIncBtn.Rect(); r.Empty() {
		t.Fatalf("lenIncBtn should be drawn on desktop, got empty rect")
	}
	if r := g.drum.lenDecBtn.Rect(); r.Empty() {
		t.Fatalf("lenDecBtn should be drawn on desktop, got empty rect")
	}
}

// The inline pairs are the only surfaces; an overflow entry would duplicate them.
func TestLenButtons_OverflowAbsent(t *testing.T) {
	for _, mobile := range []bool{true, false} {
		mobile := mobile
		name := "desktop"
		if mobile {
			name = "mobile"
		}
		t.Run(name, func(t *testing.T) {
			setupMobileTest(t, mobile)
			logger := log.New(testLogOutput(), log.LevelInfo)
			g := New(logger)
			t.Cleanup(g.CloseForTest)
			if mobile {
				g.Layout(390, 844)
			} else {
				g.Layout(1280, 720)
			}

			for _, it := range g.drum.overflowItems() {
				if it.label == "Window length +" || it.label == "Window length −" {
					t.Fatalf("overflow should not contain window-length controls; got %q", it.label)
				}
			}
		})
	}
}
