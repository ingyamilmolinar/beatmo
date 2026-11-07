//go:build js && !test

package ui

import (
	"fmt"
	"image"
	"strings"
	"syscall/js"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	scope "github.com/ingyamilmolinar/beatmo/internal/scope"
)

func synthWidgetRectsJS(g *Game) interface{} {
	if g.drum == nil {
		return js.Global().Get("Array").New()
	}
	knobs := g.drum.SynthTabControls()
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
	js.Global().Set("setEQView", jsFn(func(args jsArgs) any {
		if g.drum == nil || args.Len() == 0 {
			return nil
		}
		mode := strings.ToLower(args.Str(0))
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

	// setEQTab(name)
	js.Global().Set("setEQTab", jsFn(func(args jsArgs) any {
		if g.drum == nil || g.drum.eqPanelZone == nil || args.Len() == 0 {
			return nil
		}
		// Both the legacy slugs (meters/scope) and the UI names (levels/chain).
		switch strings.ToLower(args.Str(0)) {
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

	// setSubdivisions(n) -> bool, through the canonical handler whether or not the menu exists; false while
	// playing or for an invalid step.
	js.Global().Set("setSubdivisions", jsFn(func(args jsArgs) any {
		if g.drum == nil || args.Len() < 1 {
			return false
		}
		n := args.Int(0)
		if err := g.SetSubdivisions(n); err != nil {
			return false
		}
		if b := g.drum.subdivBtn(); b != nil {
			b.Text = fmt.Sprintf("÷%d", n)
		}
		g.drum.timelineUnitsPerBeat = n
		return true
	}))

	// setViewMode(slug) -> bool, through dv.setViewMode, which also syncs the audio sub-tab and the segment.
	js.Global().Set("setViewMode", jsFn(func(args jsArgs) any {
		if g.drum == nil || args.Len() == 0 {
			return false
		}
		if m, ok := viewModeFromSlug(args.Str(0)); ok {
			g.drum.setViewMode(m)
			return true
		}
		return false
	}))

	// synthControlRects() -> [{name,x,y,w,h}]; only the DRILLED target's knobs have rects, so call
	// selectSynthSection first.
	js.Global().Set("synthControlRects", jsFn(func(args jsArgs) any {
		return synthWidgetRectsJS(g)
	}))

	// synthDialRects() -> [{card,dial,x,y,w,h}] for real-pointer benches.
	js.Global().Set("synthDialRects", jsFn(func(args jsArgs) any {
		out := js.Global().Get("Array").New()
		if g.drum == nil {
			return out
		}
		for _, c := range g.drum.instEditorMacroCards {
			for j := 0; j < c.nDials; j++ {
				r := c.dialRects[j]
				if r.Empty() {
					continue
				}
				obj := js.Global().Get("Object").New()
				obj.Set("card", c.id)
				obj.Set("dial", c.dials[j].id)
				obj.Set("x", r.Min.X)
				obj.Set("y", r.Min.Y)
				obj.Set("w", r.Dx())
				obj.Set("h", r.Dy())
				out.Call("push", obj)
			}
		}
		return out
	}))

	// synthChipRects() -> [{name,x,y,w,h,enabled,selected}] per ON stage; off stages live under "+".
	js.Global().Set("synthChipRects", jsFn(func(args jsArgs) any {
		if g.drum == nil {
			return js.Global().Get("Array").New()
		}
		pills := g.drum.instEditorPipePills
		out := js.Global().Get("Array").New(len(pills))
		for i, c := range pills {
			obj := js.Global().Get("Object").New()
			obj.Set("name", sectionLabel(c.id))
			obj.Set("x", c.rect.Min.X)
			obj.Set("y", c.rect.Min.Y)
			obj.Set("w", c.rect.Dx())
			obj.Set("h", c.rect.Dy())
			obj.Set("enabled", true)
			obj.Set("selected", c.btn != nil && c.btn.Toggled())
			out.SetIndex(i, obj)
		}
		return out
	}))

	// synthEnablePillRect() -> {x,y,w,h}, all zero when no stage is selected or the pill is inert.
	js.Global().Set("synthEnablePillRect", jsFn(func(args jsArgs) any {
		obj := js.Global().Get("Object").New()
		var r image.Rectangle
		if g.drum != nil {
			r = g.drum.synthDetailEnablePillRect()
		}
		obj.Set("x", r.Min.X)
		obj.Set("y", r.Min.Y)
		obj.Set("w", r.Dx())
		obj.Set("h", r.Dy())
		return obj
	}))

	// selectSynthSection(label) -> bool.
	js.Global().Set("selectSynthSection", jsFn(func(args jsArgs) any {
		if g.drum == nil || args.Len() == 0 {
			return false
		}
		return g.drum.SelectSynthSectionByLabel(args.Str(0))
	}))

	// synthFooterButtonRects() -> rects keyed by sentinel tag; empty fields are hidden buttons.
	js.Global().Set("synthFooterButtonRects", jsFn(func(args jsArgs) any {
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
			case synthSaveAsButtonTag:
				key = "saveAs"
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

	// synthSaveAsDialogState() -> null | {value,rect,okRect,cancelRect}.
	js.Global().Set("synthSaveAsDialogState", jsFn(func(args jsArgs) any {
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

	// synthSaveAsDialogSetValue(s): injects text without the soft-keyboard plumbing.
	js.Global().Set("synthSaveAsDialogSetValue", jsFn(func(args jsArgs) any {
		if g.drum == nil || args.Len() < 1 {
			return nil
		}
		dlg := g.drum.SynthSaveAsDialog()
		if dlg == nil {
			return nil
		}
		dlg.SetValue(args.Str(0))
		return nil
	}))

	// synthSaveAsDialogConfirm(): the same entry point the OK button fires.
	js.Global().Set("synthSaveAsDialogConfirm", jsFn(func(args jsArgs) any {
		if g.drum != nil {
			g.drum.ConfirmSaveAsDialog()
		}
		return nil
	}))

	// synthSaveAsDialogCancel() – synthesises a Cancel press.
	js.Global().Set("synthSaveAsDialogCancel", jsFn(func(args jsArgs) any {
		if g.drum != nil {
			g.drum.CancelSaveAsDialog()
		}
		return nil
	}))

	// enableScopeExport(): the runtime equivalent of desktop SCOPE_EXPORT=1.
	js.Global().Set("enableScopeExport", jsFn(func(args jsArgs) any {
		audio.EnableScopeExport()
		return nil
	}))

	// downloadScopeExport() -> bytes dumped (0 if disabled or empty).
	js.Global().Set("downloadScopeExport", jsFn(func(args jsArgs) any {
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

	// scopeExportBufferLen() -> bytes, without draining.
	js.Global().Set("scopeExportBufferLen", jsFn(func(args jsArgs) any {
		svc := audio.ExportService()
		if svc == nil {
			return 0
		}
		return svc.BufferLen()
	}))

	// forceScopeExportSnapshot(): bypasses the timer so tests do not race the background poll.
	js.Global().Set("forceScopeExportSnapshot", jsFn(func(args jsArgs) any {
		return audio.ForceScopeExportSnapshot()
	}))

	// probeAnalyzerState(): verifies the WASM synthesis path without pixel sampling.
	js.Global().Set("probeAnalyzerState", jsFn(func(args jsArgs) any {
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

	// setScopeTaps(stageA, stageB): synth/antipop/insertfx/eq/sends/master; "" clears a tap.
	js.Global().Set("setScopeTaps", jsFn(func(args jsArgs) any {
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
			case "master":
				return scope.StageMaster
			default:
				return scope.Stage(-1)
			}
		}
		sz := g.drum.eqPanelZone.chainZone
		if args.Len() > 0 {
			sz.SetTapA(nameToStage(args.Str(0)))
		}
		if args.Len() > 1 {
			sz.SetTapB(nameToStage(args.Str(1)))
		}
		g.drum.bgDirty = true
		return true
	}))

	// setEQChannel(id) -> bool: "main" or an instrument id; "" or invalid is refused.
	js.Global().Set("setEQChannel", jsFn(func(args jsArgs) any {
		if g.drum == nil {
			return false
		}
		id := args.Channel(0)
		if id.IsZero() {
			return false
		}
		// The single channel-select chokepoint, so every tab follows.
		g.drum.selectAudioChannel(id)
		g.drum.bgDirty = true
		return true
	}))

	js.Global().Set("probeScopeState", jsFn(func(args jsArgs) any {
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
	js.Global().Set("eqBandsSnapshot", jsFn(func(args jsArgs) any {
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

	// eqBandHandlePoints() -> [{band,x,y}], [] when the EQ plot is hidden.
	js.Global().Set("eqBandHandlePoints", jsFn(func(args jsArgs) any {
		out := make([]any, 0, len(eqBandDefs))
		if g.drum == nil || g.drum.eqPanelZone == nil {
			return out
		}
		z := g.drum.eqPanelZone
		if z.ActiveTab() != TabEQ || z.eqPlotRect().Empty() {
			return out
		}
		for i := range eqBandDefs {
			x, y := z.eqBandHandlePos(i)
			out = append(out, map[string]any{"band": i, "x": x, "y": y})
		}
		return out
	}))

	// eqControlsSnapshot() -> { gainsDB: [] } for current slider gains.
	js.Global().Set("eqControlsSnapshot", jsFn(func(args jsArgs) any {
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
	js.Global().Set("widgetLayoutSnapshot", jsFn(func(args jsArgs) any {
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

	// Read-back getters so a real click on a chrome pill can be checked against state; the pill rects are in the
	// snapshot's `chrome` object.
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
	js.Global().Set("k20View", jsFn(func(args jsArgs) any {
		if lc := levelsControlsOf(); lc != nil {
			return js.ValueOf(lc.K20View())
		}
		return js.ValueOf(nil)
	}))
	js.Global().Set("chainDisplayMode", jsFn(func(args jsArgs) any {
		if cz := chainZoneOf(); cz != nil {
			return js.ValueOf(cz.DisplayMode())
		}
		return js.ValueOf(nil)
	}))
	js.Global().Set("autoGain", jsFn(func(args jsArgs) any {
		if cz := chainZoneOf(); cz != nil {
			return js.ValueOf(cz.AutoGain())
		}
		return js.ValueOf(nil)
	}))
	js.Global().Set("scopeFrozen", jsFn(func(args jsArgs) any {
		if cz := chainZoneOf(); cz != nil {
			return js.ValueOf(cz.Frozen())
		}
		return js.ValueOf(nil)
	}))

	// fullLayoutSnapshot(): every component's geometry in one call, to minimise round-trips.
	js.Global().Set("fullLayoutSnapshot", jsFn(func(args jsArgs) any {
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
		// WIDER than timelineRect (the ribbon reserves header chrome): use it for anything cell-indexed.
		drumLayout.Set("cellGridRect", rectToJS(dv.cellGridRect))
		obj.Set("drumLayout", drumLayout)

		// Only controls with a real on-canvas rect: overflow-menu controls report 0×0 when not inline and would fail
		// the parity check's positive-area rule.
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
			// Capped to the drawn bar: the uncapped row-0 cell overlaps the rack band, which the rack owns.
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
			// On desktop color/edit/origin/delete live behind this kebab; on mobile the row label opens the menu.
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
		// From eqBandHandlePos, the single source of the handle centres.
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

		// Keyed by slug; zero rects on mobile, where the bottom nav covers it.
		tabs := js.Global().Get("Object").New()
		if dv.eqPanelZone != nil && dv.eqPanelZone.stickyBar != nil {
			for i, t := range AllPanelTabs() {
				if b := dv.eqPanelZone.stickyBar.TabBtn(i); b != nil {
					tabs.Set(PanelTabSlug(t), rectToJS(b.Rect()))
				}
			}
		}
		obj.Set("tabs", tabs)

		// Non-empty only on mobile; "active" is the selected segment index.
		nav := js.Global().Get("Object").New()
		if dv.viewSwitchSegmented != nil {
			// From the live list: production has no Chain segment, so a fixed table would shift every later slug.
			for i, m := range bottomNavModes() {
				nav.Set(viewModeSlug(m), rectToJS(dv.viewSwitchSegmented.SegmentRect(i)))
			}
			nav.Set("active", dv.viewSwitchSegmented.Active())
		}
		obj.Set("bottomNav", nav)

		// Non-empty only when the owning tab is visible.
		chrome := js.Global().Get("Object").New()
		setChrome := func(name string, b *Button) {
			if b != nil {
				chrome.Set(name, rectToJS(b.Rect()))
			}
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
			state.Set("channel", dv.eqPanelZone.ActiveChannel().String())
		}
		// totalNodes: graph node count — objective checkpoint for build/edit/delete tests.
		if g.graph != nil {
			state.Set("totalNodes", len(g.graph.Nodes))
		}
		// Baselines for the pinch/pan gesture tests.
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
	js.Global().Set("nudgeWidgetSplit", jsFn(func(args jsArgs) any {
		if g.drum == nil || g.drum.widgets == nil || args.Len() < 3 {
			return nil
		}
		axis := args.Str(0)
		idx := args.Int(1)
		delta := args.Int(2)
		g.drum.widgets.ResizeAxis(axis, idx, delta)
		g.drum.refreshWidgetLayout()
		g.drum.recalcButtons()
		g.drum.calcLayout()
		g.drum.invalidateRowCaches()
		g.drum.rowsLayerDirty = true
		return nil
	}))

	// moveWidget(id, col, row)
	js.Global().Set("moveWidget", jsFn(func(args jsArgs) any {
		if g.drum == nil || g.drum.widgets == nil || args.Len() < 3 {
			return nil
		}
		id := WidgetKind(strings.ToLower(args.Str(0)))
		col := args.Int(1)
		row := args.Int(2)
		g.drum.widgets.MoveWidget(id, col, row)
		g.drum.refreshWidgetLayout()
		g.drum.recalcButtons()
		g.drum.calcLayout()
		g.drum.invalidateRowCaches()
		g.drum.rowsLayerDirty = true
		return nil
	}))

	// toggleWidget(id, visible)
	js.Global().Set("toggleWidget", jsFn(func(args jsArgs) any {
		if g.drum == nil || g.drum.widgets == nil || args.Len() == 0 {
			return nil
		}
		id := WidgetKind(strings.ToLower(args.Str(0)))
		visible := true
		if args.Len() > 1 {
			visible = args.Bool(1)
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
	js.Global().Set("eqFreqResponse", jsFn(func(args jsArgs) any {
		if g.drum == nil {
			return js.Global().Get("Array").New(0)
		}
		numPoints := 128
		if args.Len() > 0 && args.At(0).Type() == js.TypeNumber {
			numPoints = args.Int(0)
			if numPoints < 2 {
				numPoints = 2
			}
		}
		ch := g.drum.activeEQChannel()
		var gains []float64
		var muted []bool
		if ch == channel.Master {
			gains = g.drum.eqBandGainsDB()
			muted = g.drum.eqBandMuted()
		} else if eq := g.drum.activeEQ(); eq != nil {
			gains, muted = eq.GainsDB, eq.BandMuted
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
	js.Global().Set("setEQHPF", jsFn(func(args jsArgs) any {
		if g.drum == nil || args.Len() < 2 {
			return nil
		}
		enabled := args.Bool(0)
		cutoffHz := args.Float(1)
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
	js.Global().Set("setEQLPF", jsFn(func(args jsArgs) any {
		if g.drum == nil || args.Len() < 2 {
			return nil
		}
		enabled := args.Bool(0)
		cutoffHz := args.Float(1)
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
	js.Global().Set("addCustomWidget", jsFn(func(args jsArgs) any {
		if g.drum == nil || g.drum.widgets == nil {
			return nil
		}
		title := "Custom"
		if args.Len() > 0 && args.At(0).Truthy() {
			title = args.Str(0)
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
