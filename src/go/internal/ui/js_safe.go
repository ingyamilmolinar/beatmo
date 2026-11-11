package ui

import (
	"encoding"
	"reflect"
)

// jsSafe returns v with every value of a NAMED basic type (instrument.ID,
// a typed int enum, …) replaced by the plain type of the same kind, and every
// struct with a MarshalText (channel.ID) by its text, walking
// []any and map[string]any. syscall/js.ValueOf accepts only the plain types
// and panics on a named one; the compiler cannot see that because ValueOf
// takes any. Every JS export returns through jsFn, which passes its result
// through here. A container is copied only when one of its elements changes,
// so the common case (nothing named) allocates nothing.
func jsSafe(v any) any {
	out, _ := jsSafeValue(v)
	return out
}

func jsSafeValue(v any) (any, bool) {
	switch t := v.(type) {
	case nil, string, bool, int, int64, float64, float32, uint32:
		return v, false
	case []any:
		var out []any
		for i, e := range t {
			s, changed := jsSafeValue(e)
			if changed && out == nil {
				out = append([]any(nil), t...)
			}
			if out != nil {
				out[i] = s
			}
		}
		if out == nil {
			return v, false
		}
		return out, true
	case map[string]any:
		var out map[string]any
		for k, e := range t {
			s, changed := jsSafeValue(e)
			if !changed {
				continue
			}
			if out == nil {
				out = make(map[string]any, len(t))
				for k2, e2 := range t {
					out[k2] = e2
				}
			}
			out[k] = s
		}
		if out == nil {
			return v, false
		}
		return out, true
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Struct {
		if m, ok := v.(encoding.TextMarshaler); ok {
			if b, err := m.MarshalText(); err == nil {
				return string(b), true
			}
		}
		return v, false
	}
	if rv.Type().PkgPath() == "" {
		return v, false // a plain (unnamed) type: ValueOf decides
	}
	switch rv.Kind() {
	case reflect.String:
		return rv.String(), true
	case reflect.Bool:
		return rv.Bool(), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int(), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return rv.Uint(), true
	case reflect.Float32, reflect.Float64:
		return rv.Float(), true
	}
	return v, false
}
