//go:build js && !test

package ui

import (
	"fmt"
	"image"
	"strings"
	"syscall/js"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	scope "github.com/ingyamilmolinar/beatmo/internal/scope"
)

// synthWidgetRectsJS builds the {name, x, y, w, h} payload for the Synth
// tab's wired knob widgets, used by synthKnobRects.
func synthWidgetRectsJS(g *Game) interface{} {
	if g.drum == nil {
		return js.Global().Get("Array").New()
	}
	knobs := g.drum.SynthTabKnobs()
	bindings := g.drum.SynthTabBindings()
	out := js.Global().Get("Array").New(len(knobs))
	for i, k := range knobs {
		if i >= len(bindings) {
			break
		}
		r := k.Rect()
		obj := js.Global().Get("Object").New()
		obj.Set("name", bindings[i].def.Name)
		obj.Set("min", bindings[i].def.Min)
		obj.Set("max", bindings[i].def.Max)
		obj.Set("x", r.Min.X)
		obj.Set("y", r.Min.Y)
		obj.Set("w", r.Dx())
		obj.Set("h", r.Dy())
		out.SetIndex(i, obj)
	}
	return out
}

func (g *Game) initJSEqWidgets() {
	// setEQView(mode) – "wave" or "eq" to switch bottom panel visualization.
	js.Global().Set("setEQView", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || len(args) == 0 {
			return nil
		}
		mode := strings.ToLower(args[0].String())
		g.drum.eqWaveformMode = mode != "eq"
		if g.drum.eqPanelZone != nil {
			if mode != "eq" {
				g.drum.eqPanelZone.tabState.SetActiveTab(TabWave)
			} else {
				g.drum.eqPanelZone.tabState.SetActiveTab(TabEQ)
			}
		}
		return nil
	}))

	// setEQTab(name) – switch the EQ panel's tab by name. Accepts "eq",
	// "wave", "spectrum", "meters", "scope", "synth". Used for browser-test automation.
	js.Global().Set("setEQTab", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || g.drum.eqPanelZone == nil || len(args) == 0 {
			return nil
		}
		// Accept both the legacy slugs (meters/scope) and the canonical UI slugs
		// (levels/chain) so agent tests can switch by the user-facing tab name.
		// "sampler" is now switchable too.
		switch strings.ToLower(args[0].String()) {
		case "eq":
			g.drum.eqPanelZone.SetActiveTab(TabEQ)
		case "wave":
			g.drum.eqPanelZone.SetActiveTab(TabWave)
		case "spectrum":
			g.drum.eqPanelZone.SetActiveTab(TabSpectrum)
		case "meters", "levels":
			g.drum.eqPanelZone.SetActiveTab(TabMeters)
		case "scope", "chain":
			g.drum.eqPanelZone.SetActiveTab(TabScope)
			g.drum.bgDirty = true
		case "synth":
			g.drum.eqPanelZone.SetActiveTab(TabSynth)
		case "sampler":
			g.drum.eqPanelZone.SetActiveTab(TabSampler)
		}
		return nil
	}))

	// setSubdivisions(n) – set the grid subdivision (4/8/16/32) DIRECTLY via the
	// canonical SetSubdivisions handler, independent of the dropdown menu being
	// built/open. Deterministic path for agent/browser tests. Returns true on
	// success; false if rejected (e.g. while playing, or n is not a valid
	// divisor step). Keeps the subdiv button label + timeline units in sync so
	// screenshots and the timeline match, exactly like the menu item would.
	js.Global().Set("setSubdivisions", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || len(args) < 1 {
			return false
		}
		n := args[0].Int()
		if err := g.SetSubdivisions(n); err != nil {
			return false
		}
		if b := g.drum.subdivBtn(); b != nil {
			b.Text = fmt.Sprintf("÷%d", n)
		}
		g.drum.timelineUnitsPerBeat = n
		return true
	}))

	// setViewMode(slug) – switch the mobile view-mode / bottom-nav segment by
	// canonical slug (pads/eq/wave/spectrum/levels/chain/synth/sampler). This is
	// the deterministic path for agent tests to drive the mobile bottom-nav
	// without pixel-clicking the segmented control. Routes through
	// dv.setViewMode, which also syncs the audio sub-tab and the segment
	// highlight. On desktop a non-"pads" slug shows the corresponding audio tab;
	// "pads" is the default Rows view. Returns true when the slug is recognized.
	js.Global().Set("setViewMode", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || len(args) == 0 {
			return false
		}
		if m, ok := viewModeFromSlug(strings.ToLower(args[0].String())); ok {
			g.drum.setViewMode(m)
			return true
		}
		return false
	}))

	// synthKnobRects() – returns an array of {name, x, y, w, h} for each
	// knob widget currently rendered in the Synth tab. Coordinates are
	// canvas-relative. Chip-strip redesign: only the SELECTED stage's knobs
	// have non-empty rects — call selectSynthSection(label) first to open
	// the stage that owns the knob you want to drive.
	js.Global().Set("synthKnobRects", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		return synthWidgetRectsJS(g)
	}))

	// synthChipRects() – the pipeline chip strip model: one {name, x, y, w,
	// h, enabled, selected} per stage chip, in audio-pipeline order.
	js.Global().Set("synthChipRects", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return js.Global().Get("Array").New()
		}
		chips := g.drum.SynthTabChips()
		out := js.Global().Get("Array").New(len(chips))
		for i, c := range chips {
			obj := js.Global().Get("Object").New()
			obj.Set("name", sectionLabel(c.id))
			obj.Set("x", c.rect.Min.X)
			obj.Set("y", c.rect.Min.Y)
			obj.Set("w", c.rect.Dx())
			obj.Set("h", c.rect.Dy())
			obj.Set("enabled", c.enabled)
			obj.Set("selected", c.selected)
			out.SetIndex(i, obj)
		}
		return out
	}))

	// selectSynthSection(label) – open a stage in the Synth tab's detail
	// pane by chip label ("OSC", "ENVELOPE", …). Returns true when the
	// stage exists for the active instrument. Browser tests use this before
	// locating a knob via synthKnobRects.
	js.Global().Set("selectSynthSection", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || len(args) == 0 {
			return false
		}
		return g.drum.SelectSynthSectionByLabel(args[0].String())
	}))

	// synthFooterButtonRects() – returns the Save / Save As / Reset
	// footer button rects keyed by sentinel tag, so a browser test can
	// dispatch a click at the visible centre of each. Empty fields
	// indicate the button is hidden (e.g. very narrow panel).
	js.Global().Set("synthFooterButtonRects", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		obj := js.Global().Get("Object").New()
		if g.drum == nil {
			return obj
		}
		for _, b := range g.drum.SynthTabButtons() {
			if b == nil {
				continue
			}
			key := ""
			switch b.Text {
			case synthSaveButtonTag:
				key = "save"
			case synthSaveAsButtonTag:
				key = "saveAs"
			case synthResetButtonTag:
				key = "reset"
			}
			if key == "" {
				continue
			}
			r := b.Rect()
			entry := js.Global().Get("Object").New()
			entry.Set("x", r.Min.X)
			entry.Set("y", r.Min.Y)
			entry.Set("w", r.Dx())
			entry.Set("h", r.Dy())
			obj.Set(key, entry)
		}
		return obj
	}))

	// synthSaveAsDialogState() – returns null when no dialog is open,
	// otherwise an object {value, rect, okRect, cancelRect}. The
	// browser test reads this to know whether the dialog is visible
	// and to click OK / Cancel without depending on internal layout.
	js.Global().Set("synthSaveAsDialogState", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return js.Null()
		}
		dlg := g.drum.SynthSaveAsDialog()
		if dlg == nil {
			return js.Null()
		}
		obj := js.Global().Get("Object").New()
		obj.Set("value", dlg.Value())
		rectObj := func(r image.Rectangle) js.Value {
			o := js.Global().Get("Object").New()
			o.Set("x", r.Min.X)
			o.Set("y", r.Min.Y)
			o.Set("w", r.Dx())
			o.Set("h", r.Dy())
			return o
		}
		obj.Set("rect", rectObj(dlg.Rect()))
		obj.Set("okRect", rectObj(dlg.OKRect()))
		obj.Set("cancelRect", rectObj(dlg.CancelRect()))
		return obj
	}))

	// synthSaveAsDialogSetValue(s) – injects a typed name without
	// driving the soft-keyboard plumbing. Browser tests use this so
	// the test stays decoupled from input-method specifics.
	js.Global().Set("synthSaveAsDialogSetValue", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || len(args) < 1 {
			return nil
		}
		dlg := g.drum.SynthSaveAsDialog()
		if dlg == nil {
			return nil
		}
		dlg.SetValue(args[0].String())
		return nil
	}))

	// synthSaveAsDialogConfirm() – synthesises an OK press through the
	// same DrumView entry point the OK button fires.
	js.Global().Set("synthSaveAsDialogConfirm", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum != nil {
			g.drum.ConfirmSaveAsDialog()
		}
		return nil
	}))

	// synthSaveAsDialogCancel() – synthesises a Cancel press.
	js.Global().Set("synthSaveAsDialogCancel", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum != nil {
			g.drum.CancelSaveAsDialog()
		}
		return nil
	}))

	// enableScopeExport() – starts the WASM flight recorder. Matches the
	// desktop SCOPE_EXPORT=1 behaviour so browser tests can opt in at runtime.
	js.Global().Set("enableScopeExport", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		audio.EnableScopeExport()
		return nil
	}))

	// downloadScopeExport() – flushes the in-memory JSONL buffer and triggers
	// a file download via the existing downloadJSON helper. Returns the byte
	// length of the dumped buffer (0 if export wasn't enabled or buffer empty).
	js.Global().Set("downloadScopeExport", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		svc := audio.ExportService()
		if svc == nil {
			return 0
		}
		data := svc.DumpBuffer()
		if len(data) == 0 {
			return 0
		}
		fn := js.Global().Get("downloadJSON")
		if fn.Truthy() {
			fn.Invoke("scope_export.jsonl", string(data))
		}
		return len(data)
	}))

	// scopeExportBufferLen() – returns the current JSONL buffer byte count
	// without draining it. Lets browser tests detect snapshot activity.
	js.Global().Set("scopeExportBufferLen", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		svc := audio.ExportService()
		if svc == nil {
			return 0
		}
		return svc.BufferLen()
	}))

	// forceScopeExportSnapshot() – immediately polls analyzers, pushes samples,
	// and buffers one snapshot, bypassing the 2-second timer. Browser tests use
	// this to avoid racing against the background goroutine's 100ms poll cycle.
	js.Global().Set("forceScopeExportSnapshot", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		return audio.ForceScopeExportSnapshot()
	}))

	// probeAnalyzerState() – returns a small diagnostic object describing what
	// the current EQ-panel AnalyzerState callback produces. Used by browser
	// tests to verify the WASM synthesis path without pixel sampling.
	js.Global().Set("probeAnalyzerState", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		obj := js.Global().Get("Object").New()
		if g.drum == nil || g.drum.eqPanelZone == nil || g.drum.eqPanelZone.callbacks.AnalyzerState == nil {
			obj.Set("available", false)
			return obj
		}
		state := g.drum.eqPanelZone.callbacks.AnalyzerState()
		obj.Set("available", state != nil)
		if state == nil {
			return obj
		}
		obj.Set("instruments", len(state.Instruments))
		obj.Set("masterPeakDB", state.Master.PeakDB)
		obj.Set("masterRMSDB", state.Master.RMSDB)
		obj.Set("masterActive", state.Master.Active)
		obj.Set("masterFFTBins", len(state.Master.FFTBins))
		obj.Set("masterWaveformLen", len(state.Master.Waveform))
		obj.Set("hasDetail", state.Detail != nil)
		return obj
	}))

	// setScopeTaps(stageA, stageB) – set TapA/TapB stage selection by name for
	// browser tests. Names: "synth","antipop","insertfx","eq","sends","master".
	// Pass "" (or omit) to clear a tap (stage = -1).
	js.Global().Set("setScopeTaps", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || g.drum.eqPanelZone == nil || g.drum.eqPanelZone.chainZone == nil {
			return false
		}
		nameToStage := func(s string) scope.Stage {
			switch strings.ToLower(s) {
			case "synth":
				return scope.StageSynth
			case "antipop":
				return scope.StageAntiPop
			case "insertfx":
				return scope.StageInsertFX
			case "eq":
				return scope.StageEQ
			case "sends":
				return scope.StageSends
			case "master":
				return scope.StageMaster
			default:
				return scope.Stage(-1)
			}
		}
		sz := g.drum.eqPanelZone.chainZone
		if len(args) > 0 {
			sz.SetTapA(nameToStage(args[0].String()))
		}
		if len(args) > 1 {
			sz.SetTapB(nameToStage(args[1].String()))
		}
		g.drum.bgDirty = true
		return true
	}))

	// setEQChannel(id) – switch the EQ panel's active channel by id for
	// browser-test automation. Passing "main" or "" routes to the master
	// channel; any other string routes to the matching instrument id.
	// Drives the same code path the in-UI channel-cycle button takes,
	// including the chain-zone instrumentID update and analyser service
	// switch so probeScopeState reflects the new channel.
	js.Global().Set("setEQChannel", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return false
		}
		id := ""
		if len(args) > 0 {
			id = args[0].String()
		}
		// Route through the single channel-select chokepoint so every tab
		// follows the dropdown: EQ/zone channel, analyzer detail channel
		// (Wave/Spectrum), scope instrument and chain zone (Chain).
		g.drum.selectAudioChannel(id)
		g.drum.bgDirty = true
		return true
	}))

	// probeScopeState() – returns a small diagnostic object describing what
	// the current ScopeState callback produces.
	js.Global().Set("probeScopeState", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		obj := js.Global().Get("Object").New()
		if g.drum == nil || g.drum.eqPanelZone == nil || g.drum.eqPanelZone.chainZone == nil {
			obj.Set("available", false)
			return obj
		}
		sz := g.drum.eqPanelZone.chainZone
		if sz.callbacks.ScopeState == nil {
			obj.Set("available", false)
			return obj
		}
		state := sz.callbacks.ScopeState()
		obj.Set("available", state != nil)
		if state == nil {
			return obj
		}
		obj.Set("tapAActive", state.TapA.Active)
		obj.Set("tapASamples", len(state.TapA.Samples))
		obj.Set("tapAPeakDB", state.TapA.PeakDB)
		obj.Set("tapBActive", state.TapB.Active)
		obj.Set("tapBSamples", len(state.TapB.Samples))
		obj.Set("tapBPeakDB", state.TapB.PeakDB)
		return obj
	}))

	// eqBandsSnapshot() -> { names: [], values: [] } from the last draw.
	js.Global().Set("eqBandsSnapshot", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		obj := js.Global().Get("Object").New()
		eqCenterLabels := [10]string{"31", "62", "125", "250", "500", "1k", "2k", "4k", "8k", "16k"}
		names := js.Global().Get("Array").New(len(eqBandDefs))
		for i := range eqBandDefs {
			names.SetIndex(i, eqCenterLabels[i])
		}
		obj.Set("names", names)
		if g.drum == nil || len(g.drum.eqLastBands) == 0 {
			obj.Set("values", js.Global().Get("Array").New(0))
			return obj
		}
		vals := js.Global().Get("Array").New(len(g.drum.eqLastBands))
		for i, v := range g.drum.eqLastBands {
			vals.SetIndex(i, v)
		}
		obj.Set("values", vals)
		return obj
	}))

	// eqControlsSnapshot() -> { gainsDB: [] } for current slider gains.
	js.Global().Set("eqControlsSnapshot", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		obj := js.Global().Get("Object").New()
		if g.drum == nil || len(g.drum.eqBandGainsDB()) == 0 {
			obj.Set("gainsDB", js.Global().Get("Array").New(0))
			return obj
		}
		arr := js.Global().Get("Array").New(len(g.drum.eqBandGainsDB()))
		for i, v := range g.drum.eqBandGainsDB() {
			arr.SetIndex(i, v)
		}
		obj.Set("gainsDB", arr)
		return obj
	}))

	// widgetLayoutSnapshot() -> { layout, rack, timeline, wave, addButton, splitterY }
	js.Global().Set("widgetLayoutSnapshot", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return js.ValueOf(nil)
		}
		snap := g.drum.widgetRectsSnapshot()
		obj := js.Global().Get("Object").New()
		obj.Set("addButton", rectToJS(snap.AddButton))
		obj.Set("timeline", rectToJS(snap.Timeline))
		obj.Set("rack", rectToJS(snap.Rack))
		obj.Set("wave", rectToJS(snap.Wave))
		obj.Set("transport", rectToJS(snap.Transport))
		obj.Set("splitterY", g.split.Y)
		layout := js.Global().Get("Object").New()
		layout.Set("cols", floatSliceToJS(snap.Layout.Cols))
		layout.Set("rows", floatSliceToJS(snap.Layout.Rows))
		ws := js.Global().Get("Array").New(len(snap.Layout.Widgets))
		for i, w := range snap.Layout.Widgets {
			el := js.Global().Get("Object").New()
			el.Set("id", string(w.ID))
			el.Set("col", w.Col)
			el.Set("row", w.Row)
			el.Set("colSpan", w.ColSpan)
			el.Set("rowSpan", w.RowSpan)
			el.Set("visible", w.Visible)
			ws.SetIndex(i, el)
		}
		layout.Set("widgets", ws)
		obj.Set("layout", layout)
		return obj
	}))

	// Audio-panel chrome read-back getters — let agent tests checkpoint that a
	// real click on a tab-chrome pill actually changed the underlying state
	// (slope/Pre/K-20/log-lin on the sticky bar; display-mode/AG/freeze on Chain).
	// These wrap existing zone accessors; the pill rects live in the snapshot's
	// `chrome` object so the click itself stays a real, resilient interaction.
	spectrumControlsOf := func() *spectrumControls {
		if g.drum == nil || g.drum.eqPanelZone == nil {
			return nil
		}
		return g.drum.eqPanelZone.spectrumControls
	}
	levelsControlsOf := func() *levelsControls {
		if g.drum == nil || g.drum.eqPanelZone == nil {
			return nil
		}
		return g.drum.eqPanelZone.levelsControls
	}
	chainZoneOf := func() *ChainPanelZone {
		if g.drum == nil || g.drum.eqPanelZone == nil {
			return nil
		}
		return g.drum.eqPanelZone.chainZone
	}
	js.Global().Set("spectrumSlope", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if sc := spectrumControlsOf(); sc != nil {
			return js.ValueOf(sc.SlopeDBPerOct())
		}
		return js.ValueOf(nil)
	}))
	js.Global().Set("preOverlay", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if sc := spectrumControlsOf(); sc != nil {
			return js.ValueOf(sc.PreOverlay())
		}
		return js.ValueOf(nil)
	}))
	js.Global().Set("k20View", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if lc := levelsControlsOf(); lc != nil {
			return js.ValueOf(lc.K20View())
		}
		return js.ValueOf(nil)
	}))
	js.Global().Set("freqScaleLog", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if sc := spectrumControlsOf(); sc != nil {
			return js.ValueOf(sc.FreqScaleLog())
		}
		return js.ValueOf(nil)
	}))
	js.Global().Set("chainDisplayMode", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if cz := chainZoneOf(); cz != nil {
			return js.ValueOf(cz.DisplayMode())
		}
		return js.ValueOf(nil)
	}))
	js.Global().Set("autoGain", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if cz := chainZoneOf(); cz != nil {
			return js.ValueOf(cz.AutoGain())
		}
		return js.ValueOf(nil)
	}))
	js.Global().Set("scopeFrozen", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if cz := chainZoneOf(); cz != nil {
			return js.ValueOf(cz.Frozen())
		}
		return js.ValueOf(nil)
	}))

	// fullLayoutSnapshot() -> comprehensive layout geometry for visual parity tests.
	// Returns all UI component positions in a single call to minimize round-trips.
	js.Global().Set("fullLayoutSnapshot", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || g.split == nil {
			return js.ValueOf(nil)
		}
		dv := g.drum
		obj := js.Global().Get("Object").New()

		// Canvas dimensions
		obj.Set("canvasWidth", g.winW)
		obj.Set("canvasHeight", g.winH)

		// Grid pane
		gridR := g.split.GridRect(g.winW, g.winH)
		obj.Set("gridPane", rectToJS(gridR))
		obj.Set("gridTopOffset", gridTopOffset())

		// Splitter
		obj.Set("splitterY", g.split.Y)
		obj.Set("splitterX", g.split.X)
		obj.Set("layoutHorizontal", g.split.Horizontal())

		// Drum pane
		obj.Set("drumPane", rectToJS(dv.Bounds))

		// Drum layout details
		drumLayout := js.Global().Get("Object").New()
		drumLayout.Set("headerH", dv.headerH)
		drumLayout.Set("eqH", dv.eqH)
		drumLayout.Set("rowsAreaHeight", dv.rowsAreaHeight())
		drumLayout.Set("rowHeight", dv.rowHeight())
		drumLayout.Set("visibleRows", dv.visibleRows())
		drumLayout.Set("numRows", len(dv.Rows))
		drumLayout.Set("isSmallScreen", Profile().IsMobile())
		drumLayout.Set("timelineRect", rectToJS(dv.timelineRect))
		obj.Set("drumLayout", drumLayout)

		// Buttons. Only emit a control when it has a real on-canvas rect:
		// since the June UX overhaul, several controls (Import/Export/Upload,
		// the view switch, the main-volume slider) live behind the overflow
		// menu on desktop and report an empty 0×0 rect when not laid out
		// inline. Reporting them as 0×0 top-level buttons would make the
		// layout-parity check ("accessible buttons have positive area") fail
		// for controls that are intentionally reached via the overflow menu.
		// setBtn skips empty rects so the snapshot reflects what is actually
		// on screen; relocated controls simply drop out (the overflow button
		// that reaches them is still reported).
		buttons := js.Global().Get("Object").New()
		setBtn := func(name string, r image.Rectangle) {
			if !r.Empty() {
				buttons.Set(name, rectToJS(r))
			}
		}
		if dv.playBtn() != nil {
			setBtn("play", dv.playBtn().Rect())
		}
		if dv.stopBtn() != nil {
			setBtn("stop", dv.stopBtn().Rect())
		}
		if dv.bpmIncBtn() != nil {
			setBtn("bpmInc", dv.bpmIncBtn().Rect())
		}
		if dv.bpmDecBtn() != nil {
			setBtn("bpmDec", dv.bpmDecBtn().Rect())
		}
		if dv.addRowBtn() != nil {
			setBtn("addRow", dv.addRowBtn().Rect())
		}
		if dv.subdivBtn() != nil {
			setBtn("subdiv", dv.subdivBtn().Rect())
		}
		if dv.lenIncBtn != nil {
			setBtn("lenInc", dv.lenIncBtn.Rect())
		}
		if dv.lenDecBtn != nil {
			setBtn("lenDec", dv.lenDecBtn.Rect())
		}
		if dv.trackBtn() != nil {
			setBtn("track", dv.trackBtn().Rect())
		}
		if dv.uploadBtn() != nil {
			setBtn("upload", dv.uploadBtn().Rect())
		}
		if dv.importBtn() != nil {
			setBtn("import", dv.importBtn().Rect())
		}
		if dv.exportBtn() != nil {
			setBtn("export", dv.exportBtn().Rect())
		}
		if dv.overflowBtn() != nil {
			setBtn("overflow", dv.overflowBtn().Rect())
		}
		if dv.viewSwitchBtn() != nil {
			setBtn("viewSwitch", dv.viewSwitchBtn().Rect())
		}
		if dv.bpmBox() != nil {
			setBtn("bpmBox", dv.bpmBox().Rect)
		}
		if dv.mainVolSlider() != nil {
			setBtn("mainVol", dv.mainVolSlider().Rect())
		}
		if dv.eqToggleBtn() != nil {
			setBtn("eqToggle", dv.eqToggleBtn().Rect())
		}
		obj.Set("buttons", buttons)

		// Widgets
		widgets := js.Global().Get("Object").New()
		if r, ok := dv.widgetRects[WidgetTransport]; ok {
			// The transport widgetRect is the uncapped grid row-0 cell, which
			// can be taller than the actual drawn transport bar: refreshWidgetLayout
			// caps the header to headerH and starts the rack at Bounds.Min.Y+headerH,
			// but never re-caps the transport rect's Max.Y. Reporting the uncapped
			// rect makes the layout-parity overlap check see transport bleeding into
			// the rack band (which the rack actually owns). Cap to the drawn bar so
			// the snapshot matches what's on screen.
			if capY := dv.Bounds.Min.Y + dv.headerH; r.Max.Y > capY {
				r.Max.Y = capY
			}
			widgets.Set("transport", rectToJS(r))
		}
		if r, ok := dv.widgetRects[WidgetRack]; ok {
			widgets.Set("rack", rectToJS(r))
		}
		if r, ok := dv.widgetRects[WidgetTimeline]; ok {
			widgets.Set("timeline", rectToJS(r))
		}
		if r, ok := dv.widgetRects[WidgetWave]; ok {
			widgets.Set("wave", rectToJS(r))
		}
		obj.Set("widgets", widgets)

		// Per visible row controls
		rowCount := len(dv.Rows)
		rowsArr := js.Global().Get("Array").New(rowCount)
		for i := 0; i < rowCount; i++ {
			row := js.Global().Get("Object").New()
			if i < len(dv.rowLabels()) && dv.rowLabels()[i] != nil {
				row.Set("label", rectToJS(dv.rowLabels()[i].Rect()))
			}
			if i < len(dv.rowMuteBtns()) && dv.rowMuteBtns()[i] != nil {
				row.Set("mute", rectToJS(dv.rowMuteBtns()[i].Rect()))
			}
			if i < len(dv.rowSoloBtns()) && dv.rowSoloBtns()[i] != nil {
				row.Set("solo", rectToJS(dv.rowSoloBtns()[i].Rect()))
			}
			if i < len(dv.rowColorBtns()) && dv.rowColorBtns()[i] != nil {
				row.Set("color", rectToJS(dv.rowColorBtns()[i].Rect()))
			}
			if i < len(dv.rowEditBtns()) && dv.rowEditBtns()[i] != nil {
				row.Set("edit", rectToJS(dv.rowEditBtns()[i].Rect()))
			}
			if i < len(dv.rowFXBtns()) && dv.rowFXBtns()[i] != nil {
				row.Set("fx", rectToJS(dv.rowFXBtns()[i].Rect()))
			}
			if i < len(dv.rowOriginBtns()) && dv.rowOriginBtns()[i] != nil {
				row.Set("origin", rectToJS(dv.rowOriginBtns()[i].Rect()))
			}
			if i < len(dv.rowDeleteBtns()) && dv.rowDeleteBtns()[i] != nil {
				row.Set("delete", rectToJS(dv.rowDeleteBtns()[i].Rect()))
			}
			if i < len(dv.rowVolSliders()) && dv.rowVolSliders()[i] != nil {
				row.Set("volume", rectToJS(dv.rowVolSliders()[i].Rect()))
			}
			// menu: the per-row "⋯" kebab that opens the overflow context menu.
			// On desktop this is where color/edit/origin/delete live (they have
			// zero rects inline); on mobile the row label opens the context menu
			// instead. Agents reach those controls via this kebab + contextMenu*.
			if i < len(dv.rowMenuBtns()) && dv.rowMenuBtns()[i] != nil {
				row.Set("menu", rectToJS(dv.rowMenuBtns()[i].Rect()))
			}
			// Per-row playback state — objective checkpoints for mute/solo steps.
			if i < len(dv.Rows) && dv.Rows[i] != nil {
				row.Set("muted", dv.Rows[i].Muted)
				row.Set("soloed", dv.Rows[i].Solo)
			}
			rowsArr.SetIndex(i, row)
		}
		obj.Set("rows", rowsArr)

		// Scrollbar
		obj.Set("scrollBar", rectToJS(dv.scrollBarRect()))
		obj.Set("scrollThumb", rectToJS(dv.scrollThumbRect()))

		// EQ controls
		eq := js.Global().Get("Object").New()
		if dv.eqChannelBtn() != nil {
			eq.Set("channelBtn", rectToJS(dv.eqChannelBtn().Rect()))
		}
		if dv.hpfBtn() != nil {
			eq.Set("hpfBtn", rectToJS(dv.hpfBtn().Rect()))
		}
		if dv.lpfBtn() != nil {
			eq.Set("lpfBtn", rectToJS(dv.lpfBtn().Rect()))
		}
		eq.Set("rect", rectToJS(dv.eqRect))
		if len(dv.eqMuteBtns()) > 0 {
			eqMutes := js.Global().Get("Array").New(len(dv.eqMuteBtns()))
			for j, b := range dv.eqMuteBtns() {
				if b != nil {
					eqMutes.SetIndex(j, rectToJS(b.Rect()))
				}
			}
			eq.Set("muteBtns", eqMutes)
		}
		// EQ band handle centres (screen coords) — single source of truth via
		// eqBandHandlePos. The agent drags from a handle (e.g. bands[3]) to change
		// that band's gain, then verifies via eqControlsSnapshot().gainsDB[i].
		if dv.eqPanelZone != nil {
			bands := js.Global().Get("Array").New(len(eqBandDefs))
			for i := range eqBandDefs {
				hx, hy := dv.eqPanelZone.eqBandHandlePos(i)
				o := js.Global().Get("Object").New()
				o.Set("x", hx)
				o.Set("y", hy)
				bands.SetIndex(i, o)
			}
			eq.Set("bands", bands)
		}
		obj.Set("eq", eq)

		// Audio-panel tab pills (desktop sticky bar). Keyed by canonical slug
		// (eq/wave/spectrum/levels/chain/synth/sampler) so an agent can switch
		// tabs by name. On mobile the sticky-bar tab strip is suppressed (zero
		// rects) — the bottomNav segmented control covers it.
		tabs := js.Global().Get("Object").New()
		if dv.eqPanelZone != nil && dv.eqPanelZone.stickyBar != nil {
			for i, t := range AllPanelTabs() {
				if b := dv.eqPanelZone.stickyBar.TabBtn(i); b != nil {
					tabs.Set(PanelTabSlug(t), rectToJS(b.Rect()))
				}
			}
		}
		obj.Set("tabs", tabs)

		// Mobile bottom-nav segmented control. Keyed by view-mode slug
		// (pads/eq/wave/spectrum/levels/chain/synth/sampler). Rects are
		// non-empty only on mobile; "active" is the selected segment index.
		nav := js.Global().Get("Object").New()
		if dv.viewSwitchSegmented != nil {
			navSlugs := []string{"pads", "eq", "wave", "spectrum", "levels", "chain", "synth", "sampler"}
			for i, slug := range navSlugs {
				nav.Set(slug, rectToJS(dv.viewSwitchSegmented.SegmentRect(i)))
			}
			nav.Set("active", dv.viewSwitchSegmented.Active())
		}
		obj.Set("bottomNav", nav)

		// Audio-panel chrome pill rects for the ACTIVE tab (slope/pre/k20/clearClips/
		// logFreq/resetHold on the sticky bar; overlay/split/diff/ag/freeze on Chain).
		// Non-empty only when the owning tab is visible. The agent clicks these by
		// name ("chrome:slope") and verifies via the chrome getters above.
		chrome := js.Global().Get("Object").New()
		setChrome := func(name string, b *Button) {
			if b != nil {
				chrome.Set(name, rectToJS(b.Rect()))
			}
		}
		if dv.eqPanelZone != nil && dv.eqPanelZone.spectrumControls != nil {
			sc := dv.eqPanelZone.spectrumControls
			setChrome("slope", sc.slopeBtn)
			setChrome("pre", sc.preBtn)
			setChrome("logFreq", sc.freqScaleBtn)
			setChrome("resetHold", sc.resetHoldBtn)
		}
		if dv.eqPanelZone != nil && dv.eqPanelZone.levelsControls != nil {
			lc := dv.eqPanelZone.levelsControls
			setChrome("k20", lc.k20Btn)
			setChrome("clearClips", lc.clearClipsBtn)
		}
		if dv.eqPanelZone != nil && dv.eqPanelZone.chainZone != nil {
			cz := dv.eqPanelZone.chainZone
			setChrome("overlay", cz.OverlayBtn())
			setChrome("split", cz.SplitBtn())
			setChrome("diff", cz.DiffBtn())
			setChrome("ag", cz.AGBtn())
			setChrome("freeze", cz.FreezeBtn())
		}
		obj.Set("chrome", chrome)

		// State
		state := js.Global().Get("Object").New()
		state.Set("isPlaying", g.Playing())
		state.Set("bpm", dv.BPM())
		state.Set("totalRows", len(dv.Rows))
		state.Set("suppressClicks", suppressClicksUntilRelease)
		// subdiv: current grid subdivision (4/8/16/32) — objective checkpoint.
		if g.grid != nil {
			state.Set("subdiv", g.grid.MaxDiv())
		}
		// activeTab: canonical slug of the audio panel's selected tab.
		if dv.eqPanelZone != nil {
			state.Set("activeTab", PanelTabSlug(dv.eqPanelZone.ActiveTab()))
		}
		// viewMode: mobile view-mode slug ("pads" on desktop / Rows view).
		state.Set("viewMode", viewModeSlug(dv.currentViewMode))
		// channel: the EQ/analysis channel id the panel is following.
		if dv.eqPanelZone != nil {
			state.Set("channel", dv.eqPanelZone.ActiveChannel())
		}
		// totalNodes: graph node count — objective checkpoint for build/edit/delete tests.
		if g.graph != nil {
			state.Set("totalNodes", len(g.graph.Nodes))
		}
		// Camera zoom + pan — objective checkpoints for the pinch/pan/drag gesture tests
		// (compared with relational ops gt/lt/ne against a baseline the agent reads here).
		if g.cam != nil {
			state.Set("camScale", g.cam.Scale)
			state.Set("camOffsetX", g.cam.OffsetX)
			state.Set("camOffsetY", g.cam.OffsetY)
		}
		obj.Set("state", state)

		// Touch configuration
		touchCfg := js.Global().Get("Object").New()
		touchCfg.Set("minTarget", TouchMinTarget())
		touchCfg.Set("rowHeight", TouchRowHeight())
		touchCfg.Set("minCellWidth", MinCellWidth())
		touchCfg.Set("grabZone", TouchGrabZone())
		obj.Set("touchConfig", touchCfg)

		return obj
	}))

	// nudgeWidgetSplit(axis, idx, deltaPx) – resize column/row boundary.
	js.Global().Set("nudgeWidgetSplit", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || g.drum.widgets == nil || len(args) < 3 {
			return nil
		}
		axis := args[0].String()
		idx := args[1].Int()
		delta := args[2].Int()
		g.drum.widgets.ResizeAxis(axis, idx, delta)
		g.drum.refreshWidgetLayout()
		g.drum.recalcButtons()
		g.drum.calcLayout()
		g.drum.invalidateRowCaches()
		g.drum.rowsLayerDirty = true
		return nil
	}))

	// moveWidget(id, col, row)
	js.Global().Set("moveWidget", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || g.drum.widgets == nil || len(args) < 3 {
			return nil
		}
		id := WidgetKind(strings.ToLower(args[0].String()))
		col := args[1].Int()
		row := args[2].Int()
		g.drum.widgets.MoveWidget(id, col, row)
		g.drum.refreshWidgetLayout()
		g.drum.recalcButtons()
		g.drum.calcLayout()
		g.drum.invalidateRowCaches()
		g.drum.rowsLayerDirty = true
		return nil
	}))

	// toggleWidget(id, visible)
	js.Global().Set("toggleWidget", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || g.drum.widgets == nil || len(args) == 0 {
			return nil
		}
		id := WidgetKind(strings.ToLower(args[0].String()))
		visible := true
		if len(args) > 1 {
			visible = args[1].Bool()
		}
		g.drum.widgets.ToggleWidget(id, visible)
		g.drum.refreshWidgetLayout()
		g.drum.recalcButtons()
		g.drum.calcLayout()
		g.drum.invalidateRowCaches()
		g.drum.rowsLayerDirty = true
		return nil
	}))

	// eqFreqResponse(numPoints?) -> [{freq, gainDB}, ...] for the active channel's EQ.
	js.Global().Set("eqFreqResponse", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return js.Global().Get("Array").New(0)
		}
		numPoints := 128
		if len(args) > 0 && args[0].Type() == js.TypeNumber {
			numPoints = args[0].Int()
			if numPoints < 2 {
				numPoints = 2
			}
		}
		ch := g.drum.activeEQChannel()
		var gains []float64
		var muted []bool
		if ch == "main" {
			gains = g.drum.eqBandGainsDB()
			muted = g.drum.eqBandMuted()
		} else {
			for _, r := range g.drum.Rows {
				if r.Instrument == ch {
					gains = r.EQGainsDB
					muted = r.EQBandMuted
					break
				}
			}
		}
		bands := g.drum.buildFullEQBands(gains, muted,
			g.drum.activeHPFEnabled(), g.drum.activeHPFCutoffHz(),
			g.drum.activeLPFEnabled(), g.drum.activeLPFCutoffHz())
		points := audio.ComputeFreqResponse(audio.SampleRate(), bands, numPoints, 20, 20000)
		arr := js.Global().Get("Array").New(len(points))
		for i, p := range points {
			obj := js.Global().Get("Object").New()
			obj.Set("freq", p.FreqHz)
			obj.Set("gainDB", p.GainDB)
			arr.SetIndex(i, obj)
		}
		return arr
	}))

	// setEQHPF(enabled, cutoffHz) – sets HPF state for the active EQ channel.
	js.Global().Set("setEQHPF", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || len(args) < 2 {
			return nil
		}
		enabled := args[0].Bool()
		cutoffHz := args[1].Float()
		if cutoffHz < 20 {
			cutoffHz = 20
		}
		if cutoffHz > 2000 {
			cutoffHz = 2000
		}
		g.drum.setActiveHPF(enabled, cutoffHz)
		g.drum.applyEQ()
		g.drum.eqCurveDirty = true
		g.drum.syncFilterButtonStyles()
		return nil
	}))

	// setEQLPF(enabled, cutoffHz) – sets LPF state for the active EQ channel.
	js.Global().Set("setEQLPF", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || len(args) < 2 {
			return nil
		}
		enabled := args[0].Bool()
		cutoffHz := args[1].Float()
		if cutoffHz < 1000 {
			cutoffHz = 1000
		}
		if cutoffHz > 20000 {
			cutoffHz = 20000
		}
		g.drum.setActiveLPF(enabled, cutoffHz)
		g.drum.applyEQ()
		g.drum.eqCurveDirty = true
		g.drum.syncFilterButtonStyles()
		return nil
	}))

	// addCustomWidget(title) – inserts a placeholder widget in the first free cell.
	js.Global().Set("addCustomWidget", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || g.drum.widgets == nil {
			return nil
		}
		title := "Custom"
		if len(args) > 0 && args[0].Truthy() {
			title = args[0].String()
		}
		id := WidgetKind(fmt.Sprintf("custom-%d", len(g.drum.widgets.placements)+1))
		g.drum.widgets.AddWidget(WidgetPlacement{ID: id, Title: title, Col: 0, Row: 2, ColSpan: 1, RowSpan: 1, MinW: 160, MinH: g.drum.rowHeight() * 2, Editable: true})
		g.drum.refreshWidgetLayout()
		g.drum.recalcButtons()
		g.drum.calcLayout()
		g.drum.invalidateRowCaches()
		g.drum.rowsLayerDirty = true
		return nil
	}))
}
