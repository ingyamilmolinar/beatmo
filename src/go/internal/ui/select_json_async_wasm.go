//go:build js && !test

package ui

import "syscall/js"

// selectJSONAsync invokes the browser file picker and calls cb with the file
// contents when ready. It must be called synchronously from a user gesture.
func selectJSONAsync(cb func([]byte, error)) {
    g := js.Global()
    open := g.Get("openJSONFile")
    jsLog("selectJSONAsync called; openJSONFile present=%v", open.Truthy())
    if open.Truthy() {
        p := open.Invoke()
        jsLog("openJSONFile invoked; promise=%v", p.Truthy())
        if p.Truthy() {
            then := js.FuncOf(func(this js.Value, args []js.Value) any {
                jsLog("openJSONFile.then called; args=%d", len(args))
                if len(args) > 0 {
                    cb([]byte(args[0].String()), nil)
                } else {
                    cb(nil, nil)
                }
                return nil
            })
            g.Set("__importThen", then)
            p.Call("then", then, then)
            return
        }
    }
    // Fallback: build file input directly from Go
    doc := g.Get("document")
    if !doc.Truthy() {
        jsLog("document not available; aborting import")
        cb(nil, nil)
        return
    }
    input := doc.Call("createElement", "input")
    input.Set("type", "file")
    input.Set("accept", "application/json,.json")
    jsLog("created file input; attaching change listener")
    change := js.FuncOf(func(this js.Value, args []js.Value) any {
        files := input.Get("files")
        if !files.Truthy() || files.Length() == 0 {
            jsLog("no file selected")
            cb(nil, nil)
            return nil
        }
        f := files.Index(0)
        jsLog("reading file via File.text(): name=%s size=%v", f.Get("name").String(), f.Get("size").Int())
        prom := f.Call("text")
        then := js.FuncOf(func(this js.Value, args []js.Value) any {
            if len(args) > 0 {
                cb([]byte(args[0].String()), nil)
            } else {
                cb(nil, nil)
            }
            // cleanup
            input.Call("remove")
            return nil
        })
        g.Set("__importTextThen", then)
        prom.Call("then", then, then)
        return nil
    })
    g.Set("__importOnChange", change)
    input.Call("addEventListener", "change", change)
    doc.Get("body").Call("appendChild", input)
    jsLog("clicking file input (fallback)")
    input.Call("click")
}
