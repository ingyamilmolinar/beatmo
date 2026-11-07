//go:build js && !test

package ui

import "syscall/js"

func (g *Game) initJSTouchDebug() {
	// setTouchDebug(enabled)
	js.Global().Set("setTouchDebug", jsFn(func(args jsArgs) any {
		if args.Len() < 1 {
			return nil
		}
		SetTouchDebugEnabled(args.Bool(0))
		return nil
	}))

	// touchDebugState() -> {count, touches:[{id,x,y}], lastGesture, lastPos, dpr, enabled}
	js.Global().Set("touchDebugState", jsFn(func(args jsArgs) any {
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

	// touchEventLog() -> the last 50 [{timestamp,kind,touchId,x,y}]
	js.Global().Set("touchEventLog", jsFn(func(args jsArgs) any {
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

	js.Global().Set("clearTouchEventLog", jsFn(func(args jsArgs) any {
		ClearTouchEventLog()
		return nil
	}))

	js.Global().Set("getDevicePixelRatio", jsFn(func(args jsArgs) any {
		return js.ValueOf(getDevicePixelRatio())
	}))

	// getTouchScreenSize() -> {width, height}
	js.Global().Set("getTouchScreenSize", jsFn(func(args jsArgs) any {
		obj := js.Global().Get("Object").New()
		obj.Set("width", touchScreenWidth)
		// touchScreenHeight is not tracked separately, use 0
		obj.Set("height", 0)
		return obj
	}))

	// isSmallScreenMode() -> bool: touch-friendly sizing.
	js.Global().Set("isSmallScreenMode", jsFn(func(args jsArgs) any {
		return js.ValueOf(Profile().IsMobile())
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
