//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// TestDescribeUndoChange covers the snapshot-diff that turns a generic undo step
// into a concrete "param: from → to" detail. Each case is a before/after export
// JSON pair differing in exactly one scalar; describeUndoChange must report the
// changed parameter and its old → new values (omitempty defaults handled).
func TestDescribeUndoChange(t *testing.T) {
	cases := []struct {
		name             string
		before, after    string
		wantParam        string // "@i18nkey" or literal
		wantOld, wantNew string
	}{
		{"bpm", `{"version":1,"bpm":120}`, `{"version":1,"bpm":140}`, "BPM", "120", "140"},
		{"subdiv", `{"version":1,"subdiv":8}`, `{"version":1,"subdiv":16}`, "Subdivision", "8", "16"},
		{
			"node-volume",
			`{"version":1,"nodes":[{"id":1}]}`, // absent volume ⇒ default 1.0 (100%)
			`{"version":1,"nodes":[{"id":1,"volume":0.9}]}`,
			"@" + string(keyVolumeLabel()), "100%", "90%",
		},
		{
			"node-pitch",
			`{"version":1,"nodes":[{"id":2,"pitch":0}]}`,
			`{"version":1,"nodes":[{"id":2,"pitch":3}]}`,
			"@" + string(keyPitchLabel()), "+0", "+3",
		},
		{
			"instrument-volume",
			`{"version":1,"instruments":[{"id":"kick","volume":1.0}]}`,
			`{"version":1,"instruments":[{"id":"kick","volume":0.5}]}`,
			"@" + string(keyVolumeLabel()), "100%", "50%",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, ok := describeUndoChange([]byte(tc.before), []byte(tc.after))
			if !ok {
				t.Fatalf("describeUndoChange found no change")
			}
			if d.param != tc.wantParam || d.old != tc.wantOld || d.new != tc.wantNew {
				t.Fatalf("got {param:%q old:%q new:%q}, want {param:%q old:%q new:%q}",
					d.param, d.old, d.new, tc.wantParam, tc.wantOld, tc.wantNew)
			}
		})
	}
}

// TestDescribeUndoChangeAllParams asserts EVERY user-mutable parameter class
// resolves to a concrete from → to detail (not the generic action label). Each
// case differs in exactly one parameter; the label is "@<i18n key>" (localized)
// or a literal, and the values are formatted in the param's natural unit by the
// SAME formatters the UI renders with (the unified paramFacet registry).
func TestDescribeUndoChangeAllParams(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	i18n.SetLocale(i18n.LocaleEN)
	inst := func(extra string) string {
		return `{"version":1,"instruments":[{"id":"k","volume":1` + extra + `}]}`
	}
	cases := []struct {
		name             string
		before, after    string
		wantParam        string
		wantOld, wantNew string
	}{
		{
			"sample-edit-gain",
			inst(`,"sample_edit":{"gain_db":0}`),
			inst(`,"sample_edit":{"gain_db":3}`),
			"@" + string(i18n.KeySamplerKnobGain), "+0 dB", "+3 dB",
		},
		{
			"sample-edit-start",
			inst(`,"sample_edit":{"start_frac":0}`),
			inst(`,"sample_edit":{"start_frac":0.25}`),
			"@" + string(i18n.KeySamplerKnobStart), "0%", "25%",
		},
		{
			"sample-edit-reverse",
			inst(`,"sample_edit":{"reverse":0}`),
			inst(`,"sample_edit":{"reverse":1}`),
			"@" + string(i18n.KeyReverse), "Off", "On",
		},
		{
			"synth-param-cutoff",
			inst(`,"synth_params":{"filter_cutoff":1000}`),
			inst(`,"synth_params":{"filter_cutoff":2000}`),
			"Cutoff", "1.0 kHz", "2.0 kHz",
		},
		{
			"instrument-pan",
			inst(``),
			inst(`,"pan":0.5`),
			"@" + string(i18n.KeyParamPan), "C", "R50",
		},
		{
			"instrument-delay-send",
			inst(``),
			inst(`,"delay_send":0.3`),
			"@" + string(i18n.KeyParamDelaySend), "0%", "30%",
		},
		{
			"instrument-reverb-send",
			inst(``),
			inst(`,"reverb_send":0.2`),
			"@" + string(i18n.KeyParamReverbSend), "0%", "20%",
		},
		{
			"insert-effect-param",
			inst(`,"effects":[{"type":"delay","enabled":true,"params":{"feedback":0.4}}]`),
			inst(`,"effects":[{"type":"delay","enabled":true,"params":{"feedback":0.6}}]`),
			"Delay · Feedback", "0.4", "0.6",
		},
		{
			"node-type",
			`{"version":1,"nodes":[{"id":1,"type":"regular"}]}`,
			`{"version":1,"nodes":[{"id":1,"type":"mute"}]}`,
			"Type", "regular", "mute",
		},
		{
			"master-eq-band",
			`{"version":1,"eq":{"gains_db":[0,0,0,0,0,0,0,0,0,0],"bands_hz":[[22,44],[44,88],[88,177],[177,354],[354,707],[707,1414],[1414,2828],[2828,5657],[5657,11314],[11314,20000]]}}`,
			`{"version":1,"eq":{"gains_db":[0,0,0,0,0,0,0,0,3,0],"bands_hz":[[22,44],[44,88],[88,177],[177,354],[354,707],[707,1414],[1414,2828],[2828,5657],[5657,11314],[11314,20000]]}}`,
			"EQ 8.0 kHz", "0.0", "+3.0",
		},
		{
			"master-eq-hpf-cutoff",
			`{"version":1,"eq":{"hpf_enabled":true,"hpf_cutoff_hz":80}}`,
			`{"version":1,"eq":{"hpf_enabled":true,"hpf_cutoff_hz":120}}`,
			"HPF", "80 Hz", "120 Hz",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, ok := describeUndoChange([]byte(tc.before), []byte(tc.after))
			if !ok {
				t.Fatalf("describeUndoChange found no change")
			}
			if d.param != tc.wantParam || d.old != tc.wantOld || d.new != tc.wantNew {
				t.Fatalf("got {param:%q old:%q new:%q}, want {param:%q old:%q new:%q}",
					d.param, d.old, d.new, tc.wantParam, tc.wantOld, tc.wantNew)
			}
		})
	}
}

// TestUndoNotificationSamplerEdit reproduces the reported bug: changing a Sampler
// knob and undoing showed the generic "Undo: edit sample" instead of the concrete
// value change. Driven through the real performUndo path with a real descriptor.
func TestUndoNotificationSamplerEdit(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	i18n.SetLocale(i18n.LocaleEN)
	g := newTestGameForUndo(t)
	instID := g.drum.Rows[0].Instrument
	// Global audio sample-edit state is process-wide; clear any leak from a prior
	// test and re-baseline so the diff sees a clean "no edit → +3 dB" change.
	audio.ClearSampleEdit(instID)
	g.undoManager.OnExternalLoad()
	t.Cleanup(func() { audio.ClearSampleEdit(instID) })

	// User raises the gain knob by +3 dB; the production tap records "edit sample".
	audio.SetSampleEdit(instID, audio.SampleEdit{EndFrac: 1, GainDB: 3})
	g.undoManager.record("edit sample")

	g.performUndo()
	if got := g.drum.notifStore.Latest().display(); got != "Undo: Gain +0 dB → +3 dB" {
		t.Fatalf("sampler undo notification = %q, want %q", got, "Undo: Gain +0 dB → +3 dB")
	}
}

// TestDescribeUndoChangeStructuralReturnsFalse: an add/delete (node count change)
// has no single scalar diff, so describeUndoChange returns ok=false and the undo
// notification falls back to the generic action label.
func TestDescribeUndoChangeStructuralReturnsFalse(t *testing.T) {
	before := `{"version":1,"nodes":[{"id":1}]}`
	after := `{"version":1,"nodes":[{"id":1},{"id":2}]}`
	if _, ok := describeUndoChange([]byte(before), []byte(after)); ok {
		t.Fatal("structural change (node added) should not produce a scalar detail")
	}
}

// TestUndoNotificationShowsValueChange is the user-facing requirement: undoing a
// parameter edit shows the concrete "param: from → to" instead of a generic
// "edit node". Driven through the real performUndo path.
func TestUndoNotificationShowsValueChange(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	g := newTestGameForUndo(t)
	n := g.tryAddNode(7, 7, model.NodeTypeRegular)
	g.undoManager.OnExternalLoad() // baseline: node at default (100%) volume

	mn, ok := g.graph.GetNodeByID(n.ID)
	if !ok {
		t.Fatal("node missing")
	}
	p := mn.Params
	p.Volume = 0.9
	g.graph.SetNodeParams(n.ID, p)
	emitNodeParamsChanged(n.ID, p) // records EventNodeParamsChanged

	g.performUndo()
	if got := g.drum.notifStore.Latest().display(); got != "Undo: Volume 100% → 90%" {
		t.Fatalf("undo notification = %q, want %q", got, "Undo: Volume 100% → 90%")
	}

	// The detail notification is stored as key+args (not frozen text), so it
	// re-renders the parameter label AND the prefix in the active locale.
	i18n.SetLocale(i18n.LocaleES)
	if got := g.drum.notifStore.Latest().display(); got != "Deshacer: Volumen 100% → 90%" {
		t.Fatalf("ES undo notification = %q, want %q", got, "Deshacer: Volumen 100% → 90%")
	}
}
