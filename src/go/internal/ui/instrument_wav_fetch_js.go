//go:build js

package ui

// No filesystem, and a synchronous fetch would block the UI goroutine; browser WAVs resolve through the
// sample store.
func readWavFileBytes(string) ([]byte, bool) { return nil, false }
func fetchWavURLBytes(string) ([]byte, bool) { return nil, false }
