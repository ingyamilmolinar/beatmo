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

// RGB only: the swatches are opaque while exported colours carry an explicit FF alpha.
func rgbKey(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("%02X%02X%02X", uint8(r>>8), uint8(g>>8), uint8(b>>8))
}

// THE set any circuit may use: startup demo, template or a new row.
func viceCitySwatchSet() map[string]string {
	set := make(map[string]string, len(SuggestedInstrumentSwatches))
	for _, s := range SuggestedInstrumentSwatches {
		set[rgbKey(s.RGBA)] = s.Name
	}
	return set
}

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

// Not merely on-palette: row N takes genInstrumentSequence[N % len], a predictable finite series.
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

// Both are generated from the same DESIGN.md block; this guards the two outputs from drifting apart.
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

// Past the palette size too: the uniqueness substitution and the exhausted fallback both stay on-palette.
func TestNewInstrumentColorsAreViceCityPalette(t *testing.T) {
	assertDefaultParityState(t)
	palette := viceCitySwatchSet()
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	dv := g.drum

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
