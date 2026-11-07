//go:build test

package ui

import (
	"encoding/json"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/schema"
)

// Each pair differs in exactly one scalar, reported as "param: from → to" (omitempty defaults included).
func TestDescribeUndoChange(t *testing.T) {
	cases := []struct {
		name             string
		before, after    string
		wantParam        string // "@i18nkey" or literal
		wantOld, wantNew string
	}{
		{"bpm", `{"schema_version":"0.0.1","bpm":120}`, `{"schema_version":"0.0.1","bpm":140}`, "BPM", "120", "140"},
		{"subdiv", `{"schema_version":"0.0.1","subdiv":8}`, `{"schema_version":"0.0.1","subdiv":16}`, "Subdivision", "8", "16"},
		{
			"node-volume",
			`{"schema_version":"0.0.1","nodes":[{"id":1}]}`, // absent volume ⇒ default 1.0 (100%)
			`{"schema_version":"0.0.1","nodes":[{"id":1,"volume":0.9}]}`,
			"@" + string(keyVolumeLabel()), "100%", "90%",
		},
		{
			"node-pitch",
			`{"schema_version":"0.0.1","nodes":[{"id":2,"pitch":0}]}`,
			`{"schema_version":"0.0.1","nodes":[{"id":2,"pitch":3}]}`,
			"@" + string(keyPitchLabel()), "+0", "+3",
		},
		{
			"instrument-volume",
			`{"schema_version":"0.0.1","instruments":[{"id":"kick","volume":1.0}]}`,
			`{"schema_version":"0.0.1","instruments":[{"id":"kick","volume":0.5}]}`,
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

// Every user-mutable parameter class gets a concrete detail, formatted by the SAME paramFacet formatters the UI uses.
func TestDescribeUndoChangeAllParams(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	i18n.SetLocale(i18n.LocaleEN)
	inst := func(extra string) string {
		return `{"schema_version":"0.0.1","instruments":[{"id":"k","volume":1` + extra + `}]}`
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
			"insert-effect-param",
			inst(`,"effects":[{"type":"delay","enabled":true,"params":{"feedback":0.4}}]`),
			inst(`,"effects":[{"type":"delay","enabled":true,"params":{"feedback":0.6}}]`),
			"Delay · Feedback", "0.4", "0.6",
		},
		{
			"node-type",
			`{"schema_version":"0.0.1","nodes":[{"id":1,"type":"regular"}]}`,
			`{"schema_version":"0.0.1","nodes":[{"id":1,"type":"mute"}]}`,
			"Type", "regular", "mute",
		},
		{
			"master-eq-band",
			`{"schema_version":"0.0.1","eq":{"gains_db":[0,0,0,0,0,0,0,0,0,0],"bands_hz":[[22,44],[44,88],[88,177],[177,354],[354,707],[707,1414],[1414,2828],[2828,5657],[5657,11314],[11314,20000]]}}`,
			`{"schema_version":"0.0.1","eq":{"gains_db":[0,0,0,0,0,0,0,0,3,0],"bands_hz":[[22,44],[44,88],[88,177],[177,354],[354,707],[707,1414],[1414,2828],[2828,5657],[5657,11314],[11314,20000]]}}`,
			"EQ 8.0 kHz", "0.0", "+3.0",
		},
		{
			"master-eq-hpf-cutoff",
			`{"schema_version":"0.0.1","eq":{"hpf_enabled":true,"hpf_cutoff_hz":80}}`,
			`{"schema_version":"0.0.1","eq":{"hpf_enabled":true,"hpf_cutoff_hz":120}}`,
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

// A Sampler knob undo names the value change, not "edit sample". Through the real performUndo.
func TestUndoNotificationSamplerEdit(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	i18n.SetLocale(i18n.LocaleEN)
	g := newTestGameForUndo(t)
	instID := g.drum.Rows[0].Instrument
	// Sample-edit state is process-wide; cleared and re-baselined so the diff sees "no edit → +3 dB".
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

// An add or delete has no single scalar diff, so the notification uses the generic action label.
func TestDescribeUndoChangeStructuralReturnsFalse(t *testing.T) {
	before := `{"schema_version":"0.0.1","nodes":[{"id":1}]}`
	after := `{"schema_version":"0.0.1","nodes":[{"id":1},{"id":2}]}`
	if _, ok := describeUndoChange([]byte(before), []byte(after)); ok {
		t.Fatal("structural change (node added) should not produce a scalar detail")
	}
}

// Through the real performUndo path.
func TestUndoNotificationShowsValueChange(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	g := newTestGameForUndo(t)
	warmCircuitForTest(t, g)
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

	// Stored as key+args, so label and prefix re-render in the active locale.
	i18n.SetLocale(i18n.LocaleES)
	if got := g.drum.notifStore.Latest().display(); got != "Deshacer: Volumen 100% → 90%" {
		t.Fatalf("ES undo notification = %q, want %q", got, "Deshacer: Volumen 100% → 90%")
	}
}

func TestDescribeUndoChangeNamesTheDocumentKnob(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	i18n.SetLocale(i18n.LocaleEN)
	reg := audio.BuiltinModuleRegistry()
	base, ok := audio.ShippedDoc(roster.Modular)
	if !ok {
		t.Fatal("no shipped modular instrument")
	}
	file := func(cutoff float64) []byte {
		d := base
		if _, has := d.Stages["filter"]; !has {
			var err error
			if d, err = instrument.AddStage(d, reg, "filter", true); err != nil {
				t.Fatal(err)
			}
		}
		d, err := instrument.SetField(d, reg, "filter_cutoff", cutoff)
		if err != nil {
			t.Fatal(err)
		}
		pf := projectFile{Envelope: schema.Current(), Instruments: []exportInstrument{{ID: "k", Volume: 1}},
			InstrumentDocs: map[instrument.ID]projectDoc{"k": {Doc: d}}}
		b, err := json.Marshal(pf)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	d, ok := describeUndoChange(file(1000), file(2000))
	if !ok {
		t.Fatal("describeUndoChange found no change")
	}
	if d.param != "Cutoff" || d.old != "1.0 kHz" || d.new != "2.0 kHz" {
		t.Errorf("got %q %q -> %q, want Cutoff 1.0 kHz -> 2.0 kHz", d.param, d.old, d.new)
	}
}
