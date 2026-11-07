//go:build !js

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

func TestRunList_IncludesEveryRef(t *testing.T) {
	c, err := wavref.Catalogue()
	if err != nil {
		t.Fatalf("Catalogue(): %v", err)
	}
	var buf bytes.Buffer
	if err := runList(&buf, ""); err != nil {
		t.Fatalf("runList: %v", err)
	}
	for _, r := range c.Refs {
		if !strings.Contains(buf.String(), r.ID) {
			t.Errorf("list output missing ref %q", r.ID)
		}
	}
}

func TestRunList_TierFilter(t *testing.T) {
	var buf bytes.Buffer
	if err := runList(&buf, wavref.TierSong); err != nil {
		t.Fatalf("runList: %v", err)
	}
	c, _ := wavref.Catalogue()
	for _, r := range c.Refs {
		if r.Tier != wavref.TierInstrument {
			continue
		}
		if strings.Contains(buf.String(), "\t"+r.ID+"\t") {
			t.Errorf("song-tier listing leaked instrument ref %q", r.ID)
		}
	}
}

func TestRunVerify_UnknownIDErrors(t *testing.T) {
	var buf bytes.Buffer
	if err := runVerify(&buf, "no-such-ref"); err == nil {
		t.Error("runVerify accepted an unknown id")
	}
}

func TestLoadCatalogue_SucceedsOnRealCatalogue(t *testing.T) {
	c, err := loadCatalogue()
	if err != nil {
		t.Fatalf("loadCatalogue(): %v", err)
	}
	if len(c.Refs) == 0 {
		t.Error("loadCatalogue() returned no refs")
	}
}

// The song-tier and status-only branches, which the catalogue may not exercise; both take a Ref directly.
func TestRefTargets(t *testing.T) {
	cases := []struct {
		name string
		r    wavref.Ref
		want string
	}{
		{
			name: "song tier returns template",
			r:    wavref.Ref{ID: "x", Tier: wavref.TierSong, Template: "bach-toccata"},
			want: "bach-toccata",
		},
		{
			name: "instrument tier returns joined instruments",
			r:    wavref.Ref{ID: "y", Tier: wavref.TierInstrument, Instruments: []instrument.ID{roster.Violin, roster.Cello}},
			want: "violin,cello",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := refTargets(tc.r); got != tc.want {
				t.Errorf("refTargets(%+v) = %q, want %q", tc.r, got, tc.want)
			}
		})
	}
}

func TestPresence(t *testing.T) {
	cases := []struct {
		name string
		r    wavref.Ref
		want string
	}{
		{
			name: "synthetic status short-circuits to status string",
			r:    wavref.Ref{ID: "z", Status: wavref.StatusSynthetic},
			want: wavref.StatusSynthetic,
		},
		{
			name: "needs-source status short-circuits to status string",
			r:    wavref.Ref{ID: "w", Status: wavref.StatusNeedsSource},
			want: wavref.StatusNeedsSource,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := presence(tc.r); got != tc.want {
				t.Errorf("presence(%+v) = %q, want %q", tc.r, got, tc.want)
			}
		})
	}
}
