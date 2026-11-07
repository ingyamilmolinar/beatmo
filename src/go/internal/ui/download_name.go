package ui

import "time"

// Download names must never collide: the browser's own uniquifier spells "(1)" differently in every engine and
// the page cannot control it, so the name is stamped here.

// Matches audio.StopRecording's zip name.
const downloadStampLayout = "2006-01-02_150405"

var downloadClock = time.Now

// stampedFileName("beatmo-export", ".json") -> "beatmo-export-<YYYY-MM-DD_HHMMSS>.json"
func stampedFileName(base, ext string) string {
	return base + "-" + downloadClock().Format(downloadStampLayout) + ext
}

func SetDownloadClockForTest(fn func() time.Time) (restore func()) {
	prev := downloadClock
	downloadClock = fn
	return func() { downloadClock = prev }
}
