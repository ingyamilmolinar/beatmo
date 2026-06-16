//go:build test

package ui

import (
	"encoding/json"
	"fmt"
	"image/color"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/assets"
	"github.com/ingyamilmolinar/beatmo/internal/templates"
)

// rgbKey returns the canonical "RRGGBB" upper-hex key for a color, ignoring
// alpha. Circuit colors are compared on RGB only because the curated swatches
// are opaque while exported/template colors carry an explicit FF alpha.
func rgbKey(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("%02X%02X%02X", uint8(r>>8), uint8(g>>8), uint8(b>>8))
}

// viceCitySwatchSet maps every curated Vice City swatch's RGB key to its name.
// This is THE set of colors any circuit (startup demo, genre template, or a
// newly added instrument row) is allowed to use — the "always comply with the
// Vice City style" invariant.
func viceCitySwatchSet() map[string]string {
	set := make(map[string]string, len(SuggestedInstrumentSwatches))
	for _, s := range SuggestedInstrumentSwatches {
		set[rgbKey(s.RGBA)] = s.Name
	}
	return set
}

// TestTemplateColorsAreViceCityPalette asserts every instrument color baked
// into the shipped genre templates is a member of the curated Vice City
// palette. Guards the "templates always comply with the Vice City style"
// requirement against drift in internal/templates/layout.go rowColors.
func TestTemplateColorsAreViceCityPalette(t *testing.T) {
	palette := viceCitySwatchSet()
	for _, tp := range assets.Templates() {
		var doc struct {
			Instruments []struct {
				ID    string `json:"id"`
				Color string `json:"color"`
			} `json:"instruments"`
		}
		if err := json.Unmarshal(tp.Bytes, &doc); err != nil {
			t.Fatalf("%s: unmarshal: %v", tp.Genre, err)
		}
		if len(doc.Instruments) == 0 {
			t.Fatalf("%s: no instruments parsed", tp.Genre)
		}
		for _, ins := range doc.Instruments {
			if ins.Color == "" {
				continue // inherits its instrumentDefaults color (already palette)
			}
			key := rgbKey(parseHexColor(ins.Color))
			if name, ok := palette[key]; !ok {
				t.Errorf("template %s: instrument %q color %s is NOT a Vice City swatch", tp.Genre, ins.ID, ins.Color)
			} else {
				t.Logf("template %s: %q -> %s (%s)", tp.Genre, ins.ID, name, ins.Color)
			}
		}
	}
}

// TestTemplateColorsDeriveFromSequence is the stronger sibling of
// TestTemplateColorsAreViceCityPalette: it asserts every shipped genre
// template's instrument colors are not merely on-palette but are derived, in
// row order, from the single canonical instrument SEQUENCE — row N takes
// genInstrumentSequence[N % len]. This pins the user requirement that circuit
// instrument colors "always derive from a predictable sequential finite series".
func TestTemplateColorsDeriveFromSequence(t *testing.T) {
	if len(genInstrumentSequence) == 0 {
		t.Fatal("genInstrumentSequence is empty — DESIGN.md instrumentSequence: not generated")
	}
	for _, tp := range assets.Templates() {
		var doc struct {
			Instruments []struct {
				ID    string `json:"id"`
				Color string `json:"color"`
			} `json:"instruments"`
		}
		if err := json.Unmarshal(tp.Bytes, &doc); err != nil {
			t.Fatalf("%s: unmarshal: %v", tp.Genre, err)
		}
		for i, ins := range doc.Instruments {
			if ins.Color == "" {
				continue // inherits its instrumentDefaults color
			}
			want := genInstrumentSequence[i%len(genInstrumentSequence)]
			if got := rgbKey(parseHexColor(ins.Color)); got != rgbKey(want) {
				t.Errorf("template %s instrument %d (%q): color %s != sequence[%d %% %d]=%s",
					tp.Genre, i, ins.ID, got, i, len(genInstrumentSequence), rgbKey(want))
			}
		}
	}
}

// TestInstrumentSequenceParity asserts the two generated copies of the canonical
// series — ui's genInstrumentSequence ([]color.RGBA) and templates.InstrumentSequence
// ([]string) — are identical and every entry is a Vice City swatch. Both are
// emitted from the same DESIGN.md instrumentSequence: block, so this guards the
// codegen against the two outputs drifting apart.
func TestInstrumentSequenceParity(t *testing.T) {
	palette := viceCitySwatchSet()
	if len(genInstrumentSequence) == 0 {
		t.Fatal("genInstrumentSequence is empty")
	}
	if len(genInstrumentSequence) != len(templates.InstrumentSequence) {
		t.Fatalf("series length mismatch: ui=%d templates=%d", len(genInstrumentSequence), len(templates.InstrumentSequence))
	}
	for i, c := range genInstrumentSequence {
		uiKey := rgbKey(c)
		tplKey := rgbKey(parseHexColor(templates.InstrumentSequence[i]))
		if uiKey != tplKey {
			t.Errorf("series[%d]: ui %s != templates %s", i, uiKey, tplKey)
		}
		if name, ok := palette[uiKey]; !ok {
			t.Errorf("series[%d] %s is NOT a Vice City swatch", i, uiKey)
		} else {
			t.Logf("series[%d] = %s (%s)", i, name, uiKey)
		}
	}
}

// TestStartupDemoColorsAreSequential asserts the embedded startup demo circuit
// (assets.StartupDemoJSON) — the default circuit shown on launch — has
// instrument colors that are on-palette AND derived from the canonical series
// in row order. Pre-fix the demo shipped stale legacy hues (#44A8A8, #C87850…)
// that belonged to no palette; this guards against that ever returning.
func TestStartupDemoColorsAreSequential(t *testing.T) {
	var doc struct {
		Instruments []struct {
			ID    string `json:"id"`
			Color string `json:"color"`
		} `json:"instruments"`
	}
	if err := json.Unmarshal(assets.StartupDemoJSON, &doc); err != nil {
		t.Fatalf("unmarshal startup demo: %v", err)
	}
	if len(doc.Instruments) == 0 {
		t.Fatal("startup demo has no instruments")
	}
	palette := viceCitySwatchSet()
	for i, ins := range doc.Instruments {
		if ins.Color == "" {
			continue
		}
		got := rgbKey(parseHexColor(ins.Color))
		if _, ok := palette[got]; !ok {
			t.Errorf("startup demo instrument %d (%q): color %s is NOT a Vice City swatch", i, ins.ID, ins.Color)
		}
		if want := rgbKey(seriesColorAt(i)); got != want {
			t.Errorf("startup demo instrument %d (%q): color %s != seriesColorAt(%d)=%s", i, ins.ID, got, i, want)
		}
	}
}

// TestNewInstrumentColorsAreViceCityPalette asserts the "creating new
// instruments" path only ever produces Vice City swatch colors, even when the
// row count exceeds the palette size (the uniqueness substitution and the
// exhausted-palette fallback both stay on-palette). Guards the user
// requirement that newly added instruments always comply with the style.
func TestNewInstrumentColorsAreViceCityPalette(t *testing.T) {
	assertDefaultParityState(t)
	palette := viceCitySwatchSet()
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	dv := g.drum

	// Add far more rows than the palette has distinct colors so we exercise
	// the unique-scan AND the exhausted-palette fallback branch.
	for len(dv.Rows) < len(palette)+4 {
		dv.AddRow()
	}
	for i := range dv.Rows {
		dv.Rows[i].Color = dv.ensureUniqueColor(dv.Rows[i].Color, i)
	}
	for i, r := range dv.Rows {
		if _, ok := palette[rgbKey(r.Color)]; !ok {
			t.Errorf("row %d color %s is NOT a Vice City swatch after ensureUniqueColor", i, rgbKey(r.Color))
		}
	}
}

// TestEnsureUniqueColorStaysOnPalette asserts the uniqueness substitution
// never drifts off-palette: when a palette color is already taken, the
// replacement is a DIFFERENT but still-palette swatch.
func TestEnsureUniqueColorStaysOnPalette(t *testing.T) {
	assertDefaultParityState(t)
	palette := viceCitySwatchSet()
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	dv := g.drum
	for len(dv.Rows) < 2 {
		dv.AddRow()
	}
	// Force row 0 and row 1 to the same swatch, then resolve row 1.
	flamingo := SuggestedInstrumentSwatches[0].RGBA
	dv.Rows[0].Color = flamingo
	dv.Rows[1].Color = flamingo
	got := dv.ensureUniqueColor(flamingo, 1)
	if rgbKey(got) == rgbKey(flamingo) {
		t.Fatalf("ensureUniqueColor returned the already-used color %s; expected a distinct swatch", rgbKey(flamingo))
	}
	if _, ok := palette[rgbKey(got)]; !ok {
		t.Fatalf("ensureUniqueColor substitution %s is NOT a Vice City swatch", rgbKey(got))
	}
}
