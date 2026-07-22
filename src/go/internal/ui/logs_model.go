package ui

import (
	"sort"
	"strings"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/eventlogger"
)

// timestampMode selects the timestamp-filter behavior in the logs panel.
type timestampMode int

const (
	tsModeAt timestampMode = iota
	tsModeRange
)

// logsTimeSel is one selected timestamp-filter moment (minute precision). An
// unset selection means "no filter / open-ended".
type logsTimeSel struct {
	set bool
	t   time.Time
}

// truncateToMinute / endOfMinute bracket the minute a selection names.
func truncateToMinute(t time.Time) time.Time { return t.Truncate(time.Minute) }
func endOfMinute(t time.Time) time.Time {
	return truncateToMinute(t).Add(time.Minute - time.Nanosecond)
}

// matchAtAbs reports whether at falls within sel's minute (absolute, date-aware).
// An unset sel is no filter (true).
func matchAtAbs(at time.Time, sel logsTimeSel) bool {
	if !sel.set {
		return true
	}
	return !at.Before(truncateToMinute(sel.t)) && !at.After(endOfMinute(sel.t))
}

// matchRangeAbs reports whether at ∈ [from-minute-start, to-minute-end]. Either
// bound may be unset (open-ended); both unset = no filter.
func matchRangeAbs(at time.Time, from, to logsTimeSel) bool {
	if from.set && at.Before(truncateToMinute(from.t)) {
		return false
	}
	if to.set && at.After(endOfMinute(to.t)) {
		return false
	}
	return true
}

// logLevel is the severity kind shown/filtered in the logs panel. The filter is
// single-select (mutually exclusive), defaulting to levelError.
type logLevel int

const (
	levelDebug logLevel = iota
	levelInfo
	levelWarn
	levelError
)

// logLevelOrder is the fixed left→right chip order.
var logLevelOrder = [...]logLevel{levelDebug, levelInfo, levelWarn, levelError}

// label returns the uppercase level name shown on the chip and in each row.
func (l logLevel) label() string {
	switch l {
	case levelDebug:
		return "DEBUG"
	case levelWarn:
		return "WARN"
	case levelError:
		return "ERROR"
	default:
		return "INFO"
	}
}

// logRow is one merged/filtered row in the logs panel.
type logRow struct {
	at         time.Time
	level      logLevel
	tag        string
	msg        string
	isErr      bool
	highlights []int // fuzzy match positions in msg (set by name filter)
}

// notifRowInput is the read-time projection of a UI notification into the
// logs-panel merge (kept separate so the model has no UI dependency).
type notifRowInput struct {
	at    time.Time
	text  string
	isErr bool
}

// mergeLogRows combines event records and notifications into one newest-first
// list, tagging each with a severity level. Event narrative is INFO; a
// notification is ERROR when it is an error, else INFO. (WARN/DEBUG have no
// current source but are selectable.)
func mergeLogRows(events []eventlogger.LogRecord, notifs []notifRowInput) []logRow {
	out := make([]logRow, 0, len(events)+len(notifs))
	for _, e := range events {
		out = append(out, logRow{at: e.At, level: levelInfo, tag: e.Tag, msg: e.Msg, isErr: e.IsErr})
	}
	for _, n := range notifs {
		lvl, tag := levelInfo, "info"
		if n.isErr {
			lvl, tag = levelError, "error"
		}
		out = append(out, logRow{at: n.at, level: lvl, tag: tag, msg: n.text, isErr: n.isErr})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].at.After(out[j].at) })
	return out
}

// pageCount returns the number of pages (>=1) for total items at pageSize.
func pageCount(total, pageSize int) int {
	if pageSize < 1 {
		pageSize = 1
	}
	if total <= 0 {
		return 1
	}
	return (total + pageSize - 1) / pageSize
}

// pageSlice returns the 1-based page window, clamping page into [1, pageCount].
func pageSlice[T any](items []T, pageSize, page int) []T {
	if pageSize < 1 {
		pageSize = 1
	}
	pc := pageCount(len(items), pageSize)
	if page < 1 {
		page = 1
	}
	if page > pc {
		page = pc
	}
	start := (page - 1) * pageSize
	if start >= len(items) {
		return nil
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}

// filterRows applies level (single-select), name (fuzzy over tag+msg), and
// timestamp filters. Name matches set logRow.highlights (positions in msg).
func filterRows(rows []logRow, name string, level logLevel, tsMode timestampMode, at, from, to logsTimeSel) []logRow {
	name = strings.TrimSpace(name)
	out := make([]logRow, 0, len(rows))
	for _, r := range rows {
		if r.level != level {
			continue
		}
		switch tsMode {
		case tsModeRange:
			if !matchRangeAbs(r.at, from, to) {
				continue
			}
		default:
			if !matchAtAbs(r.at, at) {
				continue
			}
		}
		if name != "" {
			hay := r.tag + " " + r.msg
			fr := FuzzyMatch(name, hay)
			if fr.Score < 0 {
				continue
			}
			// Shift highlight positions that fall inside msg back to msg-space.
			prefix := len([]rune(r.tag + " "))
			var hl []int
			for _, p := range fr.Positions {
				if p >= prefix {
					hl = append(hl, p-prefix)
				}
			}
			r.highlights = hl
		}
		out = append(out, r)
	}
	return out
}
