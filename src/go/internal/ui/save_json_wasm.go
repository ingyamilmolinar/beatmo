//go:build js && !test

package ui

import (
	"syscall/js"
)

func saveJSONDefault(name string, data []byte) error {
	g := js.Global()
	fn := g.Get("downloadJSON")
	if fn.Truthy() {
		fn.Invoke(name, string(data))
		return nil
	}
	// Fallback: construct Blob and click a temporary anchor.
	uint8Array := g.Get("Uint8Array")
	blobCtor := g.Get("Blob")
	url := g.Get("URL")
	doc := g.Get("document")
	if !uint8Array.Truthy() || !blobCtor.Truthy() || !url.Truthy() || !doc.Truthy() {
		return nil
	}
	buf := uint8Array.New(len(data))
	js.CopyBytesToJS(buf, data)
	// Blob([buf], {type:'application/json'})
	opts := js.Global().Get("Object").New()
	opts.Set("type", "application/json")
	blob := blobCtor.New([]interface{}{buf}, opts)
	href := url.Call("createObjectURL", blob)
	a := doc.Call("createElement", "a")
	a.Set("href", href)
	a.Set("download", name)
	doc.Get("body").Call("appendChild", a)
	a.Call("click")
	// cleanup
	var revoke js.Func
	revoke = js.FuncOf(func(this js.Value, args []js.Value) any {
		url.Call("revokeObjectURL", href)
		a.Call("remove")
		revoke.Release()
		return nil
	})
	if st := g.Get("setTimeout"); st.Truthy() {
		st.Invoke(revoke, 100)
	} else {
		revoke.Invoke(js.Undefined(), nil)
	}
	return nil
}
