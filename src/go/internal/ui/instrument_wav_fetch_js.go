//go:build js

package ui

// Browser build: there is no local-path filesystem, and a synchronous URL fetch
// would block the UI goroutine (fetch is async). Both are follow-ups (an async
// pre-fetch step). On the browser, WAV instruments resolve via the
// localStorage/IndexedDB sample store (the localstorage WavSource).
func readWavFileBytes(string) ([]byte, bool) { return nil, false }
func fetchWavURLBytes(string) ([]byte, bool) { return nil, false }
