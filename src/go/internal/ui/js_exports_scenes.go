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
	js.Global().Set("runSceneMobile", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		name := jsArgString(args, 0, "")
		if name == "" {
			return js.ValueOf(false)
		}
		g.QueueAction(func(g *Game) { _ = RunSceneMobile(g, name) })
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
			obj.Set("subject", string(s.Subject))
			// settleFrames lets the browser capture runner wait proportionally
			// (frames/60 → seconds) for the same settle the desktop binary honors
			// via screenshotThreshold. 0 means "use the default" (90 frames).
			obj.Set("settleFrames", s.SettleFrames)
			arr.SetIndex(i, obj)
		}
		return arr
	}))

	// ─── subject bounds for cropped capture ────────────────────
	// subjectRectJS(name) returns {x, y, w, h, visible} for the named
	// subject at the current laid-out frame. The empty name resolves to
	// the full framebuffer. Browser screenshot harness reads this after
	// runScene + forceDraw have settled and forwards the rect to
	// page.screenshot({clip}).
	js.Global().Set("subjectRectJS", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		name := jsArgString(args, 0, "")
		obj := js.Global().Get("Object").New()
		s, ok := SubjectByName(name)
		if !ok {
			obj.Set("visible", false)
			return obj
		}
		r, visible := g.SubjectRect(s)
		obj.Set("subject", string(s))
		obj.Set("visible", visible)
		obj.Set("x", r.Min.X)
		obj.Set("y", r.Min.Y)
		obj.Set("w", r.Dx())
		obj.Set("h", r.Dy())
		return obj
	}))

	// listSubjects() returns every named subject (excludes the
	// empty/full-screen one). Used by the bridge-smoke test catalogue.
	js.Global().Set("listSubjects", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		subs := AllSubjects()
		arr := js.Global().Get("Array").New(len(subs))
		for i, s := range subs {
			arr.SetIndex(i, string(s))
		}
		return arr
	}))
}

// jsArgInt returns args[idx].Int() or def if missing or not a number.
// The type check matters: js.Value.Int() panics on a non-number value, and a
// panic inside a js.FuncOf callback kills the whole WASM runtime — a stray
// string argument from the console or a test harness must degrade to def,
// not take the app down. (Type-check instead of Truthy: 0 is falsy but is a
// perfectly valid index.)
func jsArgInt(args []js.Value, idx, def int) int {
	if idx >= len(args) || args[idx].Type() != js.TypeNumber {
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
