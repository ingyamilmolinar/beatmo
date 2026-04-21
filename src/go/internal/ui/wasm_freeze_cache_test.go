//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	scope "github.com/ingyamilmolinar/beatmo/internal/scope"
)

func TestScopePanelZone_FreezeCacheDefaultNil(t *testing.T) {
	z := NewScopePanelZone(ScopeCallbacks{})
	if z.frozenState != nil {
		t.Fatal("freshly constructed zone should have frozenState=nil")
	}
	if z.instrumentID != "" {
		t.Fatalf("freshly constructed zone should have empty instrumentID, got %q", z.instrumentID)
	}
}

func TestScopePanelZone_FreezeCacheHoldsState(t *testing.T) {
	z := NewScopePanelZone(ScopeCallbacks{})
	want := &scope.State{TapA: scope.TapData{Active: true, Samples: []float64{0.1, 0.2}}}
	z.frozenState = want
	if z.frozenState != want {
		t.Fatal("frozenState should be settable and retrievable in-place")
	}
	z.frozenState = nil
	if z.frozenState != nil {
		t.Fatal("frozenState should be clearable")
	}
}

func TestScopePanelZone_InstrumentIDSettable(t *testing.T) {
	z := NewScopePanelZone(ScopeCallbacks{})
	z.instrumentID = "snare"
	if z.instrumentID != "snare" {
		t.Fatalf("expected instrumentID=snare, got %q", z.instrumentID)
	}
}

func TestEQPanelZone_FreezeCacheDefaultNil(t *testing.T) {
	z := NewEQPanelZone(EQCallbacks{})
	if z.frozenAnalyzer != nil {
		t.Fatal("freshly constructed EQ zone should have frozenAnalyzer=nil")
	}
}

func TestEQPanelZone_FreezeCacheHoldsState(t *testing.T) {
	z := NewEQPanelZone(EQCallbacks{})
	want := &analyzer.State{Master: analyzer.ChannelMetrics{ID: "main", Active: true}}
	z.frozenAnalyzer = want
	if z.frozenAnalyzer != want {
		t.Fatal("frozenAnalyzer should be settable and retrievable in-place")
	}
	z.frozenAnalyzer = nil
	if z.frozenAnalyzer != nil {
		t.Fatal("frozenAnalyzer should be clearable")
	}
}
