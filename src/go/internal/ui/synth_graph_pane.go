package ui

// Every curve is measured from audio.RenderDocForGraph, the voice a trigger plays. The root shows the overview;
// a focused dial its view, with its 0/100 ends and the gesture's BEFORE as ghosts.

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

type graphBeforeSnap struct {
	inst instrument.ID
	ctl  string
	r    *graphRender
	// Filled only with the render the gesture started from, never an older one: a stale BEFORE claims the wrong sound.
	waitFor graphIdent
	waiting bool
}

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
	// Identified by sound, not revision: an undo or a dial turned back lands the render it already has.
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
	// A dial whose named view does not move here is shown through one that does.
	if !f.picked && f.ctl != "" && lo != nil && hi != nil {
		if v := p.chosenDialView(f, lo, hi, rect.Dx(), rect.Dy()); v != f.view {
			f.view = v
			need = synthViewNeeds(v)
		}
	}
	// The last NOW keeps drawing while a newer renders, but never across instruments.
	now, nowFor, _ := pipe.getFor(slotNow)
	if now != nil && nowFor.inst != instID {
		now = nil
	}
	// Analysis runs on the worker; until it lands the previous picture stays.
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

// Cached because it flattens and hashes the whole document.
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

// Answers the declared view until the ends are analysed (on the worker, never here).
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

// Only the CURRENT document's render counts; if it has not landed, BEFORE waits for it.
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

// Full for 100 ms, then the note's own loudness until 40 dB down or 1.5 s; a short flash with no render yet.
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

// -1 at the root or when nothing is drilled.
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

// For scene Setup: two captures of a scene must match, not show whichever frame the worker finished by.
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

func (dv *DrumView) synthGraphPane() image.Rectangle {
	if r := dv.instEditorPreviewRect; !r.Empty() {
		return r
	}
	return dv.instEditorMobileFocusRect
}

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

// Idents carry the end document's CONTENT, not the revision: a turn changes the revision every detent but
// leaves the ends where they were, so the ghosts stay cached.
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

// 0 on mobile, where the graph is a band above the controls.
func synthPreviewWidth(contentR image.Rectangle, mobile bool) int {
	if mobile {
		return 0
	}
	if !devToolsOn() {
		// The graph explains every dial in production, so it gets 40 % of the tab, never under 320 px.
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
