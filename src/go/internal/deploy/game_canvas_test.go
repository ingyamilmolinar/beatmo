package deploy

import (
	"path/filepath"
	"regexp"
	"testing"
)

// firstCanvas matches a lookup of "the first <canvas> on the page".
var firstCanvas = regexp.MustCompile(`querySelector\(\s*['"]canvas['"]\s*\)|getElementsByTagName\(\s*['"]canvas['"]\s*\)`)

// TestPageScriptsNeverTakeTheFirstCanvas: the landing's crystal canvas is in
// index.html's markup, so it precedes the canvas Ebiten appends at boot. Every
// page script that wanted "the canvas" found the crystal instead: the phone
// soft-keyboard focus rects and the native <input> touch listeners bound to it
// (removed on hand-over, display:none otherwise) and never reached the game.
// The game canvas has one accessor, window.__beatmoGameCanvas (index.html's
// first head script).
func TestPageScriptsNeverTakeTheFirstCanvas(t *testing.T) {
	root := repoRoot(t)
	for _, name := range []string{"index.html", "landing.js", "audio.js"} {
		src := mustRead(t, filepath.Join(root, "src", "js", name))
		for _, loc := range firstCanvas.FindAllStringIndex(src, -1) {
			t.Errorf("%s:%d takes the page's first <canvas> (%q); use window.__beatmoGameCanvas()",
				name, lineOf(src, loc[0]), src[loc[0]:loc[1]])
		}
	}
}

func lineOf(s string, off int) int {
	n := 1
	for _, c := range s[:off] {
		if c == '\n' {
			n++
		}
	}
	return n
}
