package ui

// synth_graph_pane.go — the Synth tab's graph: which control it explains, the
// renders it asks for, and how it is drawn and glows
// (spec docs/superpowers/specs/2026-10-24-synth-graphs-and-devtools-design.md §4.2–4.7).
//
//   - Nothing focused (the root): the OVERVIEW of the note as it plays.
//   - A focused dial: the dial's view, with the dial at 0 and 100 as ghosts
//     (synthdial Turn endpoints — exact, independent of where it sits) and a
//     BEFORE ghost of the sound when the gesture began.
//   - Developer builds, drilled into raw knobs: the selected knob's view, its
//     Min/Max as ghosts; the view picker can show any view.
//
// Every curve is measured from audio.RenderDocForGraph's buffer — the voice a
// trigger plays. A trigger makes the NOW curve GLOW for as long as the note
// actually sounds (its own envelope), never hiding the picture.

import (
	"image"
	"math"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/soundview"
)

// synthGraphFocusView is what the graph explains right now.
type synthGraphFocusView struct {
	view             synthView
	ctl              string // "" = overview, "dial:card/dial", "param:name"
	caption          string
	loLabel, hiLabel string
	loFields         func() map[string]float64 // nil = no LO ghost
	hiFields         func() map[string]float64
	picked           bool // the developer view picker chose the view
}

// graphBeforeSnap is the BEFORE ghost: the NOW render captured when a gesture
// began on ctl. It stays until the focus moves to another control.
type graphBeforeSnap struct {
	inst instrument.ID
	ctl  string
	r    *graphRender
	// waitFor is the NOW ident the gesture started from, when its render had
	// not landed yet at capture: r fills in once that render exists — never
	// with an older one (a stale BEFORE claims the wrong starting sound).
	waitFor graphIdent
	waiting bool
}

// synthGraphViewOverride is the developer view picker's choice for a control
// (ctl → view); empty = the control's own view.
type synthGraphViewOverride map[string]synthView

func dialCtl(card, dial string) string { return "dial:" + card + "/" + dial }
func paramCtl(name string) string      { return "param:" + name }

// synthGraphFocus resolves what the graph explains for inst.
func (p *synthPanel) synthGraphFocus(inst instrument.ID) synthGraphFocusView {
	dv := p.dv
	merged := audio.MergedInstrumentParamsRO(inst)
	if dv.synthAtMacroRoot(inst) {
		card, dial, ok := dv.synthFocusedDial(inst)
		if !ok {
			return synthGraphFocusView{view: viewOverview}
		}
		d, _ := dv.synthDialByID(card, dial)
		v, ok := synthViewByName(d.Dial.View, merged)
		if !ok {
			v = viewLoudness
		}
		ctl := dialCtl(card, dial)
		picked := false
		if o, ok := dv.graphViewOverride[ctl]; ok && devToolsOn() {
			v, picked = o, true
		}
		caption := i18n.T(i18n.Key(d.Dial.Label))
		if g := i18n.T(i18n.Key(d.Dial.Gloss)); g != "" {
			caption += " — " + g
		}
		at := func(target float64) func() map[string]float64 {
			return func() map[string]float64 {
				start := make(map[string]float64, len(d.Terms))
				for _, t := range d.Terms {
					if val, ok := merged[t.Param]; ok {
						start[t.Param] = val
					}
				}
				return d.Turn(start, target)
			}
		}
		return synthGraphFocusView{view: v, ctl: ctl, caption: caption,
			loLabel: i18n.T(i18n.Key(d.Dial.Lo)), hiLabel: i18n.T(i18n.Key(d.Dial.Hi)),
			loFields: at(0), hiFields: at(1), picked: picked}
	}
	kIdx := dv.synthGraphControlIdx(inst)
	if kIdx < 0 || kIdx >= len(dv.instEditorBindings) {
		return synthGraphFocusView{view: viewOverview}
	}
	def := dv.instEditorBindings[kIdx].def
	v, ok := synthParamView(def, merged)
	if !ok {
		v = viewLoudness
	}
	ctl := paramCtl(def.Name)
	if o, ok := dv.graphViewOverride[ctl]; ok && devToolsOn() {
		v = o
	}
	label := synthParamDisplayName(def.Name)
	if label == "" {
		label = def.Name
	}
	caption := label
	if purpose := synthControlPurpose(def); purpose != "" {
		caption = label + " — " + purpose
	}
	endLabel := func(x float64) string {
		if n := len(def.Enum); n > 0 {
			i := int(math.Round(x))
			if i >= 0 && i < n {
				return def.Enum[i]
			}
		}
		return formatSynthParamValue(def, x)
	}
	fields := func(x float64) func() map[string]float64 {
		return func() map[string]float64 { return map[string]float64{def.Name: x} }
	}
	return synthGraphFocusView{view: v, ctl: ctl, caption: caption,
		loLabel: endLabel(def.Min), hiLabel: endLabel(def.Max),
		loFields: fields(def.Min), hiFields: fields(def.Max)}
}

// synthGraphKey is the plot cache key: everything the geometry is built from.
type synthGraphKey struct {
	view                synthView
	ctl, caption        string
	now, lo, hi, before *graphRender
	w, h                int
	edit                bool
	numbers             bool // the Details toggle
	bpm                 int  // the time axis marks beats (the sound has no tempo)
}

// drawSynthGraphPane draws the graph for inst into rect.
func (p *synthPanel) drawSynthGraphPane(dst *ebiten.Image, rect image.Rectangle, instID instrument.ID) {
	dv := p.dv
	if rect.Dx() < 48 || rect.Dy() < 40 {
		return
	}
	f := p.synthGraphFocus(instID)
	pipe := dv.synthGraphPipe()
	pipe.pump()
	edit, hasEdit := audio.SampleEditFor(instID)
	base := graphIdent{inst: instID, hash: p.synthParamsHash(instID), edit: edit}
	// NOW is identified by what it sounds like, not by the revision: an undo
	// or a dial turned back to where it was lands the render it already has.
	nowID := graphIdent{inst: instID, edit: edit, variant: "now", key: p.nowSoundKey(instID, base)}
	need := synthViewNeeds(f.view)
	pipe.want(slotNow, nowID, func() graphReq {
		d, _ := audio.EnsureInstrumentDoc(instID)
		return graphReq{id: instID, doc: d, need: need}
	})
	ends := p.synthGraphEnds(base, f)
	ghost := func(slot graphSlot, e graphEnd) *graphRender {
		if !e.ok {
			return nil
		}
		pipe.want(slot, e.ident, func() graphReq { return graphReq{id: instID, doc: e.doc, need: need} })
		if r, lf, ok := pipe.getFor(slot); ok && lf.ctl == f.ctl && lf.inst == instID && pipe.warm(r, synthRenderNeeds(f.view, r)) {
			return r
		}
		return nil
	}
	lo := ghost(slotLo, ends.lo)
	hi := ghost(slotHi, ends.hi)
	// A dial whose named view does not move on this instrument is shown
	// through the view that does (synth_view_choice.go), measured on the two
	// end renders the graph already has.
	if !f.picked && f.ctl != "" && lo != nil && hi != nil {
		if v := p.chosenDialView(f, lo, hi, rect.Dx(), rect.Dy()); v != f.view {
			f.view = v
			need = synthViewNeeds(v)
		}
	}
	// The last landed NOW keeps drawing while a newer one renders — but only
	// for THIS instrument: after a row switch the previous instrument's
	// picture is never shown as this one's (no picture yet instead).
	now, nowFor, _ := pipe.getFor(slotNow)
	if now != nil && nowFor.inst != instID {
		now = nil
	}
	// A render this view has not analysed yet is warmed on the worker; until
	// then the previous picture stays (no analysis on this goroutine).
	nowReady := pipe.warm(now, synthRenderNeeds(f.view, now))
	if b := &dv.graphBefore; b.waiting && b.inst == instID {
		if r, ok := pipe.lookup(b.waitFor); ok {
			b.r, b.waiting = r, false
		}
	}
	var before *graphRender
	if b := dv.graphBefore; f.ctl != "" && b.inst == instID && b.ctl == f.ctl && b.r != nil && pipe.warm(b.r, synthRenderNeeds(f.view, b.r)) {
		before = b.r
	}
	numbers := dv.synthGraphNumbers()
	key := synthGraphKey{view: f.view, ctl: f.ctl, caption: f.caption, now: now, lo: lo, hi: hi, before: before,
		w: rect.Dx(), h: rect.Dy(), edit: hasEdit, numbers: numbers, bpm: dv.BPM()}
	if !nowReady && p.graphPlot != nil {
		key = p.graphPlotKey // keep the previous picture until NOW is analysed
	} else if !nowReady {
		key.now = nil
		now = nil
	}
	if p.graphPlot == nil || p.graphPlotKey != key {
		in := synthViewInput{now: now, lo: lo, hi: hi, before: before, loLabel: f.loLabel, hiLabel: f.hiLabel,
			caption: f.caption, samplerEdit: hasEdit, numbers: numbers}
		if key.bpm > 0 {
			in.beatSec = 60 / float64(key.bpm)
		}
		p.graphPlot = buildSynthViewPlot(f.view, rect.Dx(), rect.Dy(), in)
		p.graphPlot.pickable = devToolsOn() && f.ctl != ""
		p.graphPlotKey = key
	}
	vp := p.graphPlot
	p.sprites.graph.Draw(dst, rect, key, func(img *ebiten.Image, r image.Rectangle) {
		drawViewPlot(img, r.Min, vp)
	})
	var env *soundview.Envelope
	if now != nil && now.an != nil {
		e := now.an.Envelope()
		env = &e
	}
	if k := dv.graphGlow().envelopeGlow(env); k > 0 {
		p.glow.draw(dst, rect.Min, vp, k)
	}
	dv.drawSynthGraphDetails(dst, rect)
	if pipe.slow(slotNow) && now != nil {
		// The new picture is late (a slow render worker): say so; the previous
		// picture stays underneath.
		txt := i18n.T(i18n.KeyGraphUpdating)
		w := StyledTextWidth(txt, RoleMicro)
		x := vp.plot.Max.X + rect.Min.X - w - 4
		y := vp.plot.Min.Y + rect.Min.Y + 2
		drawRect(dst, image.Rect(x-3, y-1, x+w+3, y+StyledTextHeight(RoleMicro)+1), TokenSurface1(), true)
		DrawTextStyled(dst, txt, x, y, RoleMicro, TokenTextSecondary())
	}
	pipe.takeDirty()
}

// nowSoundMemo remembers the live document's sound key for one revision.
type nowSoundMemo struct {
	base graphIdent
	key  uint64
}

// nowSoundKey is the live document's audio.GraphRenderKey, recomputed only
// when the document revision, the tempo or the Sampler edit moves (it
// flattens and hashes the whole document).
func (p *synthPanel) nowSoundKey(inst instrument.ID, base graphIdent) uint64 {
	if p.nowKey.base == base && p.nowKey.key != 0 {
		return p.nowKey.key
	}
	d, ok := audio.EnsureInstrumentDoc(inst)
	if !ok {
		return 0
	}
	p.nowKey = nowSoundMemo{base: base, key: audio.GraphRenderKey(inst, d, 0)}
	return p.nowKey.key
}

// dialViewMemo remembers the view chosen for a focused control's two ends.
type dialViewMemo struct {
	ctl      string
	lo, hi   *graphRender
	declared synthView
	w, h     int
	v        synthView
}

// chosenDialView is chooseDialView for the focused control, memoized on its
// end renders. Until the ends are analysed for every view it answers the
// declared view (the analyses are warmed on the worker, never here).
func (p *synthPanel) chosenDialView(f synthGraphFocusView, lo, hi *graphRender, w, h int) synthView {
	m := p.viewMemo
	if m.ctl == f.ctl && m.lo == lo && m.hi == hi && m.declared == f.view && m.w == w && m.h == h {
		return m.v
	}
	remember := func(v synthView) synthView {
		p.viewMemo = dialViewMemo{ctl: f.ctl, lo: lo, hi: hi, declared: f.view, w: w, h: h, v: v}
		return v
	}
	if viewMovePx(f.view, lo, hi, w, h) >= viewMinMovePx {
		return remember(f.view)
	}
	pipe := p.dv.synthGraphPipe()
	if !pipe.warm(lo, soundview.NeedAll) || !pipe.warm(hi, soundview.NeedAll) {
		return f.view
	}
	v, _ := chooseDialView(f.view, lo, hi, w, h)
	return remember(v)
}

// captureGraphBefore snapshots the current NOW render as the BEFORE ghost of
// ctl (called when a gesture begins on a dial or a raw knob).
//
// Only the render of the CURRENT document counts: when it has not landed yet
// (an edit just before the press), BEFORE waits for it rather than taking the
// older picture.
func (dv *DrumView) captureGraphBefore(inst instrument.ID, ctl string) {
	if dv.graphPipe == nil {
		return
	}
	want, ok := dv.graphPipe.wanted(slotNow)
	if !ok || want.inst != inst {
		dv.graphBefore = graphBeforeSnap{}
		return
	}
	if r, ok := dv.graphPipe.lookup(want); ok {
		dv.graphBefore = graphBeforeSnap{inst: inst, ctl: ctl, r: r}
		return
	}
	dv.graphBefore = graphBeforeSnap{inst: inst, ctl: ctl, waitFor: want, waiting: true}
}

// --- glow ---

const (
	glowMinHold = 100 * time.Millisecond
	glowMaxSec  = 1.5
	glowFloorDB = -40.0
)

// envelopeGlow is the trigger glow's intensity 0..1 now: full for the first
// 100 ms, then the note's own loudness (linear, re its loudest moment) at the
// elapsed time, until it falls 40 dB under its peak or 1.5 s pass. With no
// render yet it falls back to a short flash.
func (g *synthGlow) envelopeGlow(env *soundview.Envelope) float64 {
	if !g.glowing {
		return 0
	}
	el := g.clock().Sub(g.glowStart)
	if el < glowMinHold {
		return 1
	}
	if env == nil || len(env.RMSdB) == 0 {
		if el > 2*glowMinHold {
			g.glowing = false
			return 0
		}
		return 1 - float64(el-glowMinHold)/float64(glowMinHold)
	}
	t := el.Seconds()
	db := env.RMSAt(t) - env.MaxRMS()
	if t > glowMaxSec || db < glowFloorDB || t > float64(len(env.RMSdB))*env.HopSec {
		g.glowing = false
		return 0
	}
	return math.Pow(10, db/20)
}

// synthGraphControlIdx picks the control the graph explains for instID: the
// armed control, else the sticky selection when it is part of the drilled
// target, else the drilled target's first essential. -1 at the root or when
// nothing is drilled.
func (dv *DrumView) synthGraphControlIdx(instID instrument.ID) int {
	if dv.synthAtMacroRoot(instID) {
		return -1
	}
	if idx, armed := dv.synthControlArmed(instID); armed && idx >= 0 && idx < len(dv.instEditorBindings) {
		return idx
	}
	order := dv.instEditorDrilledOrder
	if len(order) == 0 {
		return -1
	}
	if dv.instEditorSelectedControl != nil {
		if idx, ok := dv.instEditorSelectedControl[instID]; ok {
			for _, k := range order {
				if k == idx {
					return idx
				}
			}
		}
	}
	return order[0]
}

// settleSynthGraphNow renders, synchronously, every render the graph wants
// for the instrument the Synth tab shows (and the renders those reveal: the
// focused control's ends). Scene Setup calls it so a capture shows the landed
// picture, never whichever frame the worker happened to finish by — two
// captures of a scene must match (TestRuntimeThemeSwitchMatchesBootEveryScene).
func (dv *DrumView) settleSynthGraphNow() {
	if dv == nil || dv.eqPanelZone == nil || dv.eqPanelZone.ActiveTab() != TabSynth {
		return
	}
	inst := dv.resolveSynthInstrument(dv.synthTabActiveInstrument())
	if inst == "" {
		return
	}
	r := dv.instEditorPreviewRect
	if r.Empty() {
		r = dv.instEditorMobileFocusRect
	}
	if r.Dx() < 48 || r.Dy() < 40 {
		r = image.Rect(0, 0, 400, 200)
	}
	scratch := ebiten.NewImage(r.Dx(), r.Dy())
	defer scratch.Deallocate()
	pipe := dv.synthGraphPipe()
	for i := 0; i < 3; i++ {
		dv.synp().drawSynthGraphPane(scratch, image.Rect(0, 0, r.Dx(), r.Dy()), inst)
		pipe.drainForTest()
		pipe.waitIdle()
	}
	dv.synp().sprites.graph.Invalidate()
	pipe.takeDirty()
}

// --- developer view picker ---

// synthGraphPickOrder is the cycle a tap on the developer view picker walks.
var synthGraphPickOrder = []synthView{viewLoudness, viewAttack, viewPitch, viewColour, viewHarmonics, viewColourTime, viewNoise, viewWave, viewOverview}

// synthGraphPane is the rect the graph draws into (desktop side pane or the
// phone band).
func (dv *DrumView) synthGraphPane() image.Rectangle {
	if r := dv.instEditorPreviewRect; !r.Empty() {
		return r
	}
	return dv.instEditorMobileFocusRect
}

// synthGraphPickRect is the picker's tap target: the caption row's right
// end, where the view title is drawn.
func synthGraphPickRect(pane image.Rectangle) image.Rectangle {
	h := viewPad/2 + StyledTextHeight(RoleMicro) + 4
	return image.Rect(max(pane.Min.X, pane.Max.X-150), pane.Min.Y, pane.Max.X, pane.Min.Y+h)
}

func (p *synthPanel) synthGraphPickHitAreas(inst instrument.ID, z int) []HitArea {
	dv := p.dv
	pane := dv.synthGraphPane()
	if !devToolsOn() || pane.Empty() || inst == "" {
		return nil
	}
	f := p.synthGraphFocus(inst)
	if f.ctl == "" {
		return nil
	}
	return []HitArea{{
		Rect:    synthGraphPickRect(pane),
		ZIndex:  z,
		Handler: &synthGraphPickAdapter{dv: dv, inst: inst},
		Tag:     "synth-graph-pick",
	}}
}

// cycleSynthGraphView moves the focused control's view to the next one in
// synthGraphPickOrder (developer builds).
func (dv *DrumView) cycleSynthGraphView(inst instrument.ID) {
	f := dv.synp().synthGraphFocus(inst)
	if f.ctl == "" || !devToolsOn() {
		return
	}
	next := synthGraphPickOrder[0]
	for i, v := range synthGraphPickOrder {
		if v == f.view {
			next = synthGraphPickOrder[(i+1)%len(synthGraphPickOrder)]
		}
	}
	if dv.graphViewOverride == nil {
		dv.graphViewOverride = synthGraphViewOverride{}
	}
	dv.graphViewOverride[f.ctl] = next
	dv.invalidateSynthPanel()
}

type synthGraphPickAdapter struct {
	dv   *DrumView
	inst instrument.ID
}

func (h *synthGraphPickAdapter) OnPress(x, y int) InputResult { return InputCaptured }
func (h *synthGraphPickAdapter) OnDrag(x, y int)              {}
func (h *synthGraphPickAdapter) OnRelease(x, y int) {
	h.dv.cycleSynthGraphView(h.dv.resolveSynthInstrument(h.inst))
}
func (h *synthGraphPickAdapter) OnWheel(x, y, steps int) InputResult { return InputIgnored }

// graphEnd is one end ghost's document and its content-addressed ident.
type graphEnd struct {
	ident graphIdent
	doc   instrument.Doc
	ok    bool
}

type graphEndsMemo struct {
	base   graphIdent
	ctl    string
	lo, hi graphEnd
}

// synthGraphEnds returns the focused control's end documents, rebuilt only
// when the document revision or the control changes. Their idents carry the
// end document's CONTENT (audio.GraphRenderKey), not the revision: turning the
// dial changes the revision every detent but lands its ends exactly where
// they were, so the ghosts stay cached instead of re-rendering per detent.
func (p *synthPanel) synthGraphEnds(base graphIdent, f synthGraphFocusView) graphEndsMemo {
	if p.graphEnds.base == base && p.graphEnds.ctl == f.ctl {
		return p.graphEnds
	}
	end := func(variant string, fields func() map[string]float64) graphEnd {
		if fields == nil {
			return graphEnd{}
		}
		d, err := audio.InstrumentDocWithFields(base.inst, fields())
		if err != nil {
			return graphEnd{}
		}
		id := graphIdent{inst: base.inst, edit: base.edit, variant: variant, ctl: f.ctl,
			key: audio.GraphRenderKey(base.inst, d, 0)}
		return graphEnd{ident: id, doc: d, ok: true}
	}
	p.graphEnds = graphEndsMemo{base: base, ctl: f.ctl, lo: end("lo", f.loFields), hi: end("hi", f.hiFields)}
	return p.graphEnds
}

// synthPreviewWidth is the horizontal slice reserved for the graph pane on
// desktop layouts. Returns 0 on mobile (no horizontal split — the graph is a
// band above the controls there).
func synthPreviewWidth(contentR image.Rectangle, mobile bool) int {
	if mobile {
		return 0
	}
	if !devToolsOn() {
		// Production: the graph is the explanation of every dial, so it
		// takes 40 % of the tab (spec 2026-10-24 §4.2), never under 320 px.
		w := max(contentR.Dx()*2/5, 320)
		if w > contentR.Dx()*3/5 {
			return 0
		}
		return w
	}
	target := Profile().DensityValues().SynthPreviewTargetW
	avail := contentR.Dx() / 3
	if avail > target {
		return target
	}
	if avail < Profile().DensityValues().SynthPreviewMinW {
		return 0
	}
	return avail
}
