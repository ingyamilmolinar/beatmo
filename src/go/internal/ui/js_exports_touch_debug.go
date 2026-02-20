//go:build js && !test

package ui

import "syscall/js"

func (g *Game) initJSTouchDebug() {
	// setTouchDebug(enabled) -> nil
	// Enables or disables touch event logging for debugging.
	js.Global().Set("setTouchDebug", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return nil
		}
		SetTouchDebugEnabled(args[0].Bool())
		return nil
	}))

	// touchDebugState() -> {count, touches: [{id, x, y}], lastGesture, lastPos, dpr, enabled}
	// Returns the current touch debug state snapshot.
	js.Global().Set("touchDebugState", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		snap := GetTouchDebugSnapshot()
		obj := js.Global().Get("Object").New()
		obj.Set("count", snap.TouchCount)
		obj.Set("dpr", snap.DPR)
		obj.Set("enabled", snap.DebugEnabled)
		obj.Set("lastGesture", snap.LastGestureKind)
		obj.Set("lastGestureAge", snap.LastGestureAge)

		// lastPos object
		lastPos := js.Global().Get("Object").New()
		lastPos.Set("x", snap.LastGestureX)
		lastPos.Set("y", snap.LastGestureY)
		obj.Set("lastPos", lastPos)

		// touches array
		touches := js.Global().Get("Array").New(len(snap.Touches))
		for i, t := range snap.Touches {
			tobj := js.Global().Get("Object").New()
			tobj.Set("id", t.ID)
			tobj.Set("x", t.X)
			tobj.Set("y", t.Y)
			touches.SetIndex(i, tobj)
		}
		obj.Set("touches", touches)

		return obj
	}))

	// touchEventLog() -> [{timestamp, kind, touchId, x, y}]
	// Returns the last 50 touch events for debugging.
	js.Global().Set("touchEventLog", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		events := GetTouchEventLog()
		arr := js.Global().Get("Array").New(len(events))
		for i, e := range events {
			obj := js.Global().Get("Object").New()
			obj.Set("timestamp", e.Timestamp.UnixMilli())
			obj.Set("kind", e.Kind.String())
			obj.Set("touchId", e.TouchID)
			obj.Set("x", e.X)
			obj.Set("y", e.Y)
			arr.SetIndex(i, obj)
		}
		return arr
	}))

	// clearTouchEventLog() -> nil
	// Clears the touch event log.
	js.Global().Set("clearTouchEventLog", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		ClearTouchEventLog()
		return nil
	}))

	// getDevicePixelRatio() -> number
	// Returns the current device pixel ratio.
	js.Global().Set("getDevicePixelRatio", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		return js.ValueOf(getDevicePixelRatio())
	}))

	// getTouchScreenSize() -> {width, height}
	// Returns the current screen size used for touch calculations.
	js.Global().Set("getTouchScreenSize", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		obj := js.Global().Get("Object").New()
		obj.Set("width", touchScreenWidth)
		// touchScreenHeight is not tracked separately, use 0
		obj.Set("height", 0)
		return obj
	}))

	// isSmallScreenMode() -> bool
	// Returns whether the UI is using touch-friendly sizing.
	js.Global().Set("isSmallScreenMode", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		return js.ValueOf(isSmallScreen())
	}))
}

// init sets up the device pixel ratio getter for WASM.
func init() {
	// Override the default DPR getter with the browser's devicePixelRatio
	getDevicePixelRatio = func() float64 {
		dpr := js.Global().Get("devicePixelRatio")
		if dpr.IsUndefined() || dpr.IsNull() {
			return 1.0
		}
		return dpr.Float()
	}
}
