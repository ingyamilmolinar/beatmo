//go:build test && !js

package ui

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	"github.com/ingyamilmolinar/beatmo/internal/theme"
)

// Each cut's script runs through real Game.Update/Draw frames on a frame clock; every step must reach its state
// through real input only, and the cue file must describe what the video shows.

type tourRun struct {
	g      *Game
	r      *tourRunner
	drawn  map[string]bool
	frames int
	end    float64 // tour clock at the end
}

func startTourGame(t *testing.T, cut string) *Game {
	t.Helper()
	t.Cleanup(func() { ApplyTheme(theme.Doc{}) })
	// The test build stubs SetForceMobileProfile / SetForceAutoSize and collapses the audio panel, so the test knobs and
	// the production-floored layout are set directly.
	prevEQ, prevEQT := eqPanelHeight, eqPanelHeightForTest
	eqPanelHeight, eqPanelHeightForTest = 190, 190
	forceAutoSize = true
	forceSmallScreenForTest = cut == "phone"
	UpdateProfile()
	t.Cleanup(func() {
		eqPanelHeight, eqPanelHeightForTest = prevEQ, prevEQT
		forceAutoSize = false
		forceSmallScreenForTest = false
		UpdateProfile()
	})
	t.Cleanup(audio.ClearAllInsertEffects)
	prevLocale := i18n.ActiveLocale() // the tour pins English
	t.Cleanup(func() { i18n.SetLocale(prevLocale) })
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	// The test build builds no startup demo; the tour imports its own circuit.
	g.demoBuilt = true
	return g
}

// steps == nil plays the cut's real script; maxFrames bounds a tour that never ends.
func runTour(t *testing.T, cut string, w, h int, steps []tourStep, maxFrames int) tourRun {
	t.Helper()
	g := startTourGame(t, cut)
	if err := g.StartTour(cut, ""); err != nil {
		t.Fatal(err)
	}
	r := g.tour.(*tourRunner)
	if steps != nil {
		r.steps = steps
	}
	frame := 0
	r.clock = func() float64 { return float64(frame) / 60 }
	r.log = func(string, ...any) {}
	drawn := map[string]bool{}
	textSpriteHook = func(s string) { drawn[s] = true }
	t.Cleanup(func() { textSpriteHook = nil })

	g.Layout(w, h)
	screen := ebiten.NewImage(w, h)
	for ; frame < maxFrames; frame++ {
		if g.Update() == ebiten.Termination {
			break
		}
		g.Draw(screen)
	}
	return tourRun{g: g, r: r, drawn: drawn, frames: frame, end: float64(frame) / 60}
}

var tourCuts = []struct {
	cut  string
	w, h int
}{{"desktop", 1280, 720}, {"phone", 390, 844}}

func TestShowcaseTourReachesEveryStep(t *testing.T) {
	for _, c := range tourCuts {
		t.Run(c.cut, func(t *testing.T) {
			const maxFrames = 60 * 180
			run := runTour(t, c.cut, c.w, c.h, nil, maxFrames)
			g, r := run.g, run.r
			if run.frames >= maxFrames {
				t.Fatalf("the tour did not finish within %d frames (step %d/%d)", maxFrames, r.idx, len(r.steps))
			}
			if r.err != nil {
				t.Fatalf("the tour failed: %v", r.err)
			}
			// One line per failure, named by step.
			for _, e := range r.errs {
				t.Errorf("%s", e)
			}
			if r.idx != len(r.steps) {
				t.Fatalf("the tour stopped at step %d/%d", r.idx, len(r.steps))
			}
			// The theme is back to the opening one, so the loop is seamless.
			n, ok := g.graph.GetNodeByID(r.target)
			if !ok || n.Type != model.NodeTypeChord {
				t.Errorf("the tour's node is not a chord (found=%v)", ok)
			}
			if len(audio.GetInsertEffects(g.drum.Rows[0].Instrument)) == 0 {
				t.Error("no insert effect on the lead row")
			}
			if p := g.UserTheme().Preset; p != "" && p != defaultThemePresetID {
				t.Errorf("the tour ended on theme %q, want the opening %q", p, defaultThemePresetID)
			}
			if !g.Playing() {
				t.Error("the song is not playing at the end of the tour")
			}
			if err := reachedNothingOpen(r, g); err != nil {
				t.Errorf("at the end: %v", err)
			}
			if c.cut == "phone" && g.drum.currentViewMode != viewModeRows {
				t.Errorf("the phone tour ended on view %v, want the pads (the opening view)", g.drum.currentViewMode)
			}
			// The landing never names the template's source game.
			for s := range run.drawn {
				if strings.Contains(strings.ToLower(s), "tetris") {
					t.Errorf("the tour drew %q", s)
				}
			}
			if len(run.drawn) < 50 {
				t.Errorf("the text hook saw only %d strings — is it still wired?", len(run.drawn))
			}
			// The cue file the landing reads, as the recorder writes it.
			vtt, err := r.vtt(run.end)
			if err != nil {
				t.Fatal(err)
			}
			cues, err := parseTourVTT(vtt)
			if err != nil {
				t.Fatalf("the cue file does not parse: %v\n%s", err, vtt)
			}
			checkTourCues(t, cues, r.steps, r.startedAt, run.end, 60/r.beatSec, r.beatSec)
		})
	}
}

// A step without a check is one whose mis-tap would reach the video unnoticed.
func TestEveryTourStepDeclaresItsState(t *testing.T) {
	for cut, steps := range tourScripts {
		seen := map[string]bool{}
		for _, s := range steps {
			if s.reached == nil {
				t.Errorf("%s tour: step %q has no reached check", cut, s.name)
			}
			if seen[s.name] {
				t.Errorf("%s tour: two steps named %q (errors are reported by name)", cut, s.name)
			}
			seen[s.name] = true
			if s.title != "" && (s.chapter == "" || s.sub == "" || (s.side != "left" && s.side != "right")) {
				t.Errorf("%s tour: captioned step %q needs a chapter, a subtitle and a side", cut, s.name)
			}
		}
	}
}

// A missed state, a stray tap, a failing action and a non-converging repeat are each reported by name; none hangs
// the tour.
func TestTourReportsEveryKindOfFailure(t *testing.T) {
	tapEmptyGrid := func(r *tourRunner, g *Game) error {
		p, ok := emptyGridPoint(g, 0.5, 0.5)
		if !ok {
			return fmt.Errorf("no empty grid point")
		}
		r.tap(p)
		return nil
	}
	steps := []tourStep{
		{name: "claims-the-sidebar", beats: 1, act: func(*tourRunner, *Game) error { return nil },
			reached: func(_ *tourRunner, g *Game) error {
				if !g.sidebar.IsOpen() {
					return fmt.Errorf("node sidebar did not open")
				}
				return nil
			}},
		{name: "stray-tap", beats: 1, act: tapEmptyGrid, reached: func(*tourRunner, *Game) error { return nil }},
		{name: "never-converges", beats: 0, repeat: 5, until: func(*tourRunner, *Game) bool { return false },
			act: func(*tourRunner, *Game) error { return nil }, reached: func(*tourRunner, *Game) error { return nil }},
		{name: "act-fails", beats: 1, act: func(*tourRunner, *Game) error { return fmt.Errorf("boom") }},
		{name: "never-runs", beats: 1, act: func(*tourRunner, *Game) error { t.Error("a step after a failed action ran"); return nil }},
	}
	run := runTour(t, "desktop", 1280, 720, steps, 60*60)
	r := run.r
	if run.frames >= 60*60 {
		t.Fatalf("the failing tour did not terminate (step %d)", r.idx)
	}
	errs := strings.Join(r.errs, "\n")
	if !strings.Contains(errs, `step "claims-the-sidebar": node sidebar did not open`) {
		t.Errorf("a step that missed its state was not reported; errs:\n%s", errs)
	}
	if !strings.Contains(errs, `after step "stray-tap": the circuit changed`) {
		t.Errorf("a stray tap that changed the circuit was not reported; errs:\n%s", errs)
	}
	if !strings.Contains(errs, `step "never-converges": did not converge in 6 tries`) {
		t.Errorf("a repeat step that never converged was not reported; errs:\n%s", errs)
	}
	if r.err == nil || !strings.Contains(r.err.Error(), `step "act-fails": boom`) {
		t.Errorf("a failing action did not stop the tour: err=%v", r.err)
	}
}

// A slot with no new draw holds the PREVIOUS frame and a new frame lands in its own slot, never early, so a stall
// freezes the picture and never shows a frame ahead of its sound.
func TestTourFrameSlotsAreConstantRate(t *testing.T) {
	var buf bytes.Buffer
	r := &tourRunner{ffIn: nopWriteCloser{&buf}, pix: make([]byte, 4)}
	draws := []float64{0, 0.01, 0.039, 0.04, 0.2, 1.05, 1.06}
	for k, tm := range draws {
		if want := tourFrameSlots(tm); want > r.written {
			for i := range r.pix { // "ReadPixels": this draw's frame is all byte k
				r.pix[i] = byte(k)
			}
			if err := r.writeSlots(want); err != nil {
				t.Fatal(err)
			}
		}
	}
	// Slots begun by 1.06 s: floor(1.06·25)+1 = 27.
	if r.written != 27 || buf.Len() != 27*4 {
		t.Fatalf("wrote %d frames (%d bytes), want 27", r.written, buf.Len())
	}
	var got []string
	for slot := 0; slot < 27; slot++ {
		got = append(got, strconv.Itoa(int(buf.Bytes()[slot*4])))
	}
	want := "0 3 3 3 3 4 4 4 4 4 4 4 4 4 4 4 4 4 4 4 4 4 4 4 4 4 5"
	if strings.Join(got, " ") != want {
		t.Errorf("slot frames\n got %s\nwant %s", strings.Join(got, " "), want)
	}
	if r.repeated != 23 {
		t.Errorf("repeated = %d, want 23", r.repeated)
	}
	// The longest freeze: slots 6-25 held draw 4's frame (20 slots).
	if r.longestRun != 20 {
		t.Errorf("longestRun = %d, want 20", r.longestRun)
	}
	// Repeats in second 0: slots 2-4 and 6-24 = 22; second 1: slot 25.
	if fmt.Sprint(r.repeatsBySec) != "[22 1]" {
		t.Errorf("repeatsBySec = %v, want [22 1]", r.repeatsBySec)
	}
	if got := tourFrameSlots(0.03999); got != 1 {
		t.Errorf("tourFrameSlots just before the second slot = %d", got)
	}
}

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

// The audio offset (and with it A/V sync) rests on firstOnsetSeconds.
func TestFirstOnsetSeconds(t *testing.T) {
	dir := t.TempDir()
	const sr = 48000
	write := func(name string, channels, bits int, onsetFrame int, extraChunk bool) string {
		frames := sr / 2
		var data bytes.Buffer
		for f := 0; f < frames; f++ {
			v := int16(0)
			if f >= onsetFrame {
				v = int16(12000 * math.Sin(float64(f-onsetFrame)*0.3+0.5))
			} else if f%7 == 0 {
				v = 200 // noise floor well under the 2% threshold
			}
			for c := 0; c < channels; c++ {
				_ = binary.Write(&data, binary.LittleEndian, v)
			}
		}
		var b bytes.Buffer
		b.WriteString("RIFF")
		_ = binary.Write(&b, binary.LittleEndian, uint32(0))
		b.WriteString("WAVE")
		if extraChunk {
			b.WriteString("LIST")
			_ = binary.Write(&b, binary.LittleEndian, uint32(5))
			b.WriteString("abcde\x00") // odd size + pad byte
		}
		b.WriteString("fmt ")
		_ = binary.Write(&b, binary.LittleEndian, uint32(16))
		for _, v := range []any{uint16(1), uint16(channels), uint32(sr), uint32(sr * channels * bits / 8), uint16(channels * bits / 8), uint16(bits)} {
			_ = binary.Write(&b, binary.LittleEndian, v)
		}
		b.WriteString("data")
		_ = binary.Write(&b, binary.LittleEndian, uint32(data.Len()))
		b.Write(data.Bytes())
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, b.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	for _, c := range []struct {
		name     string
		channels int
		onset    int
		extra    bool
	}{{"mono.wav", 1, 4800, false}, {"stereo.wav", 2, 7213, true}, {"at0.wav", 2, 0, false}} {
		got, err := firstOnsetSeconds(write(c.name, c.channels, 16, c.onset, c.extra), 0.02)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		// The first sample may sit under the threshold (sine phase); a few samples are far below one video frame.
		if want := float64(c.onset) / sr; got < want || got > want+3.0/sr {
			t.Errorf("%s: onset %.6f s, want %.6f s", c.name, got, want)
		}
	}
	if _, err := firstOnsetSeconds(write("silent.wav", 1, 16, sr, false), 0.02); err == nil || !strings.Contains(err.Error(), "silent") {
		t.Errorf("silent WAV: err = %v", err)
	}
	if _, err := firstOnsetSeconds(write("pcm24.wav", 1, 24, 10, false), 0.02); err == nil {
		t.Error("a 24-bit WAV was accepted")
	}
	junk := filepath.Join(dir, "junk.wav")
	_ = os.WriteFile(junk, bytes.Repeat([]byte("x"), 100), 0o644)
	if _, err := firstOnsetSeconds(junk, 0.02); err == nil {
		t.Error("a non-WAV file was accepted")
	}
}

// A user decision; the template's own title is out of scope.
func TestLandingAssetsNeverSayTetris(t *testing.T) {
	js := filepath.Join("..", "..", "..", "js")
	files := []string{filepath.Join(js, "landing.js"), filepath.Join(js, "landing_crystal.js"), filepath.Join(js, "index.html")}
	media, _ := filepath.Glob(filepath.Join(js, "landing", "*"))
	files = append(files, media...)
	for _, f := range files {
		if strings.Contains(strings.ToLower(filepath.Base(f)), "tetris") {
			t.Errorf("landing file name %s", f)
		}
		if strings.HasSuffix(f, ".mp4") || strings.HasSuffix(f, ".webp") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(strings.ToLower(string(b)), "tetris") {
			t.Errorf("%s mentions the template's source game", f)
		}
	}
	for _, s := range append(append([]tourStep{}, tourScripts["desktop"]...), tourScripts["phone"]...) {
		for _, txt := range []string{s.title, s.sub, s.chapter, s.name} {
			if strings.Contains(strings.ToLower(txt), "tetris") {
				t.Errorf("tour step %q says %q", s.name, txt)
			}
		}
	}
}

// ─── the cue file (shared with landing_assets_test.go) ───

type vttCue struct {
	start, end float64
	payload    map[string]any
}

var vttTimingRE = regexp.MustCompile(`^(\d\d):(\d\d):(\d\d)\.(\d\d\d) --> (\d\d):(\d\d):(\d\d)\.(\d\d\d)$`)

// A header, then blocks of a timing line and ONE JSON payload line.
func parseTourVTT(s string) ([]vttCue, error) {
	blocks := strings.Split(strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n")), "\n\n")
	if len(blocks) == 0 || blocks[0] != "WEBVTT" {
		return nil, fmt.Errorf("no WEBVTT header")
	}
	var out []vttCue
	for i, b := range blocks[1:] {
		lines := strings.Split(b, "\n")
		if len(lines) != 2 {
			return nil, fmt.Errorf("cue %d: %d lines, want timing + one JSON line", i, len(lines))
		}
		m := vttTimingRE.FindStringSubmatch(lines[0])
		if m == nil {
			return nil, fmt.Errorf("cue %d: bad timing line %q", i, lines[0])
		}
		sec := func(h, mi, s, ms string) float64 {
			a, _ := strconv.Atoi(h)
			b, _ := strconv.Atoi(mi)
			c, _ := strconv.Atoi(s)
			d, _ := strconv.Atoi(ms)
			return float64(a*3600+b*60+c) + float64(d)/1000
		}
		c := vttCue{start: sec(m[1], m[2], m[3], m[4]), end: sec(m[5], m[6], m[7], m[8])}
		if err := json.Unmarshal([]byte(lines[1]), &c.payload); err != nil {
			return nil, fmt.Errorf("cue %d: payload: %v", i, err)
		}
		if c.end <= c.start {
			return nil, fmt.Errorf("cue %d: ends at %.3f before it starts at %.3f", i, c.end, c.start)
		}
		out = append(out, c)
	}
	return out, nil
}

var hexColorRE = regexp.MustCompile(`^#[0-9A-F]{6}$`)

// beatSec <= 0 skips the dwell check, since a committed recording's clock is the real one. The video runs from the
// first step after the pre-roll to end.
func checkTourCues(t *testing.T, cues []vttCue, steps []tourStep, start, end, bpm, beatSec float64) {
	t.Helper()
	if len(cues) == 0 {
		t.Fatal("no cues")
	}
	head := cues[0]
	song, _ := head.payload["song"].(map[string]any)
	if song == nil || head.start != 0 || math.Abs(head.end-end) > 0.05 {
		t.Fatalf("the first cue is not the whole-length head cue: %+v", head)
	}
	if got, _ := song["bpm"].(float64); math.Abs(got-bpm) > 0.01 {
		t.Errorf("head cue bpm %v, want %v", song["bpm"], bpm)
	}
	if b0, _ := song["beat0"].(float64); b0 < 0 || b0 > 10 {
		t.Errorf("head cue beat0 %v: the song should start within the first seconds", song["beat0"])
	}
	pals, _ := head.payload["palettes"].([]any)
	var palIDs []string
	for _, p := range pals {
		m, _ := p.(map[string]any)
		id, _ := m["id"].(string)
		palIDs = append(palIDs, id)
		cols, _ := m["cols"].([]any)
		if len(cols) != 6 {
			t.Errorf("palette %q has %d colours, want 6", id, len(cols))
		}
		for _, c := range append(cols, m["bg"], m["acc"]) {
			if s, _ := c.(string); !hexColorRE.MatchString(s) {
				t.Errorf("palette %q colour %v is not #RRGGBB", id, c)
			}
		}
	}
	var presetIDs []string
	for _, p := range themePresets {
		presetIDs = append(presetIDs, p.ID)
	}
	if strings.Join(palIDs, ",") != strings.Join(presetIDs, ",") {
		t.Errorf("palettes %v, want every preset %v", palIDs, presetIDs)
	}

	var captions, themes []vttCue
	for _, c := range cues[1:] {
		if c.end > end+0.05 {
			t.Errorf("cue %+v ends after the video (%.2f s)", c.payload, end)
		}
		switch {
		case c.payload["title"] != nil:
			captions = append(captions, c)
		case c.payload["theme"] != nil:
			themes = append(themes, c)
		default:
			t.Errorf("a cue the landing does not understand: %v", c.payload)
		}
	}
	var titled []tourStep
	for _, s := range steps {
		if s.title != "" {
			titled = append(titled, s)
		}
	}
	if len(captions) != len(titled) {
		t.Fatalf("%d caption cues, the script has %d captioned steps", len(captions), len(titled))
	}
	for i, s := range titled {
		c := captions[i]
		want := map[string]string{"chapter": s.chapter, "label": tourChapterLabel(s.chapter), "title": s.title, "sub": s.sub, "side": s.side}
		for k, v := range want {
			if got, _ := c.payload[k].(string); got != v {
				t.Errorf("caption %d (%s): %s = %q, want %q", i, s.name, k, got, v)
			}
		}
		if i > 0 && math.Abs(captions[i-1].end-c.start) > 0.002 {
			t.Errorf("caption %d starts at %.3f, the previous ends at %.3f: captions must be back to back", i, c.start, captions[i-1].end)
		}
		if beatSec > 0 && c.end-c.start < s.beats*beatSec-0.02 {
			t.Errorf("caption %q lasts %.2f s, shorter than its step's %.1f-beat dwell", s.title, c.end-c.start, s.beats)
		}
	}
	// Start and end on the default so the loop is seamless.
	if len(themes) == 0 {
		t.Fatal("no theme cues")
	}
	var ids []string
	for i, c := range themes {
		id, _ := c.payload["theme"].(string)
		ids = append(ids, id)
		if i > 0 && math.Abs(themes[i-1].end-c.start) > 0.002 {
			t.Errorf("theme cue %d starts at %.3f, the previous ends at %.3f", i, c.start, themes[i-1].end)
		}
	}
	if math.Abs(themes[0].start-start) > 0.05 || math.Abs(themes[len(themes)-1].end-end) > 0.05 {
		t.Errorf("theme cues span %.2f–%.2f s, the video %.2f–%.2f s", themes[0].start, themes[len(themes)-1].end, start, end)
	}
	if math.Abs(captions[0].start-start) > 0.05 {
		t.Errorf("the first caption starts at %.2f s, the video at %.2f s", captions[0].start, start)
	}
	if got, want := strings.Join(ids, ","), "vice-city,ocean,ember,vice-city"; got != want {
		t.Errorf("theme sequence %s, want %s", got, want)
	}
}
