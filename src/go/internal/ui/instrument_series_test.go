//go:build test

package ui

import "testing"

// TestRowColorsAreSequentialByIndex asserts that newly added rows take their
// color from the canonical instrument series strictly by ROW INDEX — row N gets
// seriesColorAt(N) == genInstrumentSequence[N % len] — independent of which
// instrument the row carries. This is the "pure sequential by row index"
// derivation the demo circuit and new rows must follow. Wrap is exercised by
// adding more rows than the series has colors.
func TestRowColorsAreSequentialByIndex(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	dv := g.drum

	for len(dv.Rows) < len(genInstrumentSequence)+3 {
		dv.AddRow()
	}
	for i, r := range dv.Rows {
		want := seriesColorAt(i)
		if rgbKey(r.Color) != rgbKey(want) {
			t.Errorf("row %d color %s != seriesColorAt(%d)=%s", i, rgbKey(r.Color), i, rgbKey(want))
		}
	}
}

// TestSetInstrumentKeepsIndexColor asserts that changing a row's instrument does
// NOT change its color: under pure-sequential derivation the color is bound to
// the row index, not the instrument identity.
func TestSetInstrumentKeepsIndexColor(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	dv := g.drum
	for len(dv.Rows) < 2 {
		dv.AddRow()
	}
	row := 1
	before := dv.Rows[row].Color
	dv.selRow = row
	opts := dv.instOptions
	if len(opts) == 0 {
		t.Skip("no instrument options available")
	}
	dv.SetInstrument(opts[len(opts)-1])
	if rgbKey(dv.Rows[row].Color) != rgbKey(before) {
		t.Errorf("row %d color changed on SetInstrument: %s -> %s", row, rgbKey(before), rgbKey(dv.Rows[row].Color))
	}
	if rgbKey(dv.Rows[row].Color) != rgbKey(seriesColorAt(row)) {
		t.Errorf("row %d color %s != seriesColorAt(%d)=%s", row, rgbKey(dv.Rows[row].Color), row, rgbKey(seriesColorAt(row)))
	}
}

// TestResequenceRowColors asserts ResequenceRowColors re-derives every row's
// color from the series by index, overwriting whatever colors were present
// (e.g. off-palette colors imported from a baked circuit JSON).
func TestResequenceRowColors(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	dv := g.drum
	for len(dv.Rows) < 4 {
		dv.AddRow()
	}
	// Scramble colors to an off-palette value.
	for i := range dv.Rows {
		dv.Rows[i].Color = parseHexColor("#123456FF")
	}
	dv.ResequenceRowColors()
	for i, r := range dv.Rows {
		want := seriesColorAt(i)
		if rgbKey(r.Color) != rgbKey(want) {
			t.Errorf("row %d color %s != seriesColorAt(%d)=%s after resequence", i, rgbKey(r.Color), i, rgbKey(want))
		}
	}
}
