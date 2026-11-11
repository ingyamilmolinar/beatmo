//go:build js

package brk

// broken does not type-check under js: loading must tolerate it.
var broken int = "not an int"
