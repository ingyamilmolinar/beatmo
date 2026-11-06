package ui

import "time"

// Download filenames must be unique per save.
//
// `a.download` (and a desktop save dialog's default) is only a *suggestion*:
// when the suggested name already exists in the Downloads folder, the browser's
// own collision resolver renames the file, and every engine spells the
// uniquifier differently — Chrome writes "beatmo-export (1).json", Firefox
// "beatmo-export(1).json", Safari "beatmo-export 2.json". The page has no say
// in that format, so the only way to land the same name in every browser is to
// never collide. Stamp the name here instead, the way recording downloads
// always have (beatmo-recording-<stamp>.zip).

// downloadStampLayout is the timestamp layout shared by every generated
// download name. Matches audio.StopRecording's zip name.
const downloadStampLayout = "2006-01-02_150405"

// downloadClock is the wall clock used to stamp download names. Overridable in
// tests via SetDownloadClockForTest.
var downloadClock = time.Now

// stampedFileName joins a slug, the current timestamp and an extension into a
// collision-free download name:
//
//	stampedFileName("beatmo-export", ".json") -> "beatmo-export-2026-09-13_141530.json"
func stampedFileName(base, ext string) string {
	return base + "-" + downloadClock().Format(downloadStampLayout) + ext
}

// SetDownloadClockForTest overrides the clock behind every generated download
// filename and returns a restore func.
func SetDownloadClockForTest(fn func() time.Time) (restore func()) {
	prev := downloadClock
	downloadClock = fn
	return func() { downloadClock = prev }
}
