//go:build js && !test

package ui

import (
	"syscall/js"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
)

// Every mutation is queued for the next Update: JS must never enter UI code while drum.Update holds seqMu.
func (g *Game) initJSScenes() {
	// ─── menus ──────────────────────────────────────────────────
	js.Global().Set("openContextMenu", jsFn(func(args jsArgs) any {
		row := args.IntOr(0, 0)
		g.QueueAction(func(g *Game) { g.drum.OpenContextMenu(row) })
		return nil
	}))
	js.Global().Set("closeContextMenu", jsFn(func(args jsArgs) any {
		g.QueueAction(func(g *Game) { g.drum.CloseContextMenu() })
		return nil
	}))
	js.Global().Set("openOverflowMenu", jsFn(func(args jsArgs) any {
		g.QueueAction(func(g *Game) { g.drum.OpenOverflowMenu() })
		return nil
	}))
	js.Global().Set("closeOverflowMenu", jsFn(func(args jsArgs) any {
		g.QueueAction(func(g *Game) { g.drum.CloseOverflowMenu() })
		return nil
	}))

	// menuKeyNavState(menu) -> {selected,count} | null; menu is overflow/subdiv/jump/context/instrument.
	js.Global().Set("menuKeyNavState", jsFn(func(args jsArgs) any {
		if g.drum == nil {
			return nil
		}
		var nav *MenuKeyNav
		var count int
		switch args.StrOr(0, "overflow") {
		case "overflow":
			if !g.drum.IsOverflowMenuOpen() {
				return nil
			}
			nav = g.drum.OverflowKeyNavForTest()
			count = overflowNavHost{dv: g.drum}.MenuItemCount()
		case "subdiv":
			if g.drum.subdivMenuComp == nil {
				return nil
			}
			nav = g.drum.subdivMenuComp.MenuKeyNavForTest()
			count = g.drum.subdivMenuComp.MenuItemCount()
		case "jump":
			if g.drum.jumpMenuComp == nil {
				return nil
			}
			nav = g.drum.jumpMenuComp.MenuKeyNavForTest()
			count = g.drum.jumpMenuComp.MenuItemCount()
		case "context":
			if !g.drum.IsContextMenuOpen() {
				return nil
			}
			nav = g.drum.ContextKeyNavForTest()
			count = contextNavHost{dv: g.drum}.MenuItemCount()
		case "instrument":
			if g.drum.instMenuComp == nil {
				return nil
			}
			nav = g.drum.instMenuComp.MenuKeyNavForTest()
			count = g.drum.instMenuComp.MenuItemCount()
		default:
			return nil
		}
		if nav == nil {
			return nil
		}
		return map[string]any{"selected": nav.Selected(), "count": count}
	}))

	// overflowMenuScrollState() -> {first,total,visible} | null: only a browser sees real DOM wheel deltas
	// (ebiten.Wheel is stubbed under -tags test).
	js.Global().Set("overflowMenuScrollState", jsFn(func(args jsArgs) any {
		if g.drum == nil || !g.drum.IsOverflowMenuOpen() {
			return nil
		}
		sc := g.drum.OverflowMenuScrollForTest()
		if sc == nil {
			return nil
		}
		vs := sc.ScrollBehavior().VS
		return map[string]any{
			"first":   vs.First,
			"total":   vs.Total,
			"visible": vs.Visible,
		}
	}))
	js.Global().Set("openColorMenu", jsFn(func(args jsArgs) any {
		row := args.IntOr(0, 0)
		g.QueueAction(func(g *Game) { g.drum.OpenColorMenu(row) })
		return nil
	}))
	js.Global().Set("closeColorMenu", jsFn(func(args jsArgs) any {
		g.QueueAction(func(g *Game) { g.drum.CloseColorMenu() })
		return nil
	}))
	js.Global().Set("openInstrumentMenu", jsFn(func(args jsArgs) any {
		row := args.IntOr(0, 0)
		g.QueueAction(func(g *Game) { g.drum.OpenInstrumentMenu(row) })
		return nil
	}))
	js.Global().Set("closeInstrumentMenu", jsFn(func(args jsArgs) any {
		g.QueueAction(func(g *Game) { g.drum.CloseInstrumentMenu() })
		return nil
	}))
	js.Global().Set("openSubdivMenu", jsFn(func(args jsArgs) any {
		g.QueueAction(func(g *Game) { g.drum.OpenSubdivMenu() })
		return nil
	}))
	js.Global().Set("closeSubdivMenu", jsFn(func(args jsArgs) any {
		g.QueueAction(func(g *Game) { g.drum.CloseSubdivMenu() })
		return nil
	}))

	// ─── FX panel ──────────────────────────────────────────────
	js.Global().Set("openFXPanel", jsFn(func(args jsArgs) any {
		row := args.IntOr(0, 0)
		g.QueueAction(func(g *Game) { g.drum.OpenFXPanel(row) })
		return nil
	}))
	js.Global().Set("closeFXPanel", jsFn(func(args jsArgs) any {
		g.QueueAction(func(g *Game) { g.drum.CloseFXPanel() })
		return nil
	}))

	// ─── volume popups ─────────────────────────────────────────
	js.Global().Set("openMasterVolumePopup", jsFn(func(args jsArgs) any {
		g.QueueAction(func(g *Game) { g.drum.OpenMasterVolumePopup() })
		return nil
	}))
	js.Global().Set("openVolumePopup", jsFn(func(args jsArgs) any {
		row := args.IntOr(0, 0)
		g.QueueAction(func(g *Game) { g.drum.OpenVolumePopup(row) })
		return nil
	}))
	js.Global().Set("closeVolumePopup", jsFn(func(args jsArgs) any {
		g.QueueAction(func(g *Game) { g.drum.CloseVolumePopup() })
		return nil
	}))

	// ─── audio setters ─────────────────────────────────────────
	js.Global().Set("setMasterVolume", jsFn(func(args jsArgs) any {
		v := args.FloatOr(0, 1)
		g.QueueAction(func(g *Game) { g.SetMasterVolume(v) })
		return nil
	}))
	js.Global().Set("setEQBandGain", jsFn(func(args jsArgs) any {
		ch := args.ChannelOr(0, channel.Master)
		if ch.IsZero() {
			return nil
		}
		band := args.IntOr(1, 0)
		gainDB := args.FloatOr(2, 0)
		g.QueueAction(func(g *Game) { g.SetEQBandGain(ch, band, gainDB) })
		return nil
	}))

	// ─── scene catalog ─────────────────────────────────────────
	js.Global().Set("runScene", jsFn(func(args jsArgs) any {
		name := args.StrOr(0, "")
		if name == "" {
			return js.ValueOf(false)
		}
		g.QueueAction(func(g *Game) { _ = RunScene(g, name) })
		return js.ValueOf(true)
	}))
	js.Global().Set("runSceneMobile", jsFn(func(args jsArgs) any {
		name := args.StrOr(0, "")
		if name == "" {
			return js.ValueOf(false)
		}
		g.QueueAction(func(g *Game) { _ = RunSceneMobile(g, name) })
		return js.ValueOf(true)
	}))
	js.Global().Set("listScenes", jsFn(func(args jsArgs) any {
		scenes := ListScenes()
		arr := js.Global().Get("Array").New(len(scenes))
		for i, s := range scenes {
			obj := js.Global().Get("Object").New()
			obj.Set("name", s.Name)
			obj.Set("description", s.Description)
			obj.Set("mobile", s.Mobile)
			obj.Set("subject", string(s.Subject))
			// Frames at 60/s; 0 = the default 90.
			obj.Set("settleFrames", s.SettleFrames)
			arr.SetIndex(i, obj)
		}
		return arr
	}))

	// subjectRectJS(name) -> {x,y,w,h,visible}; "" is the full framebuffer. Read after runScene + forceDraw.
	js.Global().Set("subjectRectJS", jsFn(func(args jsArgs) any {
		name := args.StrOr(0, "")
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

	// listSubjects(): every named subject (not the full-screen one).
	js.Global().Set("listSubjects", jsFn(func(args jsArgs) any {
		subs := AllSubjects()
		arr := js.Global().Get("Array").New(len(subs))
		for i, s := range subs {
			arr.SetIndex(i, string(s))
		}
		return arr
	}))
}

// Type-checked: Int() panics on a non-number, and a panic in a js.FuncOf callback kills the WASM runtime.
// (Not Truthy: 0 is a valid index.)
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
