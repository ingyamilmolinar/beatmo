package uistate

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// fakeApplier records every applier call as a printable label so tests
// can assert both the call set and ordering without a real *ui.Game.
type fakeApplier struct {
	calls []string
}

func (f *fakeApplier) SetForceMobileProfile(v bool) { f.calls = append(f.calls, label("SetForceMobileProfile", v)) }
func (f *fakeApplier) SetForceAutoSize(v bool)      { f.calls = append(f.calls, label("SetForceAutoSize", v)) }
func (f *fakeApplier) SetDefaultStart(v bool)       { f.calls = append(f.calls, label("SetDefaultStart", v)) }
func (f *fakeApplier) SetSplitterFrac(v float64)    { f.calls = append(f.calls, label("SetSplitterFrac", v)) }
func (f *fakeApplier) SetCameraOffsetX(v float64)   { f.calls = append(f.calls, label("SetCameraOffsetX", v)) }
func (f *fakeApplier) SetCameraOffsetY(v float64)   { f.calls = append(f.calls, label("SetCameraOffsetY", v)) }
func (f *fakeApplier) SetCameraScale(v float64)     { f.calls = append(f.calls, label("SetCameraScale", v)) }
func (f *fakeApplier) CenterCamera()                { f.calls = append(f.calls, "CenterCamera") }
func (f *fakeApplier) SetViewMode(audio bool)       { f.calls = append(f.calls, label("SetViewMode", audio)) }
func (f *fakeApplier) SetMobileEQCollapsed(v bool)  { f.calls = append(f.calls, label("SetMobileEQCollapsed", v)) }
func (f *fakeApplier) OpenSidebarForNodeID(id int)  { f.calls = append(f.calls, label("OpenSidebarForNodeID", id)) }

func label(name string, v any) string { return fmt.Sprintf("%s:%v", name, v) }

func ptrBool(b bool) *bool        { return &b }
func ptrFloat(f float64) *float64 { return &f }
func ptrInt(i int) *int           { return &i }

func TestApplyOrderHitsAllSetters(t *testing.T) {
	a := &fakeApplier{}
	cfg := Config{
		Profile:       "mobile",
		ForceAutoSize: ptrBool(true),
		DefaultStart:  ptrBool(false),
		Splitter:      &SplitterConfig{Frac: ptrFloat(0.5)},
		Camera: &CameraConfig{
			X: ptrFloat(10), Y: ptrFloat(-3), Scale: ptrFloat(2), Center: true,
		},
		View:    &ViewConfig{Mode: "audio", MobileEQCollapsed: ptrBool(true)},
		Sidebar: &SidebarConfig{OpenNodeID: ptrInt(7)},
	}
	if err := apply(a, cfg); err != nil {
		t.Fatalf("apply: %v", err)
	}
	want := []string{
		"SetForceMobileProfile:true",
		"SetForceAutoSize:true",
		"SetDefaultStart:false",
		"SetSplitterFrac:0.5",
		"SetCameraOffsetX:10",
		"SetCameraOffsetY:-3",
		"SetCameraScale:2",
		"CenterCamera",
		"SetViewMode:true",
		"SetMobileEQCollapsed:true",
		"OpenSidebarForNodeID:7",
	}
	if !reflect.DeepEqual(a.calls, want) {
		t.Fatalf("call sequence mismatch:\n got: %v\nwant: %v", a.calls, want)
	}
}

func TestApplyEmptyConfigCallsNothing(t *testing.T) {
	a := &fakeApplier{}
	if err := apply(a, Config{}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(a.calls) != 0 {
		t.Fatalf("expected 0 calls, got %v", a.calls)
	}
}

func TestApplyViewModeRowsAudioAndUnknown(t *testing.T) {
	cases := []struct {
		mode string
		want []string
	}{
		{"rows", []string{"SetViewMode:false"}},
		{"audio", []string{"SetViewMode:true"}},
		{"garbage", nil},
		{"", nil},
	}
	for _, tc := range cases {
		t.Run(tc.mode, func(t *testing.T) {
			a := &fakeApplier{}
			if err := apply(a, Config{View: &ViewConfig{Mode: tc.mode}}); err != nil {
				t.Fatalf("apply: %v", err)
			}
			if !reflect.DeepEqual(a.calls, tc.want) {
				t.Fatalf("mode=%q: calls=%v want=%v", tc.mode, a.calls, tc.want)
			}
		})
	}
}

func TestApplyProfileNonMobileSkipsForceMobile(t *testing.T) {
	a := &fakeApplier{}
	if err := apply(a, Config{Profile: "desktop"}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	for _, c := range a.calls {
		if strings.HasPrefix(c, "SetForceMobileProfile") {
			t.Fatalf("desktop profile should not call SetForceMobileProfile, got %v", a.calls)
		}
	}
}

func TestApplyCameraPartialFields(t *testing.T) {
	a := &fakeApplier{}
	cfg := Config{Camera: &CameraConfig{X: ptrFloat(1)}}
	if err := apply(a, cfg); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !reflect.DeepEqual(a.calls, []string{"SetCameraOffsetX:1"}) {
		t.Fatalf("calls=%v", a.calls)
	}
}

func TestApplyFileEndToEnd(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ui-state.json")
	body := []byte(`{"profile":"mobile","view":{"mode":"audio"},"camera":{"scale":1.5}}`)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	a := &fakeApplier{}
	if err := applyFile(a, path); err != nil {
		t.Fatalf("applyFile: %v", err)
	}
	want := []string{
		"SetForceMobileProfile:true",
		"SetCameraScale:1.5",
		"SetViewMode:true",
	}
	if !reflect.DeepEqual(a.calls, want) {
		t.Fatalf("calls=%v want=%v", a.calls, want)
	}
}

func TestApplyFileMissing(t *testing.T) {
	a := &fakeApplier{}
	err := applyFile(a, filepath.Join(t.TempDir(), "no-such.json"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "read ui-state") {
		t.Fatalf("error = %q, want prefix 'read ui-state'", err.Error())
	}
}

func TestApplyFileMalformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	a := &fakeApplier{}
	err := applyFile(a, path)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "parse ui-state") {
		t.Fatalf("error = %q, want prefix 'parse ui-state'", err.Error())
	}
}

