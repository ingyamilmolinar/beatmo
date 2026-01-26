//go:build js && !test

package ui

import (
	"fmt"
	"strings"
	"syscall/js"
)

func (g *Game) initJSEqWidgets() {
	// setEQView(mode) – "wave" or "eq" to switch bottom panel visualization.
	js.Global().Set("setEQView", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || len(args) == 0 {
			return nil
		}
		mode := strings.ToLower(args[0].String())
		g.drum.eqWaveformMode = mode != "eq"
		if g.drum.eqToggleBtn != nil {
			if g.drum.eqWaveformMode {
				g.drum.eqToggleBtn.Text = "Wave"
			} else {
				g.drum.eqToggleBtn.Text = "EQ"
			}
		}
		return nil
	}))

	// eqBandsSnapshot() -> { names: [], values: [] } from the last draw.
	js.Global().Set("eqBandsSnapshot", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		obj := js.Global().Get("Object").New()
		names := js.Global().Get("Array").New(len(eqBandDefs))
		for i, b := range eqBandDefs {
			lbl := fmt.Sprintf("%.0f-%.0f", b.loHz, b.hiHz)
			names.SetIndex(i, lbl)
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
		if g.drum == nil || len(g.drum.eqBandGainsDB) == 0 {
			obj.Set("gainsDB", js.Global().Get("Array").New(0))
			return obj
		}
		arr := js.Global().Get("Array").New(len(g.drum.eqBandGainsDB))
		for i, v := range g.drum.eqBandGainsDB {
			arr.SetIndex(i, v)
		}
		obj.Set("gainsDB", arr)
		return obj
	}))

	// widgetLayoutSnapshot() -> { layout, rack, timeline, wave, addButton }
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
