package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/eventlogger"
)

func logTm(h, m, s int) time.Time { return time.Date(2026, 7, 19, h, m, s, 0, time.Local) }

func TestMergeLogRowsNewestFirst(t *testing.T) {
	ev := []eventlogger.LogRecord{{At: logTm(12, 0, 1), Tag: "node", Msg: "add node"}}
	nt := []notifRowInput{{at: logTm(12, 0, 5), text: "Imported", isErr: false}, {at: logTm(12, 0, 3), text: "boom", isErr: true}}
	got := mergeLogRows(ev, nt)
	if len(got) != 3 {
		t.Fatalf("len=%d want 3", len(got))
	}
	if !got[0].at.Equal(logTm(12, 0, 5)) || got[2].tag != "node" {
		t.Fatalf("not newest-first: %+v", got)
	}
	if got[1].tag != "error" || !got[1].isErr {
		t.Fatalf("error notif tag/flag wrong: %+v", got[1])
	}
}

func TestMergeLogRowsAssignsLevels(t *testing.T) {
	ev := []eventlogger.LogRecord{{At: logTm(12, 0, 1), Tag: "node", Msg: "add node"}}
	nt := []notifRowInput{{at: logTm(12, 0, 5), text: "ok", isErr: false}, {at: logTm(12, 0, 3), text: "boom", isErr: true}}
	got := mergeLogRows(ev, nt)
	byMsg := map[string]logLevel{}
	for _, r := range got {
		byMsg[r.msg] = r.level
	}
	if byMsg["add node"] != levelInfo {
		t.Fatalf("event level = %v want INFO", byMsg["add node"])
	}
	if byMsg["ok"] != levelInfo {
		t.Fatalf("info notif level = %v want INFO", byMsg["ok"])
	}
	if byMsg["boom"] != levelError {
		t.Fatalf("error notif level = %v want ERROR", byMsg["boom"])
	}
}

func TestFilterRowsByLevel(t *testing.T) {
	rows := []logRow{
		{level: levelInfo, msg: "a", at: logTm(1, 0, 0)},
		{level: levelError, msg: "b", at: logTm(1, 0, 1)},
	}
	got := filterRows(rows, "", levelError, tsModeAt, logsTimeSel{}, logsTimeSel{}, logsTimeSel{})
	if len(got) != 1 || got[0].msg != "b" {
		t.Fatalf("level filter failed: %+v", got)
	}
}

func TestFilterRowsHighlightsAreMsgRelative(t *testing.T) {
	// Positions index into msg, not the "tag " prefix.
	rows := []logRow{{level: levelInfo, tag: "row", msg: "kick vol", at: logTm(1, 0, 0)}}
	got := filterRows(rows, "kick", levelInfo, tsModeAt, logsTimeSel{}, logsTimeSel{}, logsTimeSel{})
	if len(got) != 1 {
		t.Fatalf("expected 1 match, got %d", len(got))
	}
	// "kick" is at msg indices 0..3 → highlights must be within [0,len(msg)).
	msgLen := len([]rune("kick vol"))
	if len(got[0].highlights) == 0 {
		t.Fatal("expected non-empty highlights for an in-msg match")
	}
	for _, h := range got[0].highlights {
		if h < 0 || h >= msgLen {
			t.Fatalf("highlight index %d out of msg range [0,%d)", h, msgLen)
		}
	}
	if got[0].highlights[0] != 0 {
		t.Fatalf("first highlight should be msg index 0 ('k' of kick), got %d", got[0].highlights[0])
	}
}

func TestMatchAtAbs(t *testing.T) {
	base := time.Date(2026, 7, 19, 14, 30, 20, 0, time.Local)
	if !matchAtAbs(base, logsTimeSel{}) {
		t.Fatal("unset = no filter (match)")
	}
	sel := logsTimeSel{set: true, t: time.Date(2026, 7, 19, 14, 30, 0, 0, time.Local)}
	if !matchAtAbs(base, sel) {
		t.Fatal("same minute should match")
	}
	if matchAtAbs(base.Add(2*time.Minute), sel) {
		t.Fatal("different minute must not match")
	}
	if matchAtAbs(base.AddDate(0, 0, -1), sel) {
		t.Fatal("same clock time, different DAY must not match (date-aware)")
	}
}

func TestMatchRangeAbs(t *testing.T) {
	d := func(h, m int) time.Time { return time.Date(2026, 7, 19, h, m, 0, 0, time.Local) }
	from := logsTimeSel{set: true, t: d(12, 0)}
	to := logsTimeSel{set: true, t: d(13, 0)}
	if !matchRangeAbs(d(12, 30), from, to) {
		t.Fatal("in-range")
	}
	if matchRangeAbs(d(14, 0), from, to) {
		t.Fatal("out-of-range")
	}
	if !matchRangeAbs(d(13, 0).Add(59*time.Second), from, to) {
		t.Fatal("a moment in the `to` minute is inclusive")
	}
	if !matchRangeAbs(d(12, 30), from, logsTimeSel{}) {
		t.Fatal("open-ended to (since): a moment after `from` matches")
	}
	if matchRangeAbs(d(9, 0), from, logsTimeSel{}) {
		t.Fatal("open-ended to (since): a moment before `from` must not match")
	}
	if !matchRangeAbs(d(9, 0), logsTimeSel{}, to) {
		t.Fatal("open-ended from (until)")
	}
	if !matchRangeAbs(d(9, 9), logsTimeSel{}, logsTimeSel{}) {
		t.Fatal("both unset = no filter")
	}
}

func TestPagination(t *testing.T) {
	if pageCount(0, 10) != 1 {
		t.Fatal("empty → 1 page")
	}
	if pageCount(25, 10) != 3 {
		t.Fatal("25/10 → 3 pages")
	}
	items := []int{1, 2, 3, 4, 5}
	if got := pageSlice(items, 2, 2); len(got) != 2 || got[0] != 3 {
		t.Fatalf("page 2 of size 2 = %v want [3 4]", got)
	}
	if got := pageSlice(items, 2, 99); len(got) != 1 || got[0] != 5 {
		t.Fatalf("clamp high page to last: got %v", got)
	}
	if got := pageSlice(items, 2, 0); got[0] != 1 {
		t.Fatalf("clamp low page to 1: got %v", got)
	}
}

func TestFilterRowsByNameIncludesTag(t *testing.T) {
	rows := []logRow{{level: levelInfo, tag: "node", msg: "add node"}, {level: levelInfo, tag: "play", msg: "play started"}}
	got := filterRows(rows, "node", levelInfo, tsModeAt, logsTimeSel{}, logsTimeSel{}, logsTimeSel{})
	if len(got) != 1 || got[0].tag != "node" {
		t.Fatalf("name search over tag failed: %+v", got)
	}
}
