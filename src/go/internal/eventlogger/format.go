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
		// Canonical payload is hooks.BPMPayload; we keep float64/int
		// fallbacks so legacy raw-int callers (none in production after
		// the round-2 cleanup, but useful for tests) still render.
		switch v := p.(type) {
		case hooks.BPMPayload:
			return formatted{tag: "bpm", msg: fmt.Sprintf("BPM = %d", v.BPM)}
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
	hooks.EventImport: func(p any) formatted {
		if v, ok := p.(hooks.ImportPayload); ok {
			if v.Nodes > 0 || v.Rows > 0 {
				return formatted{tag: "import", msg: fmt.Sprintf("import completed (%d nodes, %d rows, %d bytes)", v.Nodes, v.Rows, v.Bytes)}
			}
		}
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
	hooks.EventEQFilterToggled: func(p any) formatted {
		f, _ := p.(hooks.EQFilterPayload)
		state := "off"
		if f.Enabled {
			state = "on"
		}
		return formatted{tag: "audio", msg: fmt.Sprintf("EQ %s %s %s", f.Channel, f.Filter, state)}
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
	hooks.EventRowVolume: func(p any) formatted {
		r, _ := p.(hooks.RowChangePayload)
		return formatted{tag: "row", msg: fmt.Sprintf("row %d volume = %.2f", r.Row, r.Volume)}
	},
	hooks.EventRowPan: func(p any) formatted {
		r, _ := p.(hooks.RowChangePayload)
		return formatted{tag: "row", msg: fmt.Sprintf("row %d pan = %+.2f", r.Row, r.Pan)}
	},
	hooks.EventInsertEffectMoved: func(p any) formatted {
		e, _ := p.(hooks.InsertEffectPayload)
		return formatted{tag: "audio", msg: fmt.Sprintf("insert FX %s moved %d → %d", e.Channel, e.FromSlot, e.ToSlot)}
	},
	hooks.EventInsertEffectToggled: func(p any) formatted {
		e, _ := p.(hooks.InsertEffectPayload)
		state := "off"
		if e.Enabled {
			state = "on"
		}
		return formatted{tag: "audio", msg: fmt.Sprintf("insert FX %s slot=%d %s", e.Channel, e.Slot, state)}
	},
	hooks.EventSendChanged: func(p any) formatted {
		s, _ := p.(hooks.SendPayload)
		return formatted{tag: "audio", msg: fmt.Sprintf("%s send %s = %.2f", s.Kind, s.Channel, s.Value)}
	},
	hooks.EventInstrumentParamsCommitted: func(p any) formatted {
		e, _ := p.(hooks.InstrumentParamPayload)
		return formatted{tag: "audio", msg: fmt.Sprintf("synth params committed %s (recipe=%s)", e.Channel, fallback(e.Recipe, "?"))}
	},
	hooks.EventInstrumentParamsReset: func(p any) formatted {
		e, _ := p.(hooks.InstrumentParamPayload)
		return formatted{tag: "audio", msg: fmt.Sprintf("synth params reset %s", e.Channel)}
	},
	hooks.EventAudioPanelStateChanged: func(p any) formatted {
		s, _ := p.(hooks.AudioPanelStatePayload)
		return formatted{tag: "uistate", msg: fmt.Sprintf("audio panel %s changed", s.Field)}
	},
	hooks.EventInstrumentParamChanged: func(p any) formatted {
		e, _ := p.(hooks.InstrumentParamPayload)
		recipe := e.Recipe
		if recipe == "" {
			recipe = "?"
		}
		return formatted{tag: "audio", msg: fmt.Sprintf("synth param %s (recipe=%s) %s = %.3f", e.Channel, recipe, e.Param, e.Value)}
	},

	// ── Round 2 additions ───────────────────────────────────────────────
	hooks.EventRowColorChanged: func(p any) formatted {
		c, _ := p.(hooks.RowColorPayload)
		return formatted{tag: "row", msg: fmt.Sprintf("row %d color = #%08X", c.Row, c.Color)}
	},
	hooks.EventCustomWAVLoaded: func(p any) formatted {
		w, _ := p.(hooks.CustomWAVPayload)
		verb := "loaded"
		if w.IsUpdate {
			verb = "updated"
		}
		return formatted{tag: "audio", msg: fmt.Sprintf("custom WAV %s id=%s", verb, w.InstrumentID)}
	},
	hooks.EventInstrumentRenamed: func(p any) formatted {
		r, _ := p.(hooks.InstrumentRenamePayload)
		return formatted{tag: "audio", msg: fmt.Sprintf("instrument renamed %s → %s", r.OldID, r.NewID)}
	},
	hooks.EventSceneApplied: func(p any) formatted {
		s, _ := p.(hooks.ScenePayload)
		return formatted{tag: "scene", msg: fmt.Sprintf("scene applied: %s", s.Name)}
	},
	hooks.EventUIStateApplied: func(p any) formatted {
		s, _ := p.(hooks.UIStatePayload)
		return formatted{tag: "uistate", msg: fmt.Sprintf("UI state applied: %s", s.Path)}
	},
	hooks.EventFavoriteToggled: func(p any) formatted {
		f, _ := p.(hooks.FavoritePayload)
		state := "unfavorited"
		if f.IsFavorite {
			state = "favorited"
		}
		return formatted{tag: "favorite", msg: fmt.Sprintf("%s instrument %s", state, f.InstrumentID)}
	},

	// ── Phase 4 synth-recipe lifecycle ──────────────────────────────────
	hooks.EventRecipeSaved: func(p any) formatted {
		r, _ := p.(hooks.RecipePayload)
		inst := r.InstrumentID
		if inst == "" {
			inst = "?"
		}
		return formatted{tag: "audio", msg: fmt.Sprintf("recipe saved %s (from %s)", r.RecipeID, inst)}
	},
	hooks.EventRecipeCreated: func(p any) formatted {
		r, _ := p.(hooks.RecipePayload)
		base := r.BaseRecipe
		if base == "" {
			base = "?"
		}
		name := r.DisplayName
		if name == "" {
			name = r.RecipeID
		}
		return formatted{tag: "audio", msg: fmt.Sprintf("recipe created %s (clone of %s)", name, base)}
	},
	hooks.EventRecipeDeleted: func(p any) formatted {
		r, _ := p.(hooks.RecipePayload)
		return formatted{tag: "audio", msg: fmt.Sprintf("recipe deleted %s", r.RecipeID)}
	},
	hooks.EventKitApplied: func(p any) formatted {
		k, _ := p.(hooks.KitPayload)
		name := k.DisplayName
		if name == "" {
			name = k.KitID
		}
		return formatted{tag: "audio", msg: fmt.Sprintf("kit applied %s (%d members)", name, len(k.Members))}
	},
	hooks.EventSampleSaved: func(p any) formatted {
		s, _ := p.(hooks.SamplePayload)
		return formatted{tag: "audio", msg: fmt.Sprintf("sample saved %s (%d frames)", s.SampleID, s.Frames)}
	},
	hooks.EventSampleCreated: func(p any) formatted {
		s, _ := p.(hooks.SamplePayload)
		name := s.DisplayName
		if name == "" {
			name = s.SampleID
		}
		src := s.SourceID
		if src == "" {
			src = "WAV"
		}
		return formatted{tag: "audio", msg: fmt.Sprintf("sample created %s (from %s)", name, src)}
	},
	hooks.EventSampleReset: func(p any) formatted {
		s, _ := p.(hooks.SamplePayload)
		dst := s.SourceID
		if dst == "" {
			dst = "original sample"
		}
		return formatted{tag: "audio", msg: fmt.Sprintf("sample reset %s (→ %s)", s.SampleID, dst)}
	},
	hooks.EventSampleEditChanged: func(p any) formatted {
		s, _ := p.(hooks.SamplePayload)
		return formatted{tag: "audio", msg: fmt.Sprintf("sample edit changed %s (synth stays source of truth)", s.SampleID)}
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
