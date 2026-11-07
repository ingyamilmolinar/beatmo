//go:build test

package ui

import "testing"

// Row N takes seriesColorAt(N) whatever instrument it carries; adding more rows than colours exercises the wrap.
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

// The colour is bound to the row index, not the instrument.
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
