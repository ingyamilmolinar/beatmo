package ui

import "time"

// logsNowFn returns "now"; overridable in tests for a deterministic calendar.
var logsNowFn = func() time.Time { return time.Now() }

// daysInMonth returns the number of days in (year, m).
func daysInMonth(year int, m time.Month) int {
	// Day 0 of next month == last day of this month.
	return time.Date(year, m+1, 0, 0, 0, 0, 0, time.Local).Day()
}

// firstCellDate is the Sunday on/before the 1st of (year, m), at local midnight
// — grid cell 0 of a Sunday-first month view.
func firstCellDate(year int, m time.Month) time.Time {
	first := time.Date(year, m, 1, 0, 0, 0, 0, time.Local)
	return first.AddDate(0, 0, -int(first.Weekday())) // Sunday == 0
}

// gridDays returns the 42 Sunday-first calendar cells covering (year, m).
func gridDays(year int, m time.Month) [42]time.Time {
	start := firstCellDate(year, m)
	var g [42]time.Time
	for i := 0; i < 42; i++ {
		g[i] = start.AddDate(0, 0, i)
	}
	return g
}
