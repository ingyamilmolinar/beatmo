//go:build js && !test

package ui

import (
	"math"
	"syscall/js"
)

// threeStageLatencyJSObject converts a ScheduleMetricsSnapshot to a JS
// object exposing all three latency stages. NaN values are converted to
// JS null so callers can use idiomatic `?? fallback` semantics. Returns
// JS null entirely if no observations have been recorded yet.
func threeStageLatencyJSObject(s ScheduleMetricsSnapshot) js.Value {
	if s.Count == 0 && s.BridgeCount == 0 && s.SeqFireCount == 0 {
		return js.Null()
	}
	obj := js.Global().Get("Object").New()
	obj.Set("count", float64(s.Count))
	obj.Set("overdue", float64(s.Overdue))
	obj.Set("smallLeadCount", float64(s.SmallLeadCount))

	// Stage A: sequencer-fire-late.
	a := js.Global().Get("Object").New()
	a.Set("count", float64(s.SeqFireCount))
	a.Set("avg", nanToNull(s.SeqFireAvg))
	a.Set("max", nanToNull(s.SeqFireMax))
	a.Set("p50", nanToNull(s.SeqFireP50))
	a.Set("p90", nanToNull(s.SeqFireP90))
	a.Set("p99", nanToNull(s.SeqFireP99))
	a.Set("stddev", nanToNull(s.SeqFireStdDev))
	obj.Set("seqFireLate", a)

	// Stage B: bridge latency (enqueue → dispatch).
	b := js.Global().Get("Object").New()
	b.Set("count", float64(s.BridgeCount))
	b.Set("avg", nanToNull(s.BridgeAvg))
	b.Set("max", nanToNull(s.BridgeMax))
	b.Set("p50", nanToNull(s.BridgeP50))
	b.Set("p90", nanToNull(s.BridgeP90))
	b.Set("p99", nanToNull(s.BridgeP99))
	b.Set("stddev", nanToNull(s.BridgeStdDev))
	obj.Set("bridge", b)

	// Stage C: schedule lead (dispatch → audio fire).
	c := js.Global().Get("Object").New()
	c.Set("count", float64(s.Count))
	c.Set("min", nanToNull(s.MinLead))
	c.Set("max", nanToNull(s.MaxLead))
	c.Set("avg", nanToNull(s.AvgLead))
	c.Set("p50", nanToNull(s.LeadP50))
	c.Set("p90", nanToNull(s.LeadP90))
	c.Set("p99", nanToNull(s.LeadP99))
	c.Set("stddev", nanToNull(s.LeadStdDev))
	c.Set("avgLag", s.AvgLag)
	c.Set("maxLag", s.MaxLag)
	c.Set("lagP90", nanToNull(s.LagP90))
	c.Set("lagP99", nanToNull(s.LagP99))
	obj.Set("lead", c)

	obj.Set("e2eP99", nanToNull(s.E2EP99))
	return obj
}

func nanToNull(v float64) interface{} {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return js.Null()
	}
	return v
}
