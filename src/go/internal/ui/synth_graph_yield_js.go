//go:build js

package ui

import "time"

// A goroutine blocked in time.Sleep hands the page's single thread back to JS; runtime.Gosched would not.
func init() { graphAnalysisYield = func() { time.Sleep(time.Millisecond) } }
