//go:build test

package ui

import (
	"image"
	"io"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// makeHourTestDV returns a DrumView configured so that elapsedBeats maps
// 1:1 to seconds (BPM=60 ⇒ secPerBeat=1.0, unitsPerBeat=1).
func makeHourTestDV(t *testing.T) *DrumView {
	t.Helper()
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 100, 100), graph, logger)
	dv.SetBPM(60)
	dv.timelineUnitsPerBeat = 1
	dv.Length = 8
	return dv
}

// Under one hour the format must remain M:SS so existing callers/UX are
// unchanged for short sessions.
func TestTimelineInfoBelowHourStaysMinutesSeconds(t *testing.T) {
	dv := makeHourTestDV(t)
	// 65 beats == 65s == 1m 5s → "1:05"
	got := dv.timelineInfo(65)
	if !strings.HasSuffix(got, "· 1:05") {
		t.Fatalf("below-hour format wrong: got %q want suffix %q", got, "· 1:05")
	}
}

// At exactly one hour (3600s) the format must switch to H:MM:SS so the
// readout never shows a misleading "60:00" of minutes.
func TestTimelineInfoAtHourBoundary(t *testing.T) {
	dv := makeHourTestDV(t)
	got := dv.timelineInfo(3600)
	if !strings.HasSuffix(got, "· 1:00:00") {
		t.Fatalf("hour-boundary format wrong: got %q want suffix %q", got, "· 1:00:00")
	}
	if strings.Contains(got, "60:00") {
		t.Fatalf("must not display 60:00 once we reach an hour: %q", got)
	}
}

// Past an hour both minutes and seconds must be zero-padded so the readout
// is monospace-stable: "1:02:03", not "1:2:3".
func TestTimelineInfoAboveHourFormat(t *testing.T) {
	dv := makeHourTestDV(t)
	// 1h 2m 3s = 3723s
	got := dv.timelineInfo(3723)
	if !strings.HasSuffix(got, "· 1:02:03") {
		t.Fatalf("above-hour format wrong: got %q want suffix %q", got, "· 1:02:03")
	}
}

// Multi-hour values must also render correctly — guards against accidental
// modulo-by-hour truncation if someone "fixes" with `curS%3600`.
func TestTimelineInfoMultipleHours(t *testing.T) {
	dv := makeHourTestDV(t)
	// 2h 30m 45s = 9045s
	got := dv.timelineInfo(9045)
	if !strings.HasSuffix(got, "· 2:30:45") {
		t.Fatalf("multi-hour format wrong: got %q want suffix %q", got, "· 2:30:45")
	}
}

// The cached timeline-zone path is a separate call site and must use the
// same hour-aware format. This is the regression that motivated the fix.
func TestTimelineInfoCachedUsesHourFormat(t *testing.T) {
	dv := makeHourTestDV(t)
	dv.timelineZone.LastInfoText = "" // bypass cache
	got := dv.timelineZone.timelineInfoCached(3723)
	if !strings.HasSuffix(got, "· 1:02:03") {
		t.Fatalf("cached format wrong: got %q want suffix %q", got, "· 1:02:03")
	}
}

// Sanity: 59:59 (one second shy of an hour) must still use M:SS form.
func TestTimelineInfoJustUnderHour(t *testing.T) {
	dv := makeHourTestDV(t)
	got := dv.timelineInfo(3599)
	if !strings.HasSuffix(got, "· 59:59") {
		t.Fatalf("just-under-hour format wrong: got %q want suffix %q", got, "· 59:59")
	}
}
