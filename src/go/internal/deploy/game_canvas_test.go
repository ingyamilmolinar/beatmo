package deploy

import (
	"path/filepath"
	"regexp"
	"testing"
)

// firstCanvas matches a lookup of "the first <canvas> on the page".
var firstCanvas = regexp.MustCompile(`querySelector\(\s*['"]canvas['"]\s*\)|getElementsByTagName\(\s*['"]canvas['"]\s*\)`)

// The landing's crystal canvas is static markup and precedes the canvas Ebiten appends, so any script taking "the canvas"
// bound to the crystal and never reached the game. The game canvas has one accessor, window.__beatmoGameCanvas.
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
