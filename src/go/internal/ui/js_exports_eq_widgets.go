//go:build js && !test

package ui

import (
	"fmt"
	"strings"
	"syscall/js"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

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

		// Buttons
		buttons := js.Global().Get("Object").New()
		if dv.playBtn() != nil {
			buttons.Set("play", rectToJS(dv.playBtn().Rect()))
		}
		if dv.stopBtn() != nil {
			buttons.Set("stop", rectToJS(dv.stopBtn().Rect()))
		}
		if dv.bpmIncBtn() != nil {
			buttons.Set("bpmInc", rectToJS(dv.bpmIncBtn().Rect()))
		}
		if dv.bpmDecBtn() != nil {
			buttons.Set("bpmDec", rectToJS(dv.bpmDecBtn().Rect()))
		}
		if dv.addRowBtn() != nil {
			buttons.Set("addRow", rectToJS(dv.addRowBtn().Rect()))
		}
		if dv.subdivBtn() != nil {
			buttons.Set("subdiv", rectToJS(dv.subdivBtn().Rect()))
		}
		if dv.lenIncBtn != nil {
			buttons.Set("lenInc", rectToJS(dv.lenIncBtn.Rect()))
		}
		if dv.lenDecBtn != nil {
			buttons.Set("lenDec", rectToJS(dv.lenDecBtn.Rect()))
		}
		if dv.trackBtn() != nil {
			buttons.Set("track", rectToJS(dv.trackBtn().Rect()))
		}
		if dv.uploadBtn() != nil {
			buttons.Set("upload", rectToJS(dv.uploadBtn().Rect()))
		}
		if dv.importBtn() != nil {
			buttons.Set("import", rectToJS(dv.importBtn().Rect()))
		}
		if dv.exportBtn() != nil {
			buttons.Set("export", rectToJS(dv.exportBtn().Rect()))
		}
		if dv.overflowBtn() != nil {
			buttons.Set("overflow", rectToJS(dv.overflowBtn().Rect()))
		}
		if dv.viewSwitchBtn() != nil {
			buttons.Set("viewSwitch", rectToJS(dv.viewSwitchBtn().Rect()))
		}
		if dv.bpmBox() != nil {
			buttons.Set("bpmBox", rectToJS(dv.bpmBox().Rect))
		}
		if dv.mainVolSlider() != nil {
			buttons.Set("mainVol", rectToJS(dv.mainVolSlider().Rect()))
		}
		if dv.eqToggleBtn() != nil {
			buttons.Set("eqToggle", rectToJS(dv.eqToggleBtn().Rect()))
		}
		obj.Set("buttons", buttons)

		// Widgets
		widgets := js.Global().Get("Object").New()
		if r, ok := dv.widgetRects[WidgetTransport]; ok {
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
		obj.Set("eq", eq)

		// State
		state := js.Global().Get("Object").New()
		state.Set("isPlaying", g.Playing())
		state.Set("bpm", dv.BPM())
		state.Set("totalRows", len(dv.Rows))
		state.Set("suppressClicks", suppressClicksUntilRelease)
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
