package ui

// synth_view_plot.go builds the GEOMETRY of a Synth-tab graph view — lines,
// filled bands, ticks, markers, labels and the two readouts — from analysed
// renders (specs docs/superpowers/specs/2026-10-24-synth-graphs-and-devtools-design.md §4.4,
// 2026-10-25-synth-dial-relevance-and-cheap-graphs-design.md §3.3).
//
// Pure: no Ebiten, no draw calls. synth_view_draw.go paints a viewPlot; tests
// and the honesty guard (TestEveryControlMovesItsGraph) measure it, so "the
// graph moved" is a statement about the same geometry the user sees.
//
// Rules shared by every view: TRUE SCALE (no per-render gain — the played
// voice is drawn at its played level), ONE axis for NOW and every ghost, and
// the axes come from the renders on screen, not from the control's value, so
// a turned control moves the curve, never the frame around it. Every view
// says what it shows twice: in WORDS (the default, no digit or unit) and in
// NUMBERS (the Details toggle) — and only NUMBERS mode labels the level and
// frequency axes with values.

import (
	"fmt"
	"image"
	"math"
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	"github.com/ingyamilmolinar/beatmo/internal/soundview"
)

type plotRole int

const (
	roleNow plotRole = iota
	roleLo
	roleHi
	roleBefore
)

type fpt struct{ x, y float32 }

// plotLine is a polyline (local coordinates). A line with gaps is several
// plotLines.
type plotLine struct {
	role plotRole
	pts  []fpt
}

// plotBand is a filled area between top and bot at xs (one value per x).
type plotBand struct {
	role     plotRole
	xs       []float32
	top, bot []float32
}

type plotTick struct {
	pos   float32 // x (xTicks) or y (yTicks), local
	label string
	minor bool // a beat mark: a short tick, no label, no grid line
}

type plotMarker struct {
	x     float32
	role  plotRole
	label string
}

// plotLabel is a ghost's name, at its final top-left (placeLabels).
type plotLabel struct {
	x, y float32
	text string
	role plotRole
}

// viewPlot is one view's complete geometry in a w×h local frame.
type viewPlot struct {
	view     synthView
	w, h     int
	plot     image.Rectangle
	title    string
	pickable bool // developer builds: the title is the view picker
	caption  string
	readout  string // NUMBERS: the literal measurements
	summary  string // WORDS: what they mean, for anyone
	numbers  bool   // which readout is shown (the Details toggle)
	chip     string // e.g. "with Sampler edit"
	empty    string // shown instead of a plot when NOW has not landed
	xTicks   []plotTick
	yTicks   []plotTick
	xWords   [2]string // left, right ends
	yWords   [2]string // bottom, top ends
	bands    []plotBand
	diff     []plotBand
	lines    []plotLine
	markers  []plotMarker
	labels   []plotLabel
}

// shownReadout is the readout line the graph draws.
func (vp *viewPlot) shownReadout() string {
	if vp.numbers {
		return vp.readout
	}
	return vp.summary
}

// synthViewInput is what a view is built from.
type synthViewInput struct {
	now, lo, hi, before *graphRender
	loLabel, hiLabel    string
	caption             string
	beatSec             float64
	samplerEdit         bool
	numbers             bool // the Details toggle: numbers instead of words
}

// ghosts returns the present ghost renders with their roles.
func (in synthViewInput) ghosts() []struct {
	r    *graphRender
	role plotRole
} {
	var out []struct {
		r    *graphRender
		role plotRole
	}
	for _, g := range []struct {
		r    *graphRender
		role plotRole
	}{{in.lo, roleLo}, {in.hi, roleHi}, {in.before, roleBefore}} {
		if g.r != nil && g.r.an != nil && (in.now == nil || !sameSound(g.r, in.now)) {
			out = append(out, g)
		}
	}
	return out
}

func (in synthViewInput) all() []*graphRender {
	out := []*graphRender{}
	if in.now != nil && in.now.an != nil {
		out = append(out, in.now)
	}
	for _, g := range in.ghosts() {
		out = append(out, g.r)
	}
	return out
}

const (
	viewPad     = 6
	plotFloorDB = -60.0
)

// sameSound reports whether two renders are the same sound: equal content
// keys (audio.GraphRenderKey) when they carry one, else the same request.
func sameSound(a, b *graphRender) bool {
	if a.key != 0 || b.key != 0 {
		return a.key == b.key
	}
	return a.ident == b.ident
}

// buildSynthViewPlot builds view v for a w×h frame.
func buildSynthViewPlot(v synthView, w, h int, in synthViewInput) *viewPlot {
	vp := &viewPlot{view: v, w: w, h: h, title: synthViewTitle(v), caption: in.caption, numbers: in.numbers}
	if in.samplerEdit {
		vp.chip = i18n.T(i18n.KeyGraphSamplerEdit)
	}
	rowH := StyledTextHeight(RoleMicro) + 2
	top := viewPad + 2*rowH
	left := viewPad + 30
	bottom := h - viewPad - rowH
	vp.plot = image.Rect(left, top, w-viewPad-2, bottom)
	if in.now == nil || in.now.an == nil {
		vp.empty = i18n.T(i18n.KeyGraphNoPicture)
		return vp
	}
	if vp.plot.Dx() < 24 || vp.plot.Dy() < 16 {
		return vp
	}
	switch v {
	case viewLoudness:
		buildLoudness(vp, in)
	case viewAttack:
		buildAttack(vp, in)
	case viewPitch:
		buildPitch(vp, in)
	case viewColour:
		buildColour(vp, in)
	case viewHarmonics:
		if _, ok := in.now.an.Harmonics(); ok {
			buildHarmonics(vp, in)
		} else {
			vp.view, vp.title = viewColour, synthViewTitle(viewColour)
			buildColour(vp, in)
		}
	case viewColourTime:
		buildColourTime(vp, in)
	case viewNoise:
		buildNoise(vp, in)
	case viewWave:
		if in.now.an.Pitched() {
			buildWave(vp, in)
		} else {
			// A close-up of noise shows nothing readable: its colour does.
			vp.view, vp.title = viewColour, synthViewTitle(viewColour)
			buildColour(vp, in)
		}
	default:
		buildOverview(vp, in)
	}
	vp.placeLabels()
	return vp
}

// --- shared axes ---

// viewTimeSpan is the time axis every render on screen fits: the longest
// audible length (+15 %), rounded up to a nice value.
func viewTimeSpan(in synthViewInput, minSec float64) float64 {
	span := minSec
	for _, r := range in.all() {
		span = math.Max(span, r.an.AudibleSec()*1.15)
	}
	if in.now != nil {
		span = math.Min(span, math.Max(in.now.an.DurationSec(), minSec))
	}
	return niceCeil(span)
}

func niceCeil(x float64) float64 {
	if x <= 0 {
		return 1
	}
	e := math.Pow(10, math.Floor(math.Log10(x)))
	for _, m := range []float64{1, 1.5, 2, 2.5, 3, 4, 5, 6, 8, 10} {
		if m*e >= x-1e-12 {
			return m * e
		}
	}
	return 10 * e
}

// viewTickStep picks a tick step so span shows about n ticks.
func viewTickStep(span float64, n int) float64 {
	raw := span / float64(n)
	e := math.Pow(10, math.Floor(math.Log10(raw)))
	for _, m := range []float64{1, 2, 2.5, 5, 10} {
		if m*e >= raw {
			return m * e
		}
	}
	return 10 * e
}

func (vp *viewPlot) xAt(frac float64) float32 {
	return float32(vp.plot.Min.X) + float32(math.Max(0, math.Min(1, frac)))*float32(vp.plot.Dx())
}

func (vp *viewPlot) yAt(frac float64) float32 { // frac 0 = bottom, 1 = top
	return float32(vp.plot.Max.Y) - float32(math.Max(0, math.Min(1, frac)))*float32(vp.plot.Dy())
}

// timeTicks labels the time axis — time reads plainly in both modes.
func (vp *viewPlot) timeTicks(span, beatSec float64) {
	step := viewTickStep(span, 4)
	for t := 0.0; t <= span+1e-9; t += step {
		vp.xTicks = append(vp.xTicks, plotTick{pos: vp.xAt(t / span), label: fmtDur(t)})
	}
	if beatSec > 0 && span/beatSec <= 32 {
		for b := beatSec; b < span; b += beatSec {
			vp.xTicks = append(vp.xTicks, plotTick{pos: vp.xAt(b / span), minor: true})
		}
	}
}

// dbTicks draws the level grid; its values are labelled in NUMBERS mode only
// (WORDS mode says quiet / loud at the axis ends).
func (vp *viewPlot) dbTicks() {
	for _, db := range []float64{0, -20, -40} {
		lbl := ""
		if vp.numbers {
			lbl = fmt.Sprintf("%.0f", db)
		}
		vp.yTicks = append(vp.yTicks, plotTick{pos: vp.yAt(1 - db/plotFloorDB), label: lbl})
	}
}

// hzLabel is a frequency tick's label: its value in NUMBERS mode only.
func (vp *viewPlot) hzLabel(f float64) string {
	if !vp.numbers {
		return ""
	}
	return fmtHzTick(f)
}

func dbFrac(db float64) float64 { return 1 - math.Max(plotFloorDB, math.Min(0, db))/plotFloorDB }

// addSeries adds role's curve; NOW also gets a band down to the baseline.
func (vp *viewPlot) addSeries(role plotRole, xs, ys []float32, baseline float32) {
	if len(xs) < 2 {
		return
	}
	pts := make([]fpt, len(xs))
	for i := range xs {
		pts[i] = fpt{xs[i], ys[i]}
	}
	vp.lines = append(vp.lines, plotLine{role: role, pts: pts})
	if role == roleNow {
		bot := make([]float32, len(xs))
		for i := range bot {
			bot[i] = baseline
		}
		vp.bands = append(vp.bands, plotBand{role: role, xs: xs, top: ys, bot: bot})
	}
}

// --- ghost labels ---

// addGhostLabel names a ghost where it stands farthest from the NOW curve
// (where the eye looks for the difference); placeLabels moves it off anything
// it would cover.
func (vp *viewPlot) addGhostLabel(role plotRole, text string) {
	if text == "" {
		return
	}
	var now []fpt
	for _, l := range vp.lines {
		if l.role == roleNow && len(l.pts) > len(now) {
			now = l.pts
		}
	}
	best, bestD := fpt{}, float32(-1)
	found := false
	for _, l := range vp.lines {
		if l.role != role {
			continue
		}
		for i, p := range l.pts {
			if i%2 != 0 && i != len(l.pts)-1 {
				continue
			}
			d := float32(0)
			if y, ok := interpY(now, p.x); ok {
				d = absf(p.y - y)
			}
			// Prefer the middle of the plot over its edges on ties.
			mid := float32(vp.plot.Min.X+vp.plot.Max.X) / 2
			d -= absf(p.x-mid) / float32(vp.plot.Dx()) * 2
			if !found || d > bestD {
				best, bestD, found = p, d, true
			}
		}
	}
	if !found {
		return
	}
	vp.labels = append(vp.labels, plotLabel{x: best.x, y: best.y, text: text, role: role})
}

func (vp *viewPlot) ghostLabels(in synthViewInput) {
	vp.addGhostLabel(roleLo, in.loLabel)
	vp.addGhostLabel(roleHi, in.hiLabel)
	vp.addGhostLabel(roleBefore, i18n.T(i18n.KeyGraphBefore))
}

// textRect is a piece of text the plot draws inside itself.
type textRect struct {
	r    image.Rectangle
	text string
}

// axisWordRects are the axis-end words, where drawViewPlot draws them.
func (vp *viewPlot) axisWordRects() []textRect {
	mh := StyledTextHeight(RoleMicro)
	var out []textRect
	add := func(s string, x, y int, right bool) {
		if s == "" {
			return
		}
		w := StyledTextWidth(s, RoleMicro)
		if right {
			x -= w
		}
		out = append(out, textRect{image.Rect(x, y, x+w, y+mh), s})
	}
	p := vp.plot
	add(vp.xWords[0], p.Min.X+3, p.Max.Y-mh-1, false)
	add(vp.xWords[1], p.Max.X-3, p.Max.Y-mh-1, true)
	add(vp.yWords[0], p.Min.X+3, p.Max.Y-mh-1, false)
	add(vp.yWords[1], p.Min.X+3, p.Min.Y+1, false)
	for _, m := range vp.markers {
		if m.label == "" {
			continue
		}
		w := StyledTextWidth(m.label, RoleMicro)
		x := int(m.x) + 3
		if x+w > p.Max.X {
			x = int(m.x) - 3 - w
		}
		out = append(out, textRect{image.Rect(x, p.Min.Y+1, x+w, p.Min.Y+1+mh), m.label})
	}
	return out
}

// placeLabels gives every ghost label a final spot inside the plot that
// covers no other label and no axis word: its preferred spot (left of and
// above the point it names), else the nearest free spot above or below, else
// the same on the point's right. A label with no free spot is dropped — an
// unreadable pile of words is worse than a missing name.
func (vp *viewPlot) placeLabels() {
	if len(vp.labels) == 0 {
		return
	}
	mh := StyledTextHeight(RoleMicro)
	taken := vp.axisWordRects()
	free := func(r image.Rectangle) bool {
		if !r.In(vp.plot) {
			return false
		}
		for _, t := range taken {
			if r.Overlaps(t.r.Inset(-1)) {
				return false
			}
		}
		return true
	}
	clampX := func(x, w int) int { return min(max(x, vp.plot.Min.X+2), vp.plot.Max.X-w-2) }
	clampY := func(y int) int { return min(max(y, vp.plot.Min.Y+1), vp.plot.Max.Y-mh-1) }
	var placed []plotLabel
	for _, l := range vp.labels {
		w := StyledTextWidth(l.text, RoleMicro)
		ok := false
		for _, x0 := range []int{int(l.x) - w - 2, int(l.x) + 2} {
			x0 = clampX(x0, w)
			for k := 0; k <= 8 && !ok; k++ {
				for _, sign := range []int{-1, 1} {
					y := clampY(int(l.y) - mh - 1 + sign*k*(mh+1))
					r := image.Rect(x0, y, x0+w, y+mh)
					if free(r) {
						taken = append(taken, textRect{r, l.text})
						placed = append(placed, plotLabel{x: float32(x0), y: float32(y), text: l.text, role: l.role})
						ok = true
						break
					}
				}
			}
			if ok {
				break
			}
		}
	}
	vp.labels = placed
}

// labelRects is every piece of text drawn inside the plot (axis words, marker
// labels, ghost labels) — what TestGraphLabelsNeverOverlap checks.
func (vp *viewPlot) labelRects() []textRect {
	out := vp.axisWordRects()
	mh := StyledTextHeight(RoleMicro)
	for _, l := range vp.labels {
		w := StyledTextWidth(l.text, RoleMicro)
		x, y := int(l.x), int(l.y)
		out = append(out, textRect{image.Rect(x, y, x+w, y+mh), l.text})
	}
	return out
}

// diffShade shades where the NOW and BEFORE curves (functions of x) differ by
// more than 2 px — the automatic "here is what changed" highlight.
func (vp *viewPlot) diffShade() {
	var now, before *plotLine
	for i := range vp.lines {
		switch vp.lines[i].role {
		case roleNow:
			if now == nil {
				now = &vp.lines[i]
			}
		case roleBefore:
			if before == nil {
				before = &vp.lines[i]
			}
		}
	}
	if now == nil || before == nil {
		return
	}
	var band plotBand
	flush := func() {
		if len(band.xs) >= 2 {
			vp.diff = append(vp.diff, band)
		}
		band = plotBand{role: roleBefore}
	}
	band.role = roleBefore
	for x := float32(vp.plot.Min.X); x <= float32(vp.plot.Max.X); x += 2 {
		a, okA := interpY(now.pts, x)
		b, okB := interpY(before.pts, x)
		if !okA || !okB || absf(a-b) <= 2 {
			flush()
			continue
		}
		band.xs = append(band.xs, x)
		band.top = append(band.top, minf(a, b))
		band.bot = append(band.bot, maxf(a, b))
	}
	flush()
}

func interpY(pts []fpt, x float32) (float32, bool) {
	if len(pts) < 2 || x < pts[0].x || x > pts[len(pts)-1].x {
		return 0, false
	}
	i := sort.Search(len(pts), func(i int) bool { return pts[i].x >= x })
	if i == 0 {
		return pts[0].y, true
	}
	a, b := pts[i-1], pts[i]
	if b.x == a.x {
		return b.y, true
	}
	f := (x - a.x) / (b.x - a.x)
	return a.y + f*(b.y-a.y), true
}

func absf(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
func minf(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}
func maxf(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

// setReadouts sets both readouts: numbers (+ "(was key)" when the BEFORE's key
// differs) and words (+ cmp's change phrase against the BEFORE).
func (vp *viewPlot) setReadouts(in synthViewInput, numbers string, key func(*graphRender) string, words string, cmp func(before, now *graphRender) string) {
	vp.readout, vp.summary = numbers, words
	b, n := in.before, in.now
	if b == nil || b.an == nil || n == nil || sameSound(b, n) {
		return
	}
	if key != nil {
		if k := key(b); k != "" && k != key(n) {
			vp.readout = numbers + " " + i18n.Tf(i18n.KeyGraphWas, k)
		}
	}
	if cmp != nil {
		vp.summary = joinWords(words, cmp(b, n))
	}
}

// --- views ---

func buildLoudness(vp *viewPlot, in synthViewInput) {
	span := viewTimeSpan(in, 0.1)
	vp.timeTicks(span, in.beatSec)
	vp.dbTicks()
	vp.yWords = [2]string{i18n.T(i18n.KeyGraphQuiet), i18n.T(i18n.KeyGraphLoud)}
	series := func(r *graphRender) ([]float32, []float32) {
		env := r.an.Envelope()
		var xs, ys []float32
		for i, db := range env.RMSdB {
			t := (float64(i) + 0.5) * env.HopSec
			if t > span {
				break
			}
			xs = append(xs, vp.xAt(t/span))
			ys = append(ys, vp.yAt(dbFrac(db)))
		}
		return xs, ys
	}
	for _, g := range in.ghosts() {
		xs, ys := series(g.r)
		vp.addSeries(g.role, xs, ys, 0)
	}
	xs, ys := series(in.now)
	vp.addSeries(roleNow, xs, ys, float32(vp.plot.Max.Y))
	if vs := in.now.voiceSec; vs > 0 && vs < span {
		vp.markers = append(vp.markers, plotMarker{x: vp.xAt(vs / span), role: roleNow, label: i18n.T(i18n.KeyGraphNoteEnds)})
	}
	vp.diffShade()
	vp.ghostLabels(in)
	aud := func(r *graphRender) string { return fmtDur(r.an.AudibleSec()) }
	vp.setReadouts(in, i18n.Tf(i18n.KeyGraphSoundsFor, aud(in.now)), aud,
		lengthWords(in.now.an, in.now.voiceSec), func(b, n *graphRender) string {
			return changeWords(b.an.AudibleSec(), n.an.AudibleSec(), 0.1, 0.005, i18n.KeyGraphWordsLonger, i18n.KeyGraphWordsShorter)
		})
}

// buildAttack is THE FIRST MOMENT: the note's loudness from its first onset at
// 1 ms (soundview.OnsetCurve), so a fade-in rises, a snap jumps and a click
// stands over the body — read off one curve, true scale.
func buildAttack(vp *viewPlot, in synthViewInput) {
	win := 0.05
	for _, r := range in.all() {
		win = math.Max(win, math.Min(0.3, 3*r.an.AttackSec()))
		if on := r.an.Onsets(); len(on) > 1 {
			win = math.Max(win, math.Min(0.3, on[len(on)-1]-on[0]+0.03))
		}
	}
	win = math.Min(niceCeil(win), 0.3)
	vp.timeTicks(win, 0)
	vp.dbTicks()
	vp.yWords = [2]string{i18n.T(i18n.KeyGraphQuiet), i18n.T(i18n.KeyGraphLoud)}
	series := func(r *graphRender) ([]float32, []float32) {
		c := r.an.OnsetCurve()
		var xs, ys []float32
		for k, db := range c.DB {
			t := float64(k) * c.StepSec
			if t > win {
				break
			}
			xs = append(xs, vp.xAt(t/win))
			ys = append(ys, vp.yAt(dbFrac(db)))
		}
		return xs, ys
	}
	for _, g := range in.ghosts() {
		xs, ys := series(g.r)
		vp.addSeries(g.role, xs, ys, 0)
	}
	xs, ys := series(in.now)
	vp.addSeries(roleNow, xs, ys, float32(vp.plot.Max.Y))
	n := in.now.an
	if on := n.Onsets(); len(on) > 1 {
		for _, t := range on[1:] {
			if t-on[0] < win {
				vp.markers = append(vp.markers, plotMarker{x: vp.xAt((t - on[0]) / win), role: roleNow})
			}
		}
	}
	vp.diffShade()
	vp.ghostLabels(in)
	atk := func(r *graphRender) string { return fmtDur(r.an.AttackSec()) }
	numbers := i18n.Tf(i18n.KeyGraphFullAfter, atk(in.now))
	if c := n.ClickDB(); c >= 3 {
		numbers = i18n.Tf(i18n.KeyGraphFullAfterClick, atk(in.now), int(math.Round(c)))
	}
	vp.setReadouts(in, numbers, atk, attackWords(n), func(b, now *graphRender) string {
		ba, na := b.an.AttackSec(), now.an.AttackSec()
		if w := changeWords(ba, na, 0.2, 0.002, i18n.KeyGraphWordsSofter, i18n.KeyGraphWordsSnappier); w != "" {
			return w
		}
		return changeWords(b.an.ClickDB(), now.an.ClickDB(), 0, 3, i18n.KeyGraphWordsSnappier, i18n.KeyGraphWordsSofter)
	})
}

func zipPts(xs, ys []float32) []fpt {
	pts := make([]fpt, len(xs))
	for i := range xs {
		pts[i] = fpt{xs[i], ys[i]}
	}
	return pts
}

func buildPitch(vp *viewPlot, in synthViewInput) {
	span := viewTimeSpan(in, 0.1)
	vp.timeTicks(span, in.beatSec)
	vp.yWords = [2]string{i18n.T(i18n.KeyGraphLow), i18n.T(i18n.KeyGraphHigh)}
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, r := range in.all() {
		tr := r.an.Pitch()
		for i, hz := range tr.Hz {
			if t := tr.T0 + float64(i)*tr.HopSec; hz > 0 && t <= span {
				m := hzToMidiF(hz)
				lo, hi = math.Min(lo, m), math.Max(hi, m)
			}
		}
	}
	if math.IsInf(lo, 1) {
		vp.readout = i18n.T(i18n.KeyGraphNoPitch)
		vp.summary = vp.readout
		return
	}
	if hi-lo < 6 {
		c := (hi + lo) / 2
		lo, hi = c-3, c+3
	}
	lo, hi = lo-1, hi+1
	yOf := func(m float64) float32 { return vp.yAt((m - lo) / (hi - lo)) }
	step := 1.0
	for _, s := range []float64{1, 2, 3, 6, 12, 24} {
		if (hi-lo)/s <= 4 {
			step = s
			break
		}
		step = s
	}
	for m := math.Ceil(lo/step) * step; m <= hi; m += step {
		lbl := ""
		if vp.numbers {
			lbl = midiNoteName(int(math.Round(m)))
		}
		vp.yTicks = append(vp.yTicks, plotTick{pos: yOf(m), label: lbl})
	}
	segments := func(r *graphRender, role plotRole) {
		tr := r.an.Pitch()
		var cur []fpt
		flush := func() {
			if len(cur) >= 2 {
				vp.lines = append(vp.lines, plotLine{role: role, pts: cur})
			}
			cur = nil
		}
		for i, hz := range tr.Hz {
			t := tr.T0 + float64(i)*tr.HopSec
			if t > span {
				break
			}
			if hz <= 0 {
				flush()
				continue
			}
			cur = append(cur, fpt{vp.xAt(t / span), yOf(hzToMidiF(hz))})
		}
		flush()
	}
	for _, g := range in.ghosts() {
		segments(g.r, g.role)
	}
	segments(in.now, roleNow)
	vp.diffShade()
	vp.ghostLabels(in)
	m := pitchMotionOf(in.now.an)
	vp.setReadouts(in, m.numbers(), func(r *graphRender) string { return pitchMotionOf(r.an).key() },
		pitchWords(m), func(b, n *graphRender) string {
			bm, nm := pitchMotionOf(b.an), pitchMotionOf(n.an)
			switch {
			case bm.kind == pitchGlide || nm.kind == pitchGlide:
				return changeWords(bm.dropCents(), nm.dropCents(), 0, 100, i18n.KeyGraphWordsBiggerDrop, i18n.KeyGraphWordsSmallerDrop)
			case bm.kind == pitchWobble || nm.kind == pitchWobble:
				return changeWords(bm.wobbleCents(), nm.wobbleCents(), 0.25, 5, i18n.KeyGraphWordsMoreWobble, i18n.KeyGraphWordsLessWobble)
			}
			return ""
		})
}

func hzToMidiF(hz float64) float64 { return 69 + 12*math.Log2(hz/440) }

func fmtHzNote(hz float64) string {
	return fmt.Sprintf("%s (%s)", fmtHz(hz), midiNoteName(int(math.Round(hzToMidiF(hz)))))
}

type pitchKind int

const (
	pitchNone pitchKind = iota
	pitchGlide
	pitchWobble
	pitchHold
)

// pitchMotion is what the pitch does over the audible note.
type pitchMotion struct {
	kind                 pitchKind
	startCents, endCents float64 // glide: medians at the start and the end
	settleSec            float64 // glide: until it stays within 50 cents of the end
	medCents             float64 // hold / wobble: the median
	depthCents, rateHz   float64 // wobble
}

func (m pitchMotion) dropCents() float64 {
	if m.kind != pitchGlide {
		return 0
	}
	return m.startCents - m.endCents
}

func (m pitchMotion) wobbleCents() float64 {
	if m.kind != pitchWobble {
		return 0
	}
	return m.depthCents
}

// pitchMotionOf describes the pitch track over the audible note: a GLIDE when
// its start and end (medians, not single frames) differ by more than 50 cents,
// a WOBBLE only for a real oscillation (at least two full swings, 1.5–15 a
// second, deeper than 8 cents), else it HOLDS. A slow drift is not a wobble,
// and the jitter left after a fast drop is not one either.
func pitchMotionOf(a *soundview.Analysis) pitchMotion {
	tr := a.Pitch()
	aud := a.AudibleSec()
	var ts, cents []float64
	for i, hz := range tr.Hz {
		if t := tr.T0 + float64(i)*tr.HopSec; hz > 0 && (aud <= 0 || t <= aud) {
			ts = append(ts, t)
			cents = append(cents, 100*hzToMidiF(hz))
		}
	}
	if len(cents) < 3 {
		return pitchMotion{}
	}
	k := max(1, min(3, len(cents)/5))
	start := median(cents[:k])
	end := median(cents[len(cents)*7/10:])
	if math.Abs(start-end) > 50 {
		settle := ts[0]
		for i := len(cents) - 1; i >= 0; i-- {
			if math.Abs(cents[i]-end) > 50 {
				settle = ts[min(i+1, len(ts)-1)]
				break
			}
		}
		return pitchMotion{kind: pitchGlide, startCents: start, endCents: end, settleSec: settle - ts[0]}
	}
	med := median(cents)
	dev := make([]float64, len(cents))
	for i, c := range cents {
		dev[i] = math.Abs(c - med)
	}
	sort.Float64s(dev)
	depth := dev[len(dev)*9/10]
	cross, side := 0, 0
	for _, c := range cents {
		s := 0
		switch {
		case c > med+2:
			s = 1
		case c < med-2:
			s = -1
		}
		if s != 0 && side != 0 && s != side {
			cross++
		}
		if s != 0 {
			side = s
		}
	}
	dur := ts[len(ts)-1] - ts[0]
	rate := 0.0
	if dur > 0 {
		rate = float64(cross) / 2 / dur
	}
	if depth > 8 && cross >= 4 && rate >= 1.5 && rate <= 15 {
		return pitchMotion{kind: pitchWobble, medCents: med, depthCents: depth, rateHz: rate}
	}
	return pitchMotion{kind: pitchHold, medCents: med}
}

// numbers is the motion as measurements.
func (m pitchMotion) numbers() string {
	switch m.kind {
	case pitchGlide:
		return i18n.Tf(i18n.KeyGraphPitchGlide, fmtHzNote(midiFToHz(m.startCents/100)), fmtHzNote(midiFToHz(m.endCents/100)), fmtDur(m.settleSec))
	case pitchWobble:
		return i18n.Tf(i18n.KeyGraphPitchWobble, int(math.Round(m.depthCents)), fmt.Sprintf("%.1f", m.rateHz))
	case pitchHold:
		return i18n.Tf(i18n.KeyGraphPitchHolds, fmtHzNote(midiFToHz(m.medCents/100)))
	}
	return i18n.T(i18n.KeyGraphNoPitch)
}

// key is the one number "(was …)" repeats: where a glide starts, how deep a
// wobble swings, the held pitch.
func (m pitchMotion) key() string {
	switch m.kind {
	case pitchGlide:
		return fmtHz(midiFToHz(m.startCents / 100))
	case pitchWobble:
		return fmt.Sprintf("±%d¢", int(math.Round(m.depthCents)))
	case pitchHold:
		return fmtHz(midiFToHz(m.medCents / 100))
	}
	return ""
}

// pitchReadout is the pitch track in numbers (tests, tools).
func pitchReadout(a *soundview.Analysis) string { return pitchMotionOf(a).numbers() }

func midiFToHz(m float64) float64 { return 440 * math.Pow(2, (m-69)/12) }

func (vp *viewPlot) logHzX(f float64) float32 {
	return vp.xAt(math.Log(math.Max(f, 20)/20) / math.Log(1000))
}

// buildColour is TONE COLOUR: the note's average spectrum, every curve on ONE
// level scale (each shifted by its RefDB against the loudest on screen — a
// quieter version sits lower instead of being re-normalised to the same peak).
func buildColour(vp *viewPlot, in synthViewInput) {
	vp.dbTicks()
	for _, f := range []float64{100, 1000, 10000} {
		vp.xTicks = append(vp.xTicks, plotTick{pos: vp.logHzX(f), label: vp.hzLabel(f)})
	}
	vp.xWords = [2]string{i18n.T(i18n.KeyGraphLow), i18n.T(i18n.KeyGraphHigh)}
	ref := math.Inf(-1)
	for _, r := range in.all() {
		ref = math.Max(ref, r.an.Spectrum().RefDB)
	}
	series := func(r *graphRender) ([]float32, []float32) {
		sp := r.an.Spectrum()
		shift := sp.RefDB - ref
		xs := make([]float32, len(sp.Hz))
		ys := make([]float32, len(sp.Hz))
		for i := range sp.Hz {
			xs[i] = vp.logHzX(sp.Hz[i])
			ys[i] = vp.yAt(dbFrac(sp.DB[i] + shift))
		}
		return xs, ys
	}
	for _, g := range in.ghosts() {
		xs, ys := series(g.r)
		vp.addSeries(g.role, xs, ys, 0)
	}
	xs, ys := series(in.now)
	vp.addSeries(roleNow, xs, ys, float32(vp.plot.Max.Y))
	vp.diffShade()
	vp.ghostLabels(in)
	cen := func(r *graphRender) string { return fmtHz(r.an.CentroidHz()) }
	vp.setReadouts(in, i18n.Tf(i18n.KeyGraphBrightCentre, cen(in.now)), cen,
		brightnessWords(in.now.an.CentroidHz()), func(b, n *graphRender) string {
			return changeWords(b.an.CentroidHz(), n.an.CentroidHz(), 0.08, 1, i18n.KeyGraphWordsBrighter, i18n.KeyGraphWordsDarker)
		})
}

func buildHarmonics(vp *viewPlot, in synthViewInput) {
	vp.dbTicks()
	for k := 1; k <= 12; k++ {
		lbl := ""
		if k <= 4 || k%2 == 0 {
			lbl = fmt.Sprintf("%d", k)
		}
		vp.xTicks = append(vp.xTicks, plotTick{pos: vp.xAt(float64(k) / 12), label: lbl})
	}
	series := func(r *graphRender) ([]float32, []float32, bool) {
		h, ok := r.an.Harmonics()
		if !ok {
			return nil, nil, false
		}
		xs := make([]float32, len(h.Hz))
		ys := make([]float32, len(h.Hz))
		for i := range h.Hz {
			xs[i] = vp.xAt(h.Hz[i] / h.F0 / 12)
			ys[i] = vp.yAt(dbFrac(h.DB[i]))
		}
		return xs, ys, true
	}
	for _, g := range in.ghosts() {
		if xs, ys, ok := series(g.r); ok {
			vp.addSeries(g.role, xs, ys, 0)
		}
	}
	xs, ys, _ := series(in.now)
	vp.addSeries(roleNow, xs, ys, float32(vp.plot.Max.Y))
	vp.diffShade()
	vp.ghostLabels(in)
	count := func(r *graphRender) (int, int) {
		h, ok := r.an.Harmonics()
		if !ok {
			return 0, 0
		}
		n, best := 0, 0
		for k, db := range h.Peaks {
			if db >= -40 {
				n++
			}
			if db > h.Peaks[best] {
				best = k
			}
		}
		return n, best + 1
	}
	n, best := count(in.now)
	vp.setReadouts(in, i18n.Tf(i18n.KeyGraphHarmonicsCount, n, best), func(r *graphRender) string {
		c, _ := count(r)
		return fmt.Sprintf("%d", c)
	}, harmonicsWords(n), func(b, now *graphRender) string {
		bc, _ := count(b)
		nc, _ := count(now)
		return changeWords(float64(bc), float64(nc), 0, 2, i18n.KeyGraphWordsRicher, i18n.KeyGraphWordsPlainer)
	})
}

func (vp *viewPlot) freqY(f float64, r image.Rectangle) float32 {
	frac := math.Log(math.Max(f, 30)/30) / math.Log(16000.0/30)
	return float32(r.Max.Y) - float32(math.Max(0, math.Min(1, frac)))*float32(r.Dy())
}

// centroidMoves is a note's brightness at its start and at its end (medians of
// the first and last fifth of its loud part).
func centroidMoves(r *graphRender) (float64, float64) {
	sg := r.an.Spectrogram()
	env := r.an.Envelope()
	floor := env.MaxRMS() - 30
	var vals []float64
	for c := 0; c < sg.Cols; c++ {
		if t := sg.T0 + float64(c)*sg.HopSec; sg.CentroidHz[c] > 0 && env.RMSAt(t) >= floor {
			vals = append(vals, sg.CentroidHz[c])
		}
	}
	if len(vals) == 0 {
		return 0, 0
	}
	k := max(1, len(vals)/5)
	return median(vals[:k]), median(vals[len(vals)-k:])
}

// buildColourTime is BRIGHTNESS OVER TIME: each sound's brightness centre as a
// line across the note (dark down, bright up) — the ends and BEFORE compare
// at a glance, which a spectrogram never let them do.
func buildColourTime(vp *viewPlot, in synthViewInput) {
	span := viewTimeSpan(in, 0.1)
	vp.timeTicks(span, in.beatSec)
	// The brightness axis spans the sounds on screen (octaves, at least 1.5,
	// a quarter octave of margin) like the pitch view's — fixed while the
	// control moves (its ends are on screen), never 9 octaves for any sound.
	visible := func(r *graphRender, c int) bool {
		sg := r.an.Spectrogram()
		env := r.an.Envelope()
		t := sg.T0 + float64(c)*sg.HopSec
		return t <= span && sg.CentroidHz[c] > 0 && env.RMSAt(t) >= env.MaxRMS()-40
	}
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, r := range in.all() {
		sg := r.an.Spectrogram()
		for c := 0; c < sg.Cols; c++ {
			if visible(r, c) {
				o := math.Log2(sg.CentroidHz[c])
				lo, hi = math.Min(lo, o), math.Max(hi, o)
			}
		}
	}
	if math.IsInf(lo, 1) {
		lo, hi = math.Log2(200), math.Log2(5000)
	}
	if hi-lo < 1.5 {
		c := (hi + lo) / 2
		lo, hi = c-0.75, c+0.75
	}
	lo, hi = lo-0.25, hi+0.25
	yOf := func(hz float64) float32 { return vp.yAt((math.Log2(hz) - lo) / (hi - lo)) }
	for _, f := range []float64{50, 100, 200, 500, 1000, 2000, 5000, 10000} {
		if o := math.Log2(f); o >= lo && o <= hi {
			vp.yTicks = append(vp.yTicks, plotTick{pos: yOf(f), label: vp.hzLabel(f)})
		}
	}
	vp.yWords = [2]string{i18n.T(i18n.KeyGraphAxisDark), i18n.T(i18n.KeyGraphAxisBright)}
	centroid := func(r *graphRender, role plotRole) {
		sg := r.an.Spectrogram()
		var cur []fpt
		flush := func() {
			if len(cur) >= 2 {
				vp.lines = append(vp.lines, plotLine{role: role, pts: cur})
			}
			cur = nil
		}
		for c := 0; c < sg.Cols; c++ {
			t := sg.T0 + float64(c)*sg.HopSec
			if t > span {
				break
			}
			if !visible(r, c) {
				flush()
				continue
			}
			cur = append(cur, fpt{vp.xAt(t / span), yOf(sg.CentroidHz[c])})
		}
		flush()
	}
	for _, g := range in.ghosts() {
		centroid(g.r, g.role)
	}
	centroid(in.now, roleNow)
	vp.diffShade()
	vp.ghostLabels(in)
	a, b := centroidMoves(in.now)
	mean := func(r *graphRender) float64 {
		s, e := centroidMoves(r)
		return (s + e) / 2
	}
	vp.setReadouts(in, i18n.Tf(i18n.KeyGraphCentreMoves, fmtHz(a), fmtHz(b)), func(r *graphRender) string {
		_, e := centroidMoves(r)
		return fmtHz(e)
	}, trendWords(a, b), func(bf, n *graphRender) string {
		return changeWords(mean(bf), mean(n), 0.08, 1, i18n.KeyGraphWordsBrighter, i18n.KeyGraphWordsDarker)
	})
}

func median(v []float64) float64 {
	c := append([]float64(nil), v...)
	sort.Float64s(c)
	return c[len(c)/2]
}

// noiseShareLoudDB: only frames within this of the loudest are drawn on the
// tone-vs-noise curve — the part of the note its single number is made of
// (energy-weighted), so the curve and the number agree.
const noiseShareLoudDB = 30.0

func buildNoise(vp *viewPlot, in synthViewInput) {
	span := viewTimeSpan(in, 0.1)
	vp.timeTicks(span, in.beatSec)
	for _, pct := range []float64{0, 50, 100} {
		lbl := ""
		if vp.numbers {
			lbl = fmt.Sprintf("%.0f%%", pct)
		}
		vp.yTicks = append(vp.yTicks, plotTick{pos: vp.yAt(pct / 100), label: lbl})
	}
	vp.yWords = [2]string{i18n.T(i18n.KeyGraphTone), i18n.T(i18n.KeyGraphNoise)}
	share := func(r *graphRender, role plotRole) {
		ns := r.an.Noise()
		mx := 0.0
		for i := range ns.Tonal {
			mx = math.Max(mx, ns.Tonal[i]+ns.Noise[i])
		}
		loud := mx * math.Pow(10, -noiseShareLoudDB/10)
		var cur []fpt
		flush := func() {
			if len(cur) >= 2 {
				vp.lines = append(vp.lines, plotLine{role: role, pts: cur})
				if role == roleNow {
					xs := make([]float32, len(cur))
					top := make([]float32, len(cur))
					bot := make([]float32, len(cur))
					for i, p := range cur {
						xs[i], top[i], bot[i] = p.x, p.y, float32(vp.plot.Max.Y)
					}
					vp.bands = append(vp.bands, plotBand{role: roleNow, xs: xs, top: top, bot: bot})
				}
			}
			cur = nil
		}
		// Each point is the share over three hops (the frame and its
		// neighbours, energy-weighted): one hop of a noise burst jitters.
		at := func(i int) (float64, bool) {
			var n, e float64
			for j := max(0, i-1); j <= min(len(ns.Tonal)-1, i+1); j++ {
				n += ns.Noise[j]
				e += ns.Tonal[j] + ns.Noise[j]
			}
			return n / e, e > 0
		}
		for i := range ns.Tonal {
			t := ns.T0 + float64(i)*ns.HopSec
			if t > span {
				break
			}
			e := ns.Tonal[i] + ns.Noise[i]
			if mx <= 0 || e < loud {
				flush()
				continue
			}
			if f, ok := at(i); ok {
				cur = append(cur, fpt{vp.xAt(t / span), vp.yAt(f)})
			}
		}
		flush()
	}
	for _, g := range in.ghosts() {
		share(g.r, g.role)
	}
	share(in.now, roleNow)
	vp.diffShade()
	vp.ghostLabels(in)
	pct := func(r *graphRender) int { return int(math.Round(100 * r.an.Noise().Fraction)) }
	p := pct(in.now)
	vp.setReadouts(in, i18n.Tf(i18n.KeyGraphNoiseShare, p, 100-p), func(r *graphRender) string {
		return fmt.Sprintf("%d%%", pct(r))
	}, noiseWords(in.now.an.Noise().Fraction), func(b, n *graphRender) string {
		return changeWords(b.an.Noise().Fraction, n.an.Noise().Fraction, 0, 0.05, i18n.KeyGraphWordsNoisier, i18n.KeyGraphWordsCleaner)
	})
}

// waveGrit is how far a wave has been squared off — what drive does — from
// the FULLNESS of its lobes: each half-cycle between two zero crossings (of
// the window's mean) fills area / (peak × width) of its box, 2/π for a sine,
// 1 for a square. Mapped to 0 (sine) .. 1 (square) and averaged over the
// whole lobes in the close-up: blind to the level, to a DC offset, to the
// note decaying (each lobe is half a period) and to where the window starts.
func waveGrit(s []float32) float64 {
	n := len(s)
	if n < 30 {
		return 0
	}
	mean := 0.0
	for _, v := range s {
		mean += float64(v)
	}
	mean /= float64(n)
	minW := max(4, n/30) // shorter "lobes" are ripple around a crossing
	sum, lobes := 0.0, 0
	start, sign := -1, 0
	for i, v := range s {
		d := float64(v) - mean
		sg := 0
		switch {
		case d > 0:
			sg = 1
		case d < 0:
			sg = -1
		}
		if sg == 0 || sg == sign {
			continue
		}
		if sign != 0 && start >= 0 && i-start >= minW {
			var pk, area float64
			for _, w := range s[start:i] {
				a := math.Abs(float64(w) - mean)
				pk = math.Max(pk, a)
				area += a
			}
			if pk > 0 {
				sum += area / (pk * float64(i-start))
				lobes++
			}
		}
		if sign != 0 {
			start = i // the lobe before the first crossing is partial: skipped
		}
		sign = sg
	}
	if lobes == 0 {
		return 0
	}
	f := sum / float64(lobes)
	return math.Max(0, math.Min(1, (f-2/math.Pi)/(1-2/math.Pi)))
}

// soundGrit is a render's grit at its close-up window (waveGrit).
func soundGrit(r *graphRender, window []float32) float64 { return waveGrit(window) }

func buildWave(vp *viewPlot, in synthViewInput) {
	at := in.now.an.WaveMoment()
	type win struct {
		w    soundview.WaveWindow
		role plotRole
		sr   float64
	}
	wins := []win{{in.now.an.WaveAt(at, 3), roleNow, float64(in.now.an.SampleRate())}}
	for _, g := range in.ghosts() {
		wins = append(wins, win{g.r.an.WaveAt(at, 3), g.role, float64(g.r.an.SampleRate())})
	}
	span, peak := 0.0, 0.0
	for _, w := range wins {
		span = math.Max(span, float64(len(w.w.Samples))/w.sr)
		for _, s := range w.w.Samples {
			peak = math.Max(peak, math.Abs(float64(s)))
		}
	}
	if span <= 0 || peak <= 0 {
		return
	}
	peak *= 1.1
	mid := float32(vp.plot.Min.Y+vp.plot.Max.Y) / 2
	vp.timeTicks(span, 0)
	for i := len(wins) - 1; i >= 0; i-- {
		w := wins[i]
		n := len(w.w.Samples)
		step := max(1, n/(2*vp.plot.Dx()))
		var xs, ys []float32
		for j := 0; j < n; j += step {
			xs = append(xs, vp.xAt(float64(j)/w.sr/span))
			ys = append(ys, mid-float32(float64(w.w.Samples[j])/peak)*float32(vp.plot.Dy())/2)
		}
		vp.addSeries(w.role, xs, ys, mid)
	}
	vp.diffShade()
	vp.ghostLabels(in)
	sqOf := func(r *graphRender) float64 { return soundGrit(r, r.an.WaveAt(at, 3).Samples) }
	sq := soundGrit(in.now, wins[0].w.Samples)
	pct := func(v float64) string { return fmt.Sprintf("%.0f%%", 100*v) }
	// With the dial's two ends on screen the words place the sound between
	// them (an instrument's clean end may carry harmonics of its own);
	// otherwise on the absolute grit scale.
	words := shapeWords(sq)
	if in.lo != nil && in.hi != nil && in.lo.an != nil && in.hi.an != nil {
		lo, hi := sqOf(in.lo), sqOf(in.hi)
		if hi < lo {
			lo, hi = hi, lo
		}
		if hi-lo > 0.01 {
			switch pos := (sq - lo) / (hi - lo); {
			case pos < 0.25:
				words = i18n.T(i18n.KeyGraphWordsSmooth)
			case pos < 0.65:
				words = i18n.T(i18n.KeyGraphWordsSlightlySquared)
			default:
				words = i18n.T(i18n.KeyGraphWordsSquared)
			}
		}
	}
	vp.setReadouts(in, i18n.Tf(i18n.KeyGraphWaveCrest, fmtDur(at), pct(sq)), func(r *graphRender) string {
		return pct(sqOf(r))
	}, words, func(b, n *graphRender) string {
		return changeWords(sqOf(b), sqOf(n), 0, 0.05, i18n.KeyGraphWordsGrittier, i18n.KeyGraphWordsSmoother)
	})
}

// buildOverview is YOUR SOUND: the whole note's loudness as a band (the
// envelope's 20 ms loudness as a peak-equivalent level, drawn symmetric about
// the centre, true scale — smooth, never the cycles of a low note), the
// note-end marker and a one-line summary.
func buildOverview(vp *viewPlot, in synthViewInput) {
	n := in.now.an
	span := viewTimeSpan(in, 0.1)
	if vs := in.now.voiceSec; vs > span && vs < 2*span {
		span = niceCeil(vs)
	}
	vp.timeTicks(span, in.beatSec)
	mid := float32(vp.plot.Min.Y+vp.plot.Max.Y) / 2
	half := float32(vp.plot.Dy()) / 2
	env := n.Envelope()
	cols := vp.plot.Dx()
	band := plotBand{role: roleNow}
	var upper, lower []fpt
	for c := 0; c < cols; c++ {
		t0 := span * float64(c) / float64(cols)
		t1 := span * float64(c+1) / float64(cols)
		db := env.RMSAt((t0 + t1) / 2)
		a := float32(0)
		if db > soundview.FloorDB {
			a = float32(math.Min(1, math.Sqrt2*math.Pow(10, db/20)))
		}
		x := float32(vp.plot.Min.X + c)
		band.xs = append(band.xs, x)
		band.top = append(band.top, mid-a*half)
		band.bot = append(band.bot, mid+a*half)
		upper = append(upper, fpt{x, mid - a*half})
		lower = append(lower, fpt{x, mid + a*half})
	}
	vp.bands = append(vp.bands, band)
	vp.lines = append(vp.lines, plotLine{role: roleNow, pts: upper}, plotLine{role: roleNow, pts: lower})
	if vs := in.now.voiceSec; vs > 0 && vs < span {
		vp.markers = append(vp.markers, plotMarker{x: vp.xAt(vs / span), role: roleNow, label: i18n.T(i18n.KeyGraphNoteEnds)})
	}
	vp.readout = i18n.Tf(i18n.KeyGraphSummary, fmtDur(n.AudibleSec()), fmtHz(n.CentroidHz()), int(math.Round(100*n.Noise().Fraction)))
	vp.summary = joinWords(lengthWords(n, in.now.voiceSec), brightnessWords(n.CentroidHz()), noiseWords(n.Noise().Fraction))
}

// --- formatting ---

func fmtDur(s float64) string {
	switch {
	case s <= 0:
		return "0 ms"
	case s < 0.001:
		return "< 1 ms"
	case s < 1:
		return fmt.Sprintf("%.0f ms", s*1000)
	default:
		return fmt.Sprintf("%.2f s", s)
	}
}

// fmtHzTick is a compact axis label: 100, 1k, 10k.
func fmtHzTick(f float64) string {
	if f >= 1000 {
		return fmt.Sprintf("%gk", f/1000)
	}
	return fmt.Sprintf("%g", f)
}

func fmtHz(f float64) string {
	switch {
	case f <= 0:
		return "—"
	case f >= 1000:
		return fmt.Sprintf("%.1f kHz", f/1000)
	default:
		return fmt.Sprintf("%.0f Hz", f)
	}
}
