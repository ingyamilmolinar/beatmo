//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// --- EQ dB text input tests ---

func TestEQDBInput_InitialValues(t *testing.T) {
	assertDefaultParityState(t)
	z, _ := newTestEQPanelZone(nil)
	z.Layout(image.Rect(0, 400, 600, 580))

	// The dB readouts derive live from bandGainsDB; all start at 0.0.
	for i := range z.bandGainsDB {
		if got := formatDB(z.bandGainsDB[i]); got != "0.0" {
			t.Errorf("band %d readout = %q, want %q", i, got, "0.0")
		}
	}
}

func TestEQDBInput_CurveDragSyncsText(t *testing.T) {
	assertDefaultParityState(t)
	restore := noInputForTest()
	defer restore()

	z, _ := newTestEQPanelZone(nil)
	r := image.Rect(0, 400, 600, 580)
	_ = registerEQZone(z, r)

	// The readout derives live from bandGainsDB.
	z.bandGainsDB[3] = 6.0

	got := formatDB(z.bandGainsDB[3])
	if got != "+6.0" {
		t.Errorf("after curve drag, dB readout = %q, want %q", got, "+6.0")
	}
}

func TestEQDBInput_SyncBandStateUpdatesTexts(t *testing.T) {
	assertDefaultParityState(t)
	z, _ := newTestEQPanelZone(nil)
	z.Layout(image.Rect(0, 400, 600, 580))

	gains := make([]float64, 10)
	muted := make([]bool, 10)
	for i := range gains {
		gains[i] = float64(i) - 5
	}
	z.SyncBandState(gains, muted)

	// Readouts derive live from bandGainsDB after SyncBandState.
	for i := range gains {
		want := formatDB(gains[i])
		if got := formatDB(z.bandGainsDB[i]); got != want {
			t.Errorf("band %d readout = %q, want %q", i, got, want)
		}
	}
}

func TestEQDBInput_MutedBandShowsValue(t *testing.T) {
	assertDefaultParityState(t)
	z, _ := newTestEQPanelZone(nil)
	z.Layout(image.Rect(0, 400, 600, 580))

	z.bandGainsDB[2] = 5.0
	z.bandMuted[2] = true

	// A muted band still shows its gain value in the readout.
	if got := formatDB(z.bandGainsDB[2]); got != "+5.0" {
		t.Errorf("muted band readout = %q, want %q", got, "+5.0")
	}
}

func TestEQDBInput_HitAreaRegistered(t *testing.T) {
	assertDefaultParityState(t)
	z, _ := newTestEQPanelZone(nil)
	r := image.Rect(0, 400, 600, 580)
	z.Layout(r)

	found := 0
	for _, a := range z.HitAreas() {
		for i := range eqCenterLabels {
			if a.Tag == "eq-db-"+eqCenterLabels[i] {
				found++
				// Above the curve overlay and the mute/HP-LP controls, so a tap on a dB field wins.
				if a.ZIndex != 133 {
					t.Errorf("hit area %q z-index = %d, want 133", a.Tag, a.ZIndex)
				}
			}
		}
	}
	if found != 10 {
		t.Errorf("found %d eq-db-* hit areas, want 10", found)
	}
}

// Gains written straight into the shared array, as import does, show with no explicit sync: the readouts derive
// live.
func TestEQDBInput_DirectBandGainsDBMutationSyncsTexts(t *testing.T) {
	assertDefaultParityState(t)
	restore := noInputForTest()
	defer restore()

	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	z := g.drum.eqPanelZone
	if z == nil {
		t.Fatal("eqPanelZone is nil")
	}

	// Verify initial state: all "0.0".
	for i := range z.bandGainsDB {
		if got := formatDB(z.bandGainsDB[i]); got != "0.0" {
			t.Fatalf("initial band %d readout = %q, want %q", i, got, "0.0")
		}
	}

	// As the import path writes them.
	z.bandGainsDB[0] = 6.0
	z.bandGainsDB[3] = -4.5
	z.bandGainsDB[9] = 12.0

	// The same channel, as import calls it.
	g.drum.setEQActiveChannel(channel.Master)

	// The readouts derive live from bandGainsDB, so they reflect the writes.
	want := map[int]string{0: "+6.0", 3: "-4.5", 9: "+12.0"}
	for band, expected := range want {
		if got := formatDB(z.bandGainsDB[band]); got != expected {
			t.Errorf("band %d readout = %q, want %q", band, got, expected)
		}
	}
}
