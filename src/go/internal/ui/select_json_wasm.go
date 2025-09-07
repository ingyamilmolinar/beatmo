//go:build js && !test

package ui

import "syscall/js"

func selectJSON() ([]byte, error) {
    g := js.Global()
    open := g.Get("openJSONFile")
    if !open.Truthy() {
        return nil, nil
    }
    var data []byte
    done := make(chan struct{})
    p := open.Invoke()
    if p.Truthy() {
        then := js.FuncOf(func(this js.Value, args []js.Value) any {
            if len(args) > 0 {
                s := args[0].String()
                data = []byte(s)
            }
            close(done)
            return nil
        })
        p.Call("then", then, then)
        <-done
    }
    return data, nil
}

