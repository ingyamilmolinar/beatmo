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

// BPM 60, one unit per beat: beats are seconds.
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

func TestTimelineInfoBelowHourStaysMinutesSeconds(t *testing.T) {
	dv := makeHourTestDV(t)
	// 65 beats == 65s == 1m 5s → "1:05"
	got := dv.timelineInfo(65)
	if !strings.HasSuffix(got, "· 1:05") {
		t.Fatalf("below-hour format wrong: got %q want suffix %q", got, "· 1:05")
	}
}

// Never a misleading "60:00".
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

// "1:02:03", not "1:2:3".
func TestTimelineInfoAboveHourFormat(t *testing.T) {
	dv := makeHourTestDV(t)
	// 1h 2m 3s = 3723s
	got := dv.timelineInfo(3723)
	if !strings.HasSuffix(got, "· 1:02:03") {
		t.Fatalf("above-hour format wrong: got %q want suffix %q", got, "· 1:02:03")
	}
}

// Catches a curS%3600 truncation.
func TestTimelineInfoMultipleHours(t *testing.T) {
	dv := makeHourTestDV(t)
	// 2h 30m 45s = 9045s
	got := dv.timelineInfo(9045)
	if !strings.HasSuffix(got, "· 2:30:45") {
		t.Fatalf("multi-hour format wrong: got %q want suffix %q", got, "· 2:30:45")
	}
}

// A separate call site that must share the format.
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
