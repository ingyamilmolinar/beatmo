package eventlogger

import (
	"fmt"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// formatter converts a hook payload into a one-line human message. Returning
// "" suppresses the line. The bracket tag is NOT chosen here — it comes from
// the canonical hooks.ActionRegistry (ActionMeta.Tag), read in writeEvent.
type formatter func(payload any) string

// formatters maps each hooks.Kind to its narrative one-liner. Coverage is
// enforced by coverage_test.go; adding a new Kind without an entry here
// fails CI.
var formatters = map[hooks.Kind]formatter{
	// ── Playback / transport ────────────────────────────────────────────
	hooks.EventPlayStart: func(_ any) string { return "play started" },
	hooks.EventPlayStop:  func(_ any) string { return "play stopped" },
	hooks.EventPaused:    func(_ any) string { return "paused" },
	hooks.EventResumed:   func(_ any) string { return "resumed" },
	hooks.EventSeek: func(p any) string {
		s, _ := p.(hooks.SeekPayload)
		return fmt.Sprintf("seek beats=%d", s.Beats)
	},

	// ── Recording ───────────────────────────────────────────────────────
	hooks.EventRecordStart: func(_ any) string { return "recording started" },
	hooks.EventRecordStop:  func(_ any) string { return "recording stopped" },
	hooks.EventRecordDropped: func(_ any) string {
		// This is a degraded-state signal; we still emit one line so the
		// narrative shows it happened. Repeated drops inside a single
		// session are coalesced (see coalesceKinds).
		return "recording dropped frame(s)"
	},

	// ── Transport / time ────────────────────────────────────────────────
	hooks.EventBPMChange: func(p any) string {
		// Canonical payload is hooks.BPMPayload; we keep float64/int
		// fallbacks so legacy raw-int callers (none in production after
		// the round-2 cleanup, but useful for tests) still render.
		switch v := p.(type) {
		case hooks.BPMPayload:
			return fmt.Sprintf("BPM = %d", v.BPM)
		case float64:
			return fmt.Sprintf("BPM = %.1f", v)
		case int:
			return fmt.Sprintf("BPM = %d", v)
		}
		return "BPM changed"
	},
	hooks.EventSubdivChange: func(p any) string {
		s, _ := p.(hooks.SubdivPayload)
		return fmt.Sprintf("subdiv = %d", s.Subdiv)
	},
	hooks.EventLengthChange: func(p any) string {
		s, _ := p.(hooks.LengthPayload)
		return fmt.Sprintf("length = %d", s.Length)
	},

	// ── Project I/O ─────────────────────────────────────────────────────
	hooks.EventImport: func(p any) string {
		if v, ok := p.(hooks.ImportPayload); ok {
			if v.Nodes > 0 || v.Rows > 0 {
				return fmt.Sprintf("import completed (%d nodes, %d rows, %d bytes)", v.Nodes, v.Rows, v.Bytes)
			}
		}
		return "import completed"
	},
	hooks.EventExport: func(_ any) string { return "export completed" },

	// ── Graph edits ─────────────────────────────────────────────────────
	hooks.EventNodeAdded: func(p any) string {
		n, _ := p.(hooks.NodeEdit)
		typ := n.Type
		if typ == "" {
			typ = "regular"
		}
		return fmt.Sprintf("node added id=%d at (%d,%d) type=%s", n.ID, n.I, n.J, typ)
	},
	hooks.EventNodeDeleted: func(p any) string {
		n, _ := p.(hooks.NodeEdit)
		return fmt.Sprintf("node deleted id=%d at (%d,%d)", n.ID, n.I, n.J)
	},
	hooks.EventNodeMoved: func(p any) string {
		n, _ := p.(hooks.NodeEdit)
		return fmt.Sprintf("node moved id=%d to (%d,%d)", n.ID, n.I, n.J)
	},
	hooks.EventNodeTypeChanged: func(p any) string {
		t, _ := p.(hooks.NodeTypePayload)
		return fmt.Sprintf("node id=%d type %s → %s", t.ID, t.OldType, t.NewType)
	},
	hooks.EventNodeParamsChanged: func(p any) string {
		t, _ := p.(hooks.NodeParamsPayload)
		return fmt.Sprintf("node id=%d params updated", t.ID)
	},
	hooks.EventStartNodeChanged: func(p any) string {
		s, _ := p.(hooks.StartNodePayload)
		return fmt.Sprintf("row %d start node = %d", s.Row, s.ID)
	},
	hooks.EventEdgeAdded: func(p any) string {
		e, _ := p.(hooks.EdgeEdit)
		return fmt.Sprintf("edge added (%d,%d) → (%d,%d)", e.FromI, e.FromJ, e.ToI, e.ToJ)
	},
	hooks.EventEdgeDeleted: func(p any) string {
		e, _ := p.(hooks.EdgeEdit)
		return fmt.Sprintf("edge deleted (%d,%d) → (%d,%d)", e.FromI, e.FromJ, e.ToI, e.ToJ)
	},

	// ── Drum rows ───────────────────────────────────────────────────────
	hooks.EventRowAdded: func(p any) string {
		r, _ := p.(hooks.RowChangePayload)
		return fmt.Sprintf("row %d added (%s)", r.Row, fallback(r.Name, r.Instrument))
	},
	hooks.EventRowDeleted: func(p any) string {
		r, _ := p.(hooks.RowChangePayload)
		return fmt.Sprintf("row %d deleted", r.Row)
	},
	hooks.EventRowInstrumentChange: func(p any) string {
		r, _ := p.(hooks.RowChangePayload)
		return fmt.Sprintf("row %d instrument %s → %s", r.Row, r.OldInstrument, r.Instrument)
	},
	hooks.EventRowMute: func(p any) string {
		r, _ := p.(hooks.RowChangePayload)
		state := "unmuted"
		if r.Mute {
			state = "muted"
		}
		return fmt.Sprintf("row %d %s", r.Row, state)
	},
	hooks.EventRowSolo: func(p any) string {
		r, _ := p.(hooks.RowChangePayload)
		state := "solo off"
		if r.Solo {
			state = "solo on"
		}
		return fmt.Sprintf("row %d %s", r.Row, state)
	},

	// ── Audio settings ──────────────────────────────────────────────────
	hooks.EventMasterVolumeChange: func(p any) string {
		v, _ := p.(hooks.MasterVolumePayload)
		return fmt.Sprintf("master volume = %.2f", v.Volume)
	},
	hooks.EventEQBandChange: func(p any) string {
		b, _ := p.(hooks.EQBandPayload)
		return fmt.Sprintf("EQ %s band %d = %+.1f dB", b.Channel, b.Band, b.GainDB)
	},
	hooks.EventEQFilterToggled: func(p any) string {
		f, _ := p.(hooks.EQFilterPayload)
		state := "off"
		if f.Enabled {
			state = "on"
		}
		return fmt.Sprintf("EQ %s %s %s", f.Channel, f.Filter, state)
	},
	hooks.EventInsertEffectAdded: func(p any) string {
		e, _ := p.(hooks.InsertEffectPayload)
		return fmt.Sprintf("insert FX added %s slot=%d type=%s", e.Channel, e.Slot, e.Type)
	},
	hooks.EventInsertEffectRemoved: func(p any) string {
		e, _ := p.(hooks.InsertEffectPayload)
		return fmt.Sprintf("insert FX removed %s slot=%d", e.Channel, e.Slot)
	},
	hooks.EventInsertEffectParam: func(p any) string {
		e, _ := p.(hooks.InsertEffectPayload)
		return fmt.Sprintf("insert FX %s slot=%d %s = %.2f", e.Channel, e.Slot, e.Param, e.Value)
	},
	hooks.EventRowVolume: func(p any) string {
		r, _ := p.(hooks.RowChangePayload)
		return fmt.Sprintf("row %d volume = %.2f", r.Row, r.Volume)
	},
	hooks.EventRowPan: func(p any) string {
		r, _ := p.(hooks.RowChangePayload)
		return fmt.Sprintf("row %d pan = %+.2f", r.Row, r.Pan)
	},
	hooks.EventInsertEffectMoved: func(p any) string {
		e, _ := p.(hooks.InsertEffectPayload)
		return fmt.Sprintf("insert FX %s moved %d → %d", e.Channel, e.FromSlot, e.ToSlot)
	},
	hooks.EventInsertEffectToggled: func(p any) string {
		e, _ := p.(hooks.InsertEffectPayload)
		state := "off"
		if e.Enabled {
			state = "on"
		}
		return fmt.Sprintf("insert FX %s slot=%d %s", e.Channel, e.Slot, state)
	},
	hooks.EventSendChanged: func(p any) string {
		s, _ := p.(hooks.SendPayload)
		return fmt.Sprintf("%s send %s = %.2f", s.Kind, s.Channel, s.Value)
	},
	hooks.EventInstrumentParamsCommitted: func(p any) string {
		e, _ := p.(hooks.InstrumentParamPayload)
		return fmt.Sprintf("synth params committed %s (recipe=%s)", e.Channel, fallback(e.Recipe, "?"))
	},
	hooks.EventInstrumentParamsReset: func(p any) string {
		e, _ := p.(hooks.InstrumentParamPayload)
		return fmt.Sprintf("synth params reset %s", e.Channel)
	},
	hooks.EventAudioPanelStateChanged: func(p any) string {
		s, _ := p.(hooks.AudioPanelStatePayload)
		return fmt.Sprintf("audio panel %s changed", s.Field)
	},
	hooks.EventInstrumentParamChanged: func(p any) string {
		e, _ := p.(hooks.InstrumentParamPayload)
		recipe := e.Recipe
		if recipe == "" {
			recipe = "?"
		}
		return fmt.Sprintf("synth param %s (recipe=%s) %s = %.3f", e.Channel, recipe, e.Param, e.Value)
	},

	// ── Round 2 additions ───────────────────────────────────────────────
	hooks.EventRowColorChanged: func(p any) string {
		c, _ := p.(hooks.RowColorPayload)
		return fmt.Sprintf("row %d color = #%08X", c.Row, c.Color)
	},
	hooks.EventCustomWAVLoaded: func(p any) string {
		w, _ := p.(hooks.CustomWAVPayload)
		verb := "loaded"
		if w.IsUpdate {
			verb = "updated"
		}
		return fmt.Sprintf("custom WAV %s id=%s", verb, w.InstrumentID)
	},
	hooks.EventInstrumentRenamed: func(p any) string {
		r, _ := p.(hooks.InstrumentRenamePayload)
		return fmt.Sprintf("instrument renamed %s → %s", r.OldID, r.NewID)
	},
	hooks.EventSceneApplied: func(p any) string {
		s, _ := p.(hooks.ScenePayload)
		return fmt.Sprintf("scene applied: %s", s.Name)
	},
	hooks.EventUIStateApplied: func(p any) string {
		s, _ := p.(hooks.UIStatePayload)
		return fmt.Sprintf("UI state applied: %s", s.Path)
	},
	hooks.EventFavoriteToggled: func(p any) string {
		f, _ := p.(hooks.FavoritePayload)
		state := "unfavorited"
		if f.IsFavorite {
			state = "favorited"
		}
		return fmt.Sprintf("%s instrument %s", state, f.InstrumentID)
	},
	hooks.EventLanguageChanged: func(p any) string {
		l, _ := p.(hooks.LanguagePayload)
		return fmt.Sprintf("language %s → %s", l.Old, l.New)
	},

	// ── Phase 4 synth-recipe lifecycle ──────────────────────────────────
	hooks.EventRecipeSaved: func(p any) string {
		r, _ := p.(hooks.RecipePayload)
		inst := r.InstrumentID
		if inst == "" {
			inst = "?"
		}
		return fmt.Sprintf("recipe saved %s (from %s)", r.RecipeID, inst)
	},
	hooks.EventRecipeCreated: func(p any) string {
		r, _ := p.(hooks.RecipePayload)
		base := r.BaseRecipe
		if base == "" {
			base = "?"
		}
		name := r.DisplayName
		if name == "" {
			name = r.RecipeID
		}
		return fmt.Sprintf("recipe created %s (clone of %s)", name, base)
	},
	hooks.EventRecipeDeleted: func(p any) string {
		r, _ := p.(hooks.RecipePayload)
		return fmt.Sprintf("recipe deleted %s", r.RecipeID)
	},
	hooks.EventKitApplied: func(p any) string {
		k, _ := p.(hooks.KitPayload)
		name := k.DisplayName
		if name == "" {
			name = k.KitID
		}
		return fmt.Sprintf("kit applied %s (%d members)", name, len(k.Members))
	},
	hooks.EventSampleSaved: func(p any) string {
		s, _ := p.(hooks.SamplePayload)
		return fmt.Sprintf("sample saved %s (%d frames)", s.SampleID, s.Frames)
	},
	hooks.EventSampleCreated: func(p any) string {
		s, _ := p.(hooks.SamplePayload)
		name := s.DisplayName
		if name == "" {
			name = s.SampleID
		}
		src := s.SourceID
		if src == "" {
			src = "WAV"
		}
		return fmt.Sprintf("sample created %s (from %s)", name, src)
	},
	hooks.EventSampleReset: func(p any) string {
		s, _ := p.(hooks.SamplePayload)
		dst := s.SourceID
		if dst == "" {
			dst = "original sample"
		}
		return fmt.Sprintf("sample reset %s (→ %s)", s.SampleID, dst)
	},
	hooks.EventSampleEditChanged: func(p any) string {
		s, _ := p.(hooks.SamplePayload)
		return fmt.Sprintf("sample edit changed %s (synth stays source of truth)", s.SampleID)
	},

	// ── Undo / redo ─────────────────────────────────────────────────────
	hooks.EventUndo: func(p any) string {
		u, _ := p.(hooks.UndoPayload)
		return fmt.Sprintf("undo: %s", u.Label)
	},
	hooks.EventRedo: func(p any) string {
		u, _ := p.(hooks.UndoPayload)
		return fmt.Sprintf("redo: %s", u.Label)
	},

	// ── Interaction telemetry (Layer-B) ─────────────────────────────────
	hooks.EventUITap: func(p any) string {
		u, _ := p.(hooks.UITapPayload)
		label := u.Label
		if label == "" {
			label = u.Icon
		}
		if label == "" {
			return "" // unlabeled control: suppress
		}
		return fmt.Sprintf("tap %q", label)
	},
	hooks.EventPopupOpened: func(p any) string {
		v, _ := p.(hooks.PopupPayload)
		return fmt.Sprintf("opened %s", v.ID)
	},
	hooks.EventPopupClosed: func(p any) string {
		v, _ := p.(hooks.PopupPayload)
		if v.Reason != "" {
			return fmt.Sprintf("closed %s (%s)", v.ID, v.Reason)
		}
		return fmt.Sprintf("closed %s", v.ID)
	},
	hooks.EventScroll: func(p any) string {
		s, _ := p.(hooks.ScrollPayload)
		if s.Surface == "" {
			return "scrolled"
		}
		return fmt.Sprintf("scrolled %s", s.Surface)
	},
	hooks.EventSearchChanged: func(p any) string {
		s, _ := p.(hooks.SearchPayload)
		if s.Query == "" {
			return fmt.Sprintf("search %s cleared", s.Surface)
		}
		return fmt.Sprintf("search %s = %q", s.Surface, s.Query)
	},
	hooks.EventTextCommitted: func(p any) string {
		t, _ := p.(hooks.TextPayload)
		return fmt.Sprintf("%s = %q", t.Field, t.Value)
	},
	hooks.EventViewModeChanged: func(p any) string {
		v, _ := p.(hooks.ViewModePayload)
		return fmt.Sprintf("view → %s", v.Mode)
	},

	// ── Verbose (only emitted when Options.Verbose is true) ─────────────
	hooks.EventCameraPan: func(p any) string {
		c, _ := p.(hooks.CameraPanPayload)
		return fmt.Sprintf("pan dx=%.1f dy=%.1f", c.DX, c.DY)
	},
	hooks.EventCameraZoom: func(p any) string {
		c, _ := p.(hooks.CameraZoomPayload)
		return fmt.Sprintf("zoom factor=%.2f", c.Factor)
	},
	hooks.EventDragProgress: func(p any) string {
		d, _ := p.(hooks.DragProgressPayload)
		return fmt.Sprintf("drag node=%d at (%d,%d)", d.NodeID, d.I, d.J)
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
