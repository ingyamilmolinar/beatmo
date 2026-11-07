//go:build js && !test

package ui

import "syscall/js"

var (
	kbProxyFocusFn   js.Value
	kbProxyBlurFn    js.Value
	kbProxyDrainFn   js.Value
	kbRegisterRectFn js.Value
	kbClearRectsFn   js.Value
	kbEscFn          js.Func
	kbArrowLeftFn    js.Func
	kbArrowRightFn   js.Func
	kbHomeFn         js.Func
	kbEndFn          js.Func
	kbInitialized    bool
)

func softKeyboardInit() {
	g := js.Global()
	kbProxyFocusFn = g.Get("_kbProxyFocus")
	kbProxyBlurFn = g.Get("_kbProxyBlur")
	kbProxyDrainFn = g.Get("_kbProxyDrain")
	kbRegisterRectFn = g.Get("_kbRegisterFocusRect")
	kbClearRectsFn = g.Get("_kbClearFocusRects")
	kbInitialized = kbProxyFocusFn.Truthy() && kbProxyBlurFn.Truthy() && kbProxyDrainFn.Truthy()
	// The hidden proxy holds focus while a field is active, so the canvas never sees Esc.
	kbEscFn = js.FuncOf(func(js.Value, []js.Value) any {
		SignalSoftKeyboardEscape()
		return nil
	})
	g.Set("_kbProxyEscapeGo", kbEscFn)
	// The proxy holds focus, so the canvas never sees the arrows; without this, caret movement was dead on WASM.
	kbArrowLeftFn = js.FuncOf(func(js.Value, []js.Value) any {
		SignalSoftKeyboardArrowLeft()
		return nil
	})
	g.Set("_kbProxyArrowLeftGo", kbArrowLeftFn)
	kbArrowRightFn = js.FuncOf(func(js.Value, []js.Value) any {
		SignalSoftKeyboardArrowRight()
		return nil
	})
	g.Set("_kbProxyArrowRightGo", kbArrowRightFn)
	// Delete needs no bridge: the keydown handler buffers it as the '\x7f' sentinel.
	kbHomeFn = js.FuncOf(func(js.Value, []js.Value) any {
		SignalSoftKeyboardHome()
		return nil
	})
	g.Set("_kbProxyHomeGo", kbHomeFn)
	kbEndFn = js.FuncOf(func(js.Value, []js.Value) any {
		SignalSoftKeyboardEnd()
		return nil
	})
	g.Set("_kbProxyEndGo", kbEndFn)
	mobileInputInit()
}

// The proxy buffers every keystroke through the browser's own events, so a Backspace/Enter that goes down and up
// between two ticks is never lost (as Ebiten's level check loses it). Never gate on platform: hardware keyboards need it too.
func softKeyboardShow(inputmode string) {
	if !kbInitialized {
		return
	}
	kbProxyFocusFn.Invoke(inputmode)
}

// softKeyboardHide blurs the proxy input to dismiss the soft keyboard.
func softKeyboardHide() {
	if !kbInitialized {
		return
	}
	kbProxyBlurFn.Invoke()
}

// '\b' is Backspace, '\n' Enter.
func softKeyboardDrainChars() []rune {
	if !kbInitialized {
		return nil
	}
	arr := kbProxyDrainFn.Invoke()
	if !arr.Truthy() {
		return nil
	}
	length := arr.Length()
	if length == 0 {
		return nil
	}
	runes := make([]rune, 0, length)
	for i := 0; i < length; i++ {
		s := arr.Index(i).String()
		for _, r := range s {
			runes = append(runes, r)
		}
	}
	return runes
}

// softKeyboardActive reports whether the proxy input is currently focused.
func softKeyboardActive() bool {
	if !kbInitialized {
		return false
	}
	doc := js.Global().Get("document")
	active := doc.Get("activeElement")
	if !active.Truthy() {
		return false
	}
	return active.Get("id").String() == "beatmo-kb-proxy"
}

// The JS layer focuses the proxy synchronously inside the trusted touchend, as browsers require.
func softKeyboardRegisterRect(id string, x, y, w, h int, inputmode string) {
	if !kbRegisterRectFn.Truthy() {
		return
	}
	kbRegisterRectFn.Invoke(id, x, y, w, h, inputmode)
}

// softKeyboardClearRects removes all registered focusable rects.
func softKeyboardClearRects() {
	if !kbClearRectsFn.Truthy() {
		return
	}
	kbClearRectsFn.Invoke()
}
