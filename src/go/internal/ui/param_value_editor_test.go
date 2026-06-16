package ui

import (
	"image"
	"image/color"
	"strconv"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func TestOpenValuePrefillsAndCommitsViaSpec(t *testing.T) {
	var wrote float64 = -1
	spec := ValueSpec{
		Format: func(v float64) string { return strconv.Itoa(int(v)) },
		Parse: func(s string) (float64, bool) {
			n, err := strconv.Atoi(strings.TrimSpace(s))
			if err != nil || n < 1 || n > 1000 {
				return 0, false
			}
			return float64(n), true
		},
		MinW:   60,
		Style:  BPMBoxStyle,
		MaxLen: 4,
	}
	ed := NewParamValueEditor()
	ed.OpenValue(ValueOpen{
		Spec:   spec,
		Anchor: image.Rect(0, 0, 40, 24),
		Get:    func() float64 { return 120 },
		Set:    func(v float64) { wrote = v },
	})
	if ed.ti.Value() != "120" {
		t.Fatalf("prefill=%q want 120", ed.ti.Value())
	}
	ed.ti.SetText("145")
	ed.commit()
	if wrote != 145 {
		t.Fatalf("wrote=%v want 145", wrote)
	}
	if ed.Active() {
		t.Fatalf("editor should close after commit")
	}
}

func TestSizeEditorRectFitsLongValueAndClamps(t *testing.T) {
	anchor := image.Rect(100, 10, 130, 34) // 30px cramped readout
	r := sizeEditorRect(anchor, "-127.5", 72, image.Rect(0, 0, 400, 200))
	if r.Dx() < 72 {
		t.Fatalf("width %d below MinW 72", r.Dx())
	}
	if r.Dx() < TextWidth("-127.5") {
		t.Fatalf("width %d clips value (TextWidth=%d)", r.Dx(), TextWidth("-127.5"))
	}
	if r.Min.X < 0 || r.Max.X > 400 {
		t.Fatalf("rect %v escapes clamp", r)
	}
}

func TestSizeEditorRectClampNarrowerThanFit(t *testing.T) {
	// Box wants to be wide but clamp is tiny: result must equal clamp width, inside clamp.
	anchor := image.Rect(50, 0, 60, 20)
	clamp := image.Rect(40, 0, 70, 20) // 30px wide
	r := sizeEditorRect(anchor, "a very long value indeed", 200, clamp)
	if r.Min.X < clamp.Min.X || r.Max.X > clamp.Max.X {
		t.Fatalf("rect %v escapes clamp %v", r, clamp)
	}
}

func TestEditorEscapeReverts(t *testing.T) {
	var wrote float64 = -1
	ed := NewParamValueEditor()
	ed.OpenValue(ValueOpen{
		Spec:   paramSpec(audio.ParamDef{Name: "g", Min: -12, Max: 12}),
		Anchor: image.Rect(0, 0, 40, 24),
		Get:    func() float64 { return 3 },
		Set:    func(v float64) { wrote = v },
	})
	ed.ti.SetText("9")
	ed.cancel() // Escape path
	if wrote != -1 {
		t.Fatalf("escape must not write; wrote=%v", wrote)
	}
	if ed.Active() {
		t.Fatalf("escape should close the editor")
	}
}

func TestParseParamEntryContinuousClamps(t *testing.T) {
	def := audio.ParamDef{Name: "filter_cutoff", Min: 20, Max: 20000, Unit: "Hz"}
	if v, ok := parseParamEntry(def, "  8042 "); !ok || v != 8042 {
		t.Fatalf("got v=%v ok=%v", v, ok)
	}
	if v, ok := parseParamEntry(def, "99999"); !ok || v != 20000 {
		t.Fatalf("clamp-high v=%v ok=%v", v, ok)
	}
	if v, ok := parseParamEntry(def, "0"); !ok || v != 20 {
		t.Fatalf("clamp-low v=%v ok=%v", v, ok)
	}
	if _, ok := parseParamEntry(def, "abc"); ok {
		t.Fatalf("expected parse failure")
	}
	if _, ok := parseParamEntry(def, "NaN"); ok {
		t.Fatalf("expected NaN rejection")
	}
	if _, ok := parseParamEntry(def, ""); ok {
		t.Fatalf("empty must fail")
	}
}

func TestParseParamEntryEnumByLabelOrIndex(t *testing.T) {
	def := audio.ParamDef{Name: "osc_type", Min: 0, Max: 3, Enum: []string{"Sine", "Saw", "Square", "Triangle"}}
	if v, ok := parseParamEntry(def, "square"); !ok || v != 2 {
		t.Fatalf("label match v=%v ok=%v", v, ok)
	}
	if v, ok := parseParamEntry(def, "  Triangle "); !ok || v != 3 {
		t.Fatalf("label trims+matches v=%v ok=%v", v, ok)
	}
	if v, ok := parseParamEntry(def, "3"); !ok || v != 3 {
		t.Fatalf("index match v=%v ok=%v", v, ok)
	}
	if _, ok := parseParamEntry(def, "9"); ok {
		t.Fatalf("out-of-range index must fail")
	}
	if _, ok := parseParamEntry(def, "nope"); ok {
		t.Fatalf("unknown label must fail")
	}
}

func TestFormatParamForEntryRoundTrips(t *testing.T) {
	def := audio.ParamDef{Name: "filter_cutoff", Min: 20, Max: 20000, Unit: "Hz"}
	s := formatParamForEntry(def, 8042)
	if v, ok := parseParamEntry(def, s); !ok || v != 8042 {
		t.Fatalf("round-trip s=%q v=%v ok=%v", s, v, ok)
	}
	enumDef := audio.ParamDef{Name: "osc_type", Min: 0, Max: 3, Enum: []string{"Sine", "Saw", "Square", "Triangle"}}
	if got := formatParamForEntry(enumDef, 2); got != "Square" {
		t.Fatalf("enum format got %q", got)
	}
}

func TestParamValueEditorCommitsValidValue(t *testing.T) {
	def := audio.ParamDef{Name: "filter_cutoff", Min: 20, Max: 20000, Unit: "Hz"}
	var wrote float64 = -1
	ed := NewParamValueEditor()
	ed.Open(def, image.Rect(0, 0, 80, 24), func() float64 { return 1000 }, func(v float64) { wrote = v })
	if !ed.Active() {
		t.Fatalf("editor should be active after Open")
	}
	ed.ti.SetText("8042")
	ed.commit()
	if wrote != 8042 {
		t.Fatalf("wrote=%v want 8042", wrote)
	}
	if ed.Active() {
		t.Fatalf("editor should close after commit")
	}
}

func TestParamValueEditorRevertsInvalid(t *testing.T) {
	def := audio.ParamDef{Name: "filter_cutoff", Min: 20, Max: 20000, Unit: "Hz"}
	var wrote float64 = -1
	ed := NewParamValueEditor()
	ed.Open(def, image.Rect(0, 0, 80, 24), func() float64 { return 1000 }, func(v float64) { wrote = v })
	ed.ti.SetText("garbage")
	ed.commit()
	if wrote != -1 {
		t.Fatalf("invalid input must not write; wrote=%v", wrote)
	}
	if ed.errorAnim <= 0 {
		t.Fatalf("invalid input must flag error anim")
	}
	if ed.Active() {
		t.Fatalf("editor should close after a (failed) commit")
	}
}

func TestParamValueEditorOpenPrefillsCurrentValue(t *testing.T) {
	def := audio.ParamDef{Name: "osc_type", Min: 0, Max: 3, Enum: []string{"Sine", "Saw", "Square", "Triangle"}}
	ed := NewParamValueEditor()
	ed.Open(def, image.Rect(0, 0, 80, 24), func() float64 { return 2 }, func(float64) {})
	if ed.ti.Value() != "Square" {
		t.Fatalf("prefill=%q want Square", ed.ti.Value())
	}
}

func TestParamValueEditorFlashRendersAfterInvalid(t *testing.T) {
	def := audio.ParamDef{Name: "filter_cutoff", Min: 20, Max: 20000, Unit: "Hz"}
	ed := NewParamValueEditor()
	ed.Open(def, image.Rect(0, 0, 80, 24), func() float64 { return 1000 }, func(float64) {})
	ed.ti.SetText("garbage")
	ed.commit() // invalid: errorAnim set, editor closed

	// Swap drawRect to count flash draws; restore after.
	var flashDraws int
	orig := drawRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		flashDraws++
	}
	defer func() { drawRect = orig }()

	img := newTrackedImage("test.pve", 100, 40)
	defer releaseImage(img)
	ed.Draw(img) // must draw the flash even though inactive
	if flashDraws == 0 {
		t.Fatalf("error flash did not render after invalid commit")
	}
}

func TestEditorMobileIDSetPerSurface(t *testing.T) {
	ed := NewParamValueEditor()
	ed.OpenValue(ValueOpen{
		Spec:          paramSpec(audio.ParamDef{Name: "g", Min: 0, Max: 1}),
		Anchor:        image.Rect(0, 0, 40, 24),
		Get:           func() float64 { return 0.5 },
		Set:           func(float64) {},
		MobileInputID: "synth-param",
	})
	if ed.ti.MobileInputID != "synth-param" {
		t.Fatalf("mobile id=%q want synth-param", ed.ti.MobileInputID)
	}
	ed.commit()
}

func TestOpenValueClearsPendingErrorFlash(t *testing.T) {
	ed := NewParamValueEditor()
	ed.Open(audio.ParamDef{Name: "g", Min: 0, Max: 10}, image.Rect(0, 0, 40, 24),
		func() float64 { return 1 }, func(float64) {})
	ed.ti.SetText("garbage")
	ed.commit() // invalid -> errorAnim=1, closed
	if ed.errorAnim <= 0 {
		t.Fatalf("precondition: expected errorAnim>0 after invalid commit")
	}
	// Re-open (a different surface in production) must clear the stale flash.
	ed.OpenValue(ValueOpen{
		Spec:   paramSpec(audio.ParamDef{Name: "h", Min: 0, Max: 10}),
		Anchor: image.Rect(0, 0, 40, 24),
		Get:    func() float64 { return 2 },
		Set:    func(float64) {},
	})
	if ed.errorAnim != 0 {
		t.Fatalf("OpenValue must clear errorAnim, got %v", ed.errorAnim)
	}
}

// noKeyInput stubs all input off; helper to drive a single key/char frame.
func driveEditorKey(ed *ParamValueEditor, key ebiten.Key) {
	restore := SetInputForTest(
		func() (int, int) { return -1, -1 },
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == key },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()
	ed.Update()
}

func driveEditorChars(ed *ParamValueEditor, s string) {
	sent := false
	restore := SetInputForTest(
		func() (int, int) { return -1, -1 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune {
			if sent {
				return nil
			}
			sent = true
			return []rune(s)
		},
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()
	ed.Update()
}

func TestParamValueEditorLeftArrowThenInsertAtCaret(t *testing.T) {
	ed := NewParamValueEditor()
	ed.Open(audio.ParamDef{Name: "g", Min: 0, Max: 1000},
		image.Rect(0, 0, 80, 24), func() float64 { return 120 }, func(float64) {})
	if ed.ti.Value() != "120" {
		t.Fatalf("prefill=%q want 120", ed.ti.Value())
	}
	driveEditorKey(ed, ebiten.KeyLeft) // caret 3 -> 2 (between '2' and '0')
	if ed.ti.cursor != 2 {
		t.Fatalf("after Left cursor=%d want 2", ed.ti.cursor)
	}
	driveEditorChars(ed, "5") // insert at caret 2
	if ed.ti.Value() != "1250" {
		t.Fatalf("after Left+insert got %q want 1250", ed.ti.Value())
	}
}

func TestParamValueEditorRightArrowMovesCaret(t *testing.T) {
	ed := NewParamValueEditor()
	ed.Open(audio.ParamDef{Name: "g", Min: 0, Max: 1000},
		image.Rect(0, 0, 80, 24), func() float64 { return 120 }, func(float64) {})
	ed.ti.cursor = 0
	driveEditorKey(ed, ebiten.KeyRight) // 0 -> 1
	if ed.ti.cursor != 1 {
		t.Fatalf("after Right cursor=%d want 1", ed.ti.cursor)
	}
}

func TestParamValueEditorEnterCommitsViaUpdate(t *testing.T) {
	var wrote float64 = -1
	ed := NewParamValueEditor()
	ed.Open(audio.ParamDef{Name: "g", Min: 0, Max: 1000},
		image.Rect(0, 0, 80, 24), func() float64 { return 100 }, func(v float64) { wrote = v })
	ed.ti.SetText("250")
	driveEditorKey(ed, ebiten.KeyEnter)
	if wrote != 250 {
		t.Fatalf("Enter must commit; wrote=%v want 250", wrote)
	}
	if ed.Active() {
		t.Fatalf("editor must close after Enter-commit")
	}
}

func TestParamValueEditorEscapeRevertsViaUpdate(t *testing.T) {
	var wrote float64 = -1
	ed := NewParamValueEditor()
	ed.Open(audio.ParamDef{Name: "g", Min: 0, Max: 1000},
		image.Rect(0, 0, 80, 24), func() float64 { return 100 }, func(v float64) { wrote = v })
	ed.ti.SetText("999")
	driveEditorKey(ed, ebiten.KeyEscape)
	if wrote != -1 {
		t.Fatalf("Escape must NOT write; wrote=%v", wrote)
	}
	if ed.Active() {
		t.Fatalf("editor must close after Escape")
	}
}
