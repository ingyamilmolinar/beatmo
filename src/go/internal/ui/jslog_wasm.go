//go:build js && !test

package ui

import (
	"fmt"
	"syscall/js"
)

func jsLog(format string, args ...interface{}) {
	msg := fmt.Sprintf("[IMPORT] "+format, args...)
	js.Global().Get("console").Call("log", msg)
}
