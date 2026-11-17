//go:build js && !test

package ui

import (
	"bytes"
	"encoding/base64"
	"runtime/coverage"
	"syscall/js"
)

func init() {
	// flushGoCoverage() returns {meta: base64, counters: base64} or null
	// if the binary was not built with -cover.
	js.Global().Set("flushGoCoverage", jsFn(func(args jsArgs) any {
		var meta bytes.Buffer
		if err := coverage.WriteMeta(&meta); err != nil {
			return nil
		}
		var counters bytes.Buffer
		if err := coverage.WriteCounters(&counters); err != nil {
			return nil
		}
		obj := map[string]interface{}{
			"meta":     base64.StdEncoding.EncodeToString(meta.Bytes()),
			"counters": base64.StdEncoding.EncodeToString(counters.Bytes()),
		}
		return js.ValueOf(obj)
	}))

	// clearGoCoverage() resets coverage counters. Returns true on success.
	js.Global().Set("clearGoCoverage", jsFn(func(args jsArgs) any {
		if err := coverage.ClearCounters(); err != nil {
			return js.ValueOf(false)
		}
		return js.ValueOf(true)
	}))
}
