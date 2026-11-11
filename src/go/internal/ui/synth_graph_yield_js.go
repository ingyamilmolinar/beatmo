//go:build js

package ui

import "time"

// On the browser Go shares the page's single thread with the frame loop and
// the audio scheduler: between analysis stages a graph job sleeps briefly so
// the event loop runs (a goroutine blocked in time.Sleep hands the thread back
// to JS; runtime.Gosched would not).
func init() { graphAnalysisYield = func() { time.Sleep(time.Millisecond) } }
