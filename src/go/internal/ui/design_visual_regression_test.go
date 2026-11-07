package ui

import (
	"image/color"
	"testing"
)

// Every surface once outside the token system reads a generated gen* symbol at its call site, so a DESIGN.md
// edit reaches it. A coverage check: screenshots can match while tokenisation is broken.
func TestDesignVisualRegression(t *testing.T) {
	t.Run("grid_lattice_uses_generated_tokens", func(t *testing.T) {
		// Each grid tier aliases a genColor*, not an inline literal.
		cases := []struct {
			name   string
			alias  color.RGBA
			source color.RGBA
		}{
			{"colGridLine", colGridLine, genColorGridLine},
			{"colGridHalf", colGridHalf, genColorGridHalf},
			{"colGridQuarter", colGridQuarter, genColorGridQuarter},
			{"colGridEighth", colGridEighth, genColorGridEighth},
			{"colGridSixteenth", colGridSixteenth, genColorGridSixteenth},
			{"colGridThirtySecond", colGridThirtySecond, genColorGridThirtySecond},
		}
		for _, tc := range cases {
			if tc.alias != tc.source {
				t.Errorf("%s = %v but expected alias of %v", tc.name, tc.alias, tc.source)
			}
		}
	})

	t.Run("node_edge_use_generated_tokens", func(t *testing.T) {
		nodeFill, ok := NodeUI.Fill.(color.RGBA)
		if !ok {
			t.Fatalf("NodeUI.Fill is not color.RGBA: %T", NodeUI.Fill)
		}
		if nodeFill != genColorNodeFill {
			t.Errorf("NodeUI.Fill = %v but expected genColorNodeFill = %v", nodeFill, genColorNodeFill)
		}
		nodeBorder, ok := NodeUI.Border.(color.RGBA)
		if !ok {
			t.Fatalf("NodeUI.Border is not color.RGBA: %T", NodeUI.Border)
		}
		if nodeBorder != genColorNodeBorder {
			t.Errorf("NodeUI.Border = %v but expected genColorNodeBorder = %v", nodeBorder, genColorNodeBorder)
		}
		edge, ok := EdgeUI.Color.(color.NRGBA)
		if !ok {
			t.Fatalf("EdgeUI.Color is not color.NRGBA: %T", EdgeUI.Color)
		}
		if edge.R != genColorEdgeColor.R || edge.G != genColorEdgeColor.G || edge.B != genColorEdgeColor.B {
			t.Errorf("EdgeUI.Color RGB = (%d,%d,%d) but expected genColorEdgeColor (%d,%d,%d)",
				edge.R, edge.G, edge.B, genColorEdgeColor.R, genColorEdgeColor.G, genColorEdgeColor.B)
		}
		if edge.A != genAlphaEdgeDefault {
			t.Errorf("EdgeUI.Color alpha = %d but expected genAlphaEdgeDefault = %d", edge.A, genAlphaEdgeDefault)
		}
	})

	t.Run("instrument_palette_routed_through_gen", func(t *testing.T) {
		// instColors must be a 1:1 lift of genInstrumentDefaults.
		if len(instColors) != len(genInstrumentDefaults) {
			t.Errorf("instColors len=%d but genInstrumentDefaults len=%d", len(instColors), len(genInstrumentDefaults))
		}
		for id, want := range genInstrumentDefaults {
			got, ok := instColors[id].(color.RGBA)
			if !ok {
				t.Errorf("instColors[%q] not color.RGBA: %T", id, instColors[id])
				continue
			}
			if got != want {
				t.Errorf("instColors[%q] = %v but genInstrumentDefaults[%q] = %v", id, got, id, want)
			}
		}
		// customPalette must be a 1:1 lift of genInstrumentFallbackPalette.
		if len(customPalette) != len(genInstrumentFallbackPalette) {
			t.Errorf("customPalette len=%d but genInstrumentFallbackPalette len=%d",
				len(customPalette), len(genInstrumentFallbackPalette))
		}
		for i, want := range genInstrumentFallbackPalette {
			if i >= len(customPalette) {
				break
			}
			got, ok := customPalette[i].(color.RGBA)
			if !ok {
				t.Errorf("customPalette[%d] not color.RGBA: %T", i, customPalette[i])
				continue
			}
			if got != want {
				t.Errorf("customPalette[%d] = %v but fallback[%d] = %v", i, got, i, want)
			}
		}
	})

	t.Run("animation_helpers_consume_gen", func(t *testing.T) {
		// Decay helper round-trips the generated rate/threshold.
		v, alive := DecayStep(1.0, genAnimHighlightDecay)
		if !alive {
			t.Errorf("DecayStep(1.0, genAnimHighlightDecay) returned not-alive at first tick")
		}
		want := 1.0 * genAnimHighlightDecay.Rate
		if v != want {
			t.Errorf("DecayStep(1.0) = %v, want %v (= 1.0 * %v)", v, want, genAnimHighlightDecay.Rate)
		}
		// Threshold honored.
		_, aliveLow := DecayStep(genAnimHighlightDecay.Threshold/2, genAnimHighlightDecay)
		if aliveLow {
			t.Errorf("DecayStep below threshold returned alive=true")
		}
		// sin(0) = 0, so frame 0 is Base.
		if got := SinPulse(0, genAnimPlayheadPulse); got != genAnimPlayheadPulse.Base {
			t.Errorf("SinPulse(0) = %v, want Base = %v", got, genAnimPlayheadPulse.Base)
		}
	})

	t.Run("geometry_consumed_at_call_sites", func(t *testing.T) {
		// step × genGeomEdgeArrowStepFraction.
		g := &Grid{Step: 100}
		if got, want := g.EdgeArrowSize(), 100*genGeomEdgeArrowStepFraction; got != want {
			t.Errorf("Grid.EdgeArrowSize() = %v, want %v (= 100 * %v)", got, want, genGeomEdgeArrowStepFraction)
		}
		// So geometry.node-radius / signal-radius retune the node and pulse.
		if got, want := NodeUI.Radius, float32(genGeomNodeRadius); got != want {
			t.Errorf("NodeUI.Radius = %v, want %v (= float32(genGeomNodeRadius))", got, want)
		}
		if got, want := SignalUI.Radius, float32(genGeomSignalRadius); got != want {
			t.Errorf("SignalUI.Radius = %v, want %v (= float32(genGeomSignalRadius))", got, want)
		}
	})

	t.Run("theme_residue_routed_through_gen", func(t *testing.T) {
		// Once inline literals in theme.go, now through gen* tokens.
		if colMuteActiveBdr != genColorMuteActiveBorder {
			t.Errorf("colMuteActiveBdr = %v but expected genColorMuteActiveBorder = %v", colMuteActiveBdr, genColorMuteActiveBorder)
		}
		if colRecordIdle != genColorRecordIdle {
			t.Errorf("colRecordIdle = %v but expected genColorRecordIdle = %v", colRecordIdle, genColorRecordIdle)
		}
		if colRecordActive != genColorRecordActive {
			t.Errorf("colRecordActive = %v but expected genColorRecordActive = %v", colRecordActive, genColorRecordActive)
		}
		// Through their named alpha buckets.
		if colBeatGroupAlt.A != genAlphaBeatGroupAlt {
			t.Errorf("colBeatGroupAlt.A = %d but expected genAlphaBeatGroupAlt = %d", colBeatGroupAlt.A, genAlphaBeatGroupAlt)
		}
		if colTransportDivider.A != genAlphaBorderPanel {
			t.Errorf("colTransportDivider.A = %d but expected genAlphaBorderPanel = %d", colTransportDivider.A, genAlphaBorderPanel)
		}
		if colTransportGroupBG.A != genAlphaRowRackZebra {
			t.Errorf("colTransportGroupBG.A = %d but expected genAlphaRowRackZebra = %d", colTransportGroupBG.A, genAlphaRowRackZebra)
		}
	})
}
