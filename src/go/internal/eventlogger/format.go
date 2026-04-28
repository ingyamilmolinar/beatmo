package eventlogger

import (
	"fmt"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// formatted is the shape returned by every formatter: a tag (printed in its
// own column by the underlying logger) and a one-line human message.
type formatted struct {
	tag string
	msg string
}

// formatter converts a hook payload into a formatted INFO line. Returning
// formatted{} (zero value) suppresses the line — useful for events whose
// payloads are intentionally informational only.
type formatter func(payload any) formatted

// formatters maps each hooks.Kind to its narrative one-liner. Coverage is
// enforced by coverage_test.go; adding a new Kind without an entry here
// fails CI.
var formatters = map[hooks.Kind]formatter{
	// ── Playback / transport ────────────────────────────────────────────
	hooks.EventPlayStart: func(_ any) formatted {
		return formatted{tag: "play", msg: "play started"}
	},
	hooks.EventPlayStop: func(_ any) formatted {
		return formatted{tag: "play", msg: "play stopped"}
	},
	hooks.EventPaused: func(_ any) formatted {
		return formatted{tag: "play", msg: "paused"}
	},
	hooks.EventResumed: func(_ any) formatted {
		return formatted{tag: "play", msg: "resumed"}
	},
	hooks.EventSeek: func(p any) formatted {
		s, _ := p.(hooks.SeekPayload)
		return formatted{tag: "seek", msg: fmt.Sprintf("seek beats=%d", s.Beats)}
	},

	// ── Recording ───────────────────────────────────────────────────────
	hooks.EventRecordStart: func(_ any) formatted {
		return formatted{tag: "record", msg: "recording started"}
	},
	hooks.EventRecordStop: func(_ any) formatted {
		return formatted{tag: "record", msg: "recording stopped"}
	},
	hooks.EventRecordDropped: func(_ any) formatted {
		// This is a degraded-state signal; we still emit one line so the
		// narrative shows it happened. Repeated drops inside a single
		// session are coalesced (see coalesceKinds).
		return formatted{tag: "record", msg: "recording dropped frame(s)"}
	},

	// ── Transport / time ────────────────────────────────────────────────
	hooks.EventBPMChange: func(p any) formatted {
		// Payload may be a float64 BPM (the convention used in
		// internal/ui/event_helpers.go). Print one decimal so 90.5 BPM
		// values aren't truncated.
		switch v := p.(type) {
		case float64:
			return formatted{tag: "bpm", msg: fmt.Sprintf("BPM = %.1f", v)}
		case int:
			return formatted{tag: "bpm", msg: fmt.Sprintf("BPM = %d", v)}
		}
		return formatted{tag: "bpm", msg: "BPM changed"}
	},
	hooks.EventSubdivChange: func(p any) formatted {
		s, _ := p.(hooks.SubdivPayload)
		return formatted{tag: "subdiv", msg: fmt.Sprintf("subdiv = %d", s.Subdiv)}
	},
	hooks.EventLengthChange: func(p any) formatted {
		s, _ := p.(hooks.LengthPayload)
		return formatted{tag: "length", msg: fmt.Sprintf("length = %d", s.Length)}
	},

	// ── Project I/O ─────────────────────────────────────────────────────
	hooks.EventImport: func(_ any) formatted {
		return formatted{tag: "import", msg: "import completed"}
	},
	hooks.EventExport: func(_ any) formatted {
		return formatted{tag: "export", msg: "export completed"}
	},

	// ── Graph edits ─────────────────────────────────────────────────────
	hooks.EventNodeAdded: func(p any) formatted {
		n, _ := p.(hooks.NodeEdit)
		typ := n.Type
		if typ == "" {
			typ = "regular"
		}
		return formatted{tag: "node", msg: fmt.Sprintf("node added id=%d at (%d,%d) type=%s", n.ID, n.I, n.J, typ)}
	},
	hooks.EventNodeDeleted: func(p any) formatted {
		n, _ := p.(hooks.NodeEdit)
		return formatted{tag: "node", msg: fmt.Sprintf("node deleted id=%d at (%d,%d)", n.ID, n.I, n.J)}
	},
	hooks.EventNodeMoved: func(p any) formatted {
		n, _ := p.(hooks.NodeEdit)
		return formatted{tag: "node", msg: fmt.Sprintf("node moved id=%d to (%d,%d)", n.ID, n.I, n.J)}
	},
	hooks.EventNodeTypeChanged: func(p any) formatted {
		t, _ := p.(hooks.NodeTypePayload)
		return formatted{tag: "node", msg: fmt.Sprintf("node id=%d type %s → %s", t.ID, t.OldType, t.NewType)}
	},
	hooks.EventNodeParamsChanged: func(p any) formatted {
		t, _ := p.(hooks.NodeParamsPayload)
		return formatted{tag: "node", msg: fmt.Sprintf("node id=%d params updated", t.ID)}
	},
	hooks.EventStartNodeChanged: func(p any) formatted {
		s, _ := p.(hooks.StartNodePayload)
		return formatted{tag: "node", msg: fmt.Sprintf("row %d start node = %d", s.Row, s.ID)}
	},
	hooks.EventEdgeAdded: func(p any) formatted {
		e, _ := p.(hooks.EdgeEdit)
		return formatted{tag: "edge", msg: fmt.Sprintf("edge added (%d,%d) → (%d,%d)", e.FromI, e.FromJ, e.ToI, e.ToJ)}
	},
	hooks.EventEdgeDeleted: func(p any) formatted {
		e, _ := p.(hooks.EdgeEdit)
		return formatted{tag: "edge", msg: fmt.Sprintf("edge deleted (%d,%d) → (%d,%d)", e.FromI, e.FromJ, e.ToI, e.ToJ)}
	},

	// ── Drum rows ───────────────────────────────────────────────────────
	hooks.EventRowAdded: func(p any) formatted {
		r, _ := p.(hooks.RowChangePayload)
		return formatted{tag: "row", msg: fmt.Sprintf("row %d added (%s)", r.Row, fallback(r.Name, r.Instrument))}
	},
	hooks.EventRowDeleted: func(p any) formatted {
		r, _ := p.(hooks.RowChangePayload)
		return formatted{tag: "row", msg: fmt.Sprintf("row %d deleted", r.Row)}
	},
	hooks.EventRowInstrumentChange: func(p any) formatted {
		r, _ := p.(hooks.RowChangePayload)
		return formatted{tag: "row", msg: fmt.Sprintf("row %d instrument %s → %s", r.Row, r.OldInstrument, r.Instrument)}
	},
	hooks.EventRowMute: func(p any) formatted {
		r, _ := p.(hooks.RowChangePayload)
		state := "unmuted"
		if r.Mute {
			state = "muted"
		}
		return formatted{tag: "row", msg: fmt.Sprintf("row %d %s", r.Row, state)}
	},
	hooks.EventRowSolo: func(p any) formatted {
		r, _ := p.(hooks.RowChangePayload)
		state := "solo off"
		if r.Solo {
			state = "solo on"
		}
		return formatted{tag: "row", msg: fmt.Sprintf("row %d %s", r.Row, state)}
	},

	// ── Audio settings ──────────────────────────────────────────────────
	hooks.EventMasterVolumeChange: func(p any) formatted {
		v, _ := p.(hooks.MasterVolumePayload)
		return formatted{tag: "audio", msg: fmt.Sprintf("master volume = %.2f", v.Volume)}
	},
	hooks.EventEQBandChange: func(p any) formatted {
		b, _ := p.(hooks.EQBandPayload)
		return formatted{tag: "audio", msg: fmt.Sprintf("EQ %s band %d = %+.1f dB", b.Channel, b.Band, b.GainDB)}
	},
	hooks.EventInsertEffectAdded: func(p any) formatted {
		e, _ := p.(hooks.InsertEffectPayload)
		return formatted{tag: "audio", msg: fmt.Sprintf("insert FX added %s slot=%d type=%s", e.Channel, e.Slot, e.Type)}
	},
	hooks.EventInsertEffectRemoved: func(p any) formatted {
		e, _ := p.(hooks.InsertEffectPayload)
		return formatted{tag: "audio", msg: fmt.Sprintf("insert FX removed %s slot=%d", e.Channel, e.Slot)}
	},
	hooks.EventInsertEffectParam: func(p any) formatted {
		e, _ := p.(hooks.InsertEffectPayload)
		return formatted{tag: "audio", msg: fmt.Sprintf("insert FX %s slot=%d %s = %.2f", e.Channel, e.Slot, e.Param, e.Value)}
	},

	// ── Verbose (only emitted when Options.Verbose is true) ─────────────
	hooks.EventCameraPan: func(p any) formatted {
		c, _ := p.(hooks.CameraPanPayload)
		return formatted{tag: "camera", msg: fmt.Sprintf("pan dx=%.1f dy=%.1f", c.DX, c.DY)}
	},
	hooks.EventCameraZoom: func(p any) formatted {
		c, _ := p.(hooks.CameraZoomPayload)
		return formatted{tag: "camera", msg: fmt.Sprintf("zoom factor=%.2f", c.Factor)}
	},
	hooks.EventDragProgress: func(p any) formatted {
		d, _ := p.(hooks.DragProgressPayload)
		return formatted{tag: "drag", msg: fmt.Sprintf("drag node=%d at (%d,%d)", d.NodeID, d.I, d.J)}
	},
}

// lookupFormatter returns the formatter for k, or (nil, false) if none.
func lookupFormatter(k hooks.Kind) (formatter, bool) {
	f, ok := formatters[k]
	return f, ok
}

func fallback(primary, alt string) string {
	if primary != "" {
		return primary
	}
	return alt
}
