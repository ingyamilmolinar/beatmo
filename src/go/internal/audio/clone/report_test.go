package clone

import (
	"strings"
	"testing"
)

func TestMarkdownCarriesTheModeTable(t *testing.T) {
	x := modalSignal(barModes(), 48000, 0.05, 2, 0.002, 51)
	p, err := Prepare(x, 48000, PrepareOptions{Label: "bar", F0Hz: 440})
	if err != nil {
		t.Fatal(err)
	}
	a, err := Analyze(p)
	if err != nil {
		t.Fatal(err)
	}
	md := a.Markdown()
	for _, want := range []string{"# Clone analysis — bar", "| Hz |", "440.0", "Decay law", "Strike", "Residual", "Timings"} {
		if !strings.Contains(md, want) {
			t.Errorf("report lacks %q", want)
		}
	}
}
