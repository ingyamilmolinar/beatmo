//go:build js

package ex

import "syscall/js"

type ID string

type Role int

type Channel struct{ name string }

func (c Channel) MarshalText() ([]byte, error) { return []byte(c.name), nil }

type Opaque struct{ n int }

func jsFn(fn func() any) js.Func { return js.Func{} }

func uses(id ID, r Role, c Channel, o Opaque, s string, n int) {
	g := js.Global()
	g.Set("a", id)                                 // finding: named string
	g.Set("b", string(id))                         // ok
	g.Call("f", s, n, r)                           // finding: named int
	_ = js.ValueOf([]any{c})                       // finding: struct in []any
	_ = js.ValueOf(map[string]any{"x": o, "y": s}) // finding: struct in map
	_ = js.ValueOf([]string{s})                    // finding: []string is not walked by ValueOf
	_ = jsFn(func() any {
		if n > 0 {
			return id // ok: jsSafe converts a named basic type
		}
		if n > 1 {
			return c // ok: jsSafe converts a TextMarshaler struct
		}
		return o // finding: jsSafe cannot convert it
	})
}
