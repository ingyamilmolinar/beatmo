//go:build js && !test

package ui

import (
	"syscall/js"
)

// initJSScenes registers every public opener/closer and the runScene/listScenes
// bridge. All mutations route through QueueAction so they take effect on the
// next Update tick after seqMu is released — JS must never enter UI code
// synchronously while drum.Update holds the per-frame lock.
func (g *Game) initJSScenes() {
	// ─── menus ──────────────────────────────────────────────────
	js.Global().Set("openContextMenu", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		row := jsArgInt(args, 0, 0)
		g.QueueAction(func(g *Game) { g.drum.OpenContextMenu(row) })
		return nil
	}))
	js.Global().Set("closeContextMenu", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		g.QueueAction(func(g *Game) { g.drum.CloseContextMenu() })
		return nil
	}))
	js.Global().Set("openOverflowMenu", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		g.QueueAction(func(g *Game) { g.drum.OpenOverflowMenu() })
		return nil
	}))
	js.Global().Set("closeOverflowMenu", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		g.QueueAction(func(g *Game) { g.drum.CloseOverflowMenu() })
		return nil
	}))
	js.Global().Set("openColorMenu", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		row := jsArgInt(args, 0, 0)
		g.QueueAction(func(g *Game) { g.drum.OpenColorMenu(row) })
		return nil
	}))
	js.Global().Set("closeColorMenu", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		g.QueueAction(func(g *Game) { g.drum.CloseColorMenu() })
		return nil
	}))
	js.Global().Set("openInstrumentMenu", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		row := jsArgInt(args, 0, 0)
		g.QueueAction(func(g *Game) { g.drum.OpenInstrumentMenu(row) })
		return nil
	}))
	js.Global().Set("closeInstrumentMenu", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		g.QueueAction(func(g *Game) { g.drum.CloseInstrumentMenu() })
		return nil
	}))
	js.Global().Set("openSubdivMenu", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		g.QueueAction(func(g *Game) { g.drum.OpenSubdivMenu() })
		return nil
	}))
	js.Global().Set("closeSubdivMenu", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		g.QueueAction(func(g *Game) { g.drum.CloseSubdivMenu() })
		return nil
	}))

	// ─── FX panel ──────────────────────────────────────────────
	js.Global().Set("openFXPanel", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		row := jsArgInt(args, 0, 0)
		g.QueueAction(func(g *Game) { g.drum.OpenFXPanel(row) })
		return nil
	}))
	js.Global().Set("closeFXPanel", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		g.QueueAction(func(g *Game) { g.drum.CloseFXPanel() })
		return nil
	}))

	// ─── volume popups ─────────────────────────────────────────
	js.Global().Set("openMasterVolumePopup", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		g.QueueAction(func(g *Game) { g.drum.OpenMasterVolumePopup() })
		return nil
	}))
	js.Global().Set("openVolumePopup", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		row := jsArgInt(args, 0, 0)
		g.QueueAction(func(g *Game) { g.drum.OpenVolumePopup(row) })
		return nil
	}))
	js.Global().Set("closeVolumePopup", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		g.QueueAction(func(g *Game) { g.drum.CloseVolumePopup() })
		return nil
	}))

	// ─── audio setters ─────────────────────────────────────────
	js.Global().Set("setMasterVolume", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		v := jsArgFloat(args, 0, 1)
		g.QueueAction(func(g *Game) { g.SetMasterVolume(v) })
		return nil
	}))
	js.Global().Set("setEQBandGain", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		channel := jsArgString(args, 0, "main")
		band := jsArgInt(args, 1, 0)
		gainDB := jsArgFloat(args, 2, 0)
		g.QueueAction(func(g *Game) { g.SetEQBandGain(channel, band, gainDB) })
		return nil
	}))

	// ─── scene catalog ─────────────────────────────────────────
	js.Global().Set("runScene", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		name := jsArgString(args, 0, "")
		if name == "" {
			return js.ValueOf(false)
		}
		g.QueueAction(func(g *Game) { _ = RunScene(g, name) })
		return js.ValueOf(true)
	}))
	js.Global().Set("listScenes", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		scenes := ListScenes()
		arr := js.Global().Get("Array").New(len(scenes))
		for i, s := range scenes {
			obj := js.Global().Get("Object").New()
			obj.Set("name", s.Name)
			obj.Set("description", s.Description)
			obj.Set("mobile", s.Mobile)
			arr.SetIndex(i, obj)
		}
		return arr
	}))
}

// jsArgInt returns args[idx].Int() or def if missing.
func jsArgInt(args []js.Value, idx, def int) int {
	if idx >= len(args) || !args[idx].Truthy() {
		return def
	}
	return args[idx].Int()
}

// jsArgFloat returns args[idx].Float() or def if missing.
func jsArgFloat(args []js.Value, idx int, def float64) float64 {
	if idx >= len(args) {
		return def
	}
	if args[idx].Type() != js.TypeNumber {
		return def
	}
	return args[idx].Float()
}

// jsArgString returns args[idx].String() or def if missing.
func jsArgString(args []js.Value, idx int, def string) string {
	if idx >= len(args) || !args[idx].Truthy() {
		return def
	}
	return args[idx].String()
}
