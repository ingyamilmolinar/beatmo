//go:build !js

package ui

// The showcase tour: a scripted walk through the real game, driven through the
// real input path, that `make showcase-video` records for the landing page
// (spec docs/superpowers/specs/2026-10-23-landing-page-design.md, "The
// recording"). Desktop only (the !js tag keeps it out of main.wasm).
//
// Input: the tour owns the input.go function variables for its whole run
// (SetInputForTest) and feeds one scripted frame per Update — a cursor
// position, the left button and the wheel. Nothing calls a game setter; every
// change the video shows is a tap, a drag or a wheel notch the normal
// Game.Update handles.
//
// Recording: the master bus goes through the game's own desktop Rec path, and
// every finished frame is piped as raw RGBA to an ffmpeg child at a constant
// 25 fps of the RECORDING clock. A frame slot that passes without a new draw
// repeats the previous frame, so picture and sound share one timeline by
// construction; repeated slots are counted and reported.

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/assets"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/theme"
)

// TourFPS is the recording's constant frame rate. 25, not 30: under xvfb's
// software GL the game draws ~28 frames/s, so 30 repeated one slot in six
// (measured 17-19%) while 25 keeps nearly every slot a fresh frame.
const TourFPS = 25

// tourTemplateStem names the built-in template the tour plays (the techno
// Korobeiniki arrangement). Only code ever sees this stem; the landing never
// shows it.
const tourTemplateStem = "tetris-techno"

type tourFrame struct {
	x, y   int
	down   bool
	wheelY float64
}

// tourStep is one beat of the tour: an action (queued input frames), a dwell
// measured from the step's start, and an optional caption that opens a
// chapter on the landing page.
type tourStep struct {
	name    string
	chapter string // chapter id; a step with a caption and a chapter starts a new caption cue
	title   string
	sub     string
	side    string  // "left" | "right": where the landing puts the caption
	beats   float64 // dwell from the step's start, in beats of the song
	act     func(r *tourRunner, g *Game) error
	reached func(r *tourRunner, g *Game) error // the state this step must leave (asserted by the test)
	preroll bool                               // runs before the recording starts (layout setup)
	until   func(r *tourRunner, g *Game) bool  // with repeat: re-run act until this holds
	repeat  int                                // max re-runs of act
}

type tourCue struct {
	start, end float64
	payload    map[string]any
}

type tourRunner struct {
	cut       string
	steps     []tourStep
	idx       int
	queue     []tourFrame
	cur       tourFrame
	parkX     int
	parkY     int
	booted    bool
	settle    int
	playing   bool
	waitUntil float64
	start     time.Time
	beatSec   float64
	beat0     float64
	err       error
	restore   func()
	target    model.NodeID    // the node the tour is editing
	cell      image.Rectangle // the picker cell just picked (for the confirming re-tap)
	done      bool            // a repeat step's goal was reached
	reps      int             // re-runs of the current repeat step
	errs      []string        // failed step checks / invariants (tour_checks.go)

	// what the step checks compare against (tour_checks.go)
	mark      map[string]float64       // a value an act saw before it acted (a handle's Y, the zoom)
	wantPitch float64                  // the note the tour picked
	docs0     map[instrument.ID]string // instrument document hashes before the dial turn
	nodes0    int                      // the template's node / edge counts
	edges0    int
	started   bool    // the first step after the pre-roll has begun (the video's start)
	startedAt float64 // ... at this tour-clock time (0 when recording)

	// recording
	recordDir    string
	audioDir     string
	ff           *exec.Cmd
	ffIn         io.WriteCloser
	pix          []byte // the frame being read back
	prev         []byte // the last frame written (held through slots no draw arrived in)
	w, h         int
	written      int
	repeated     int
	longestRun   int     // the longest run of repeated slots (a visible freeze)
	draws        int     // Draw calls while recording
	captureSec   float64 // time spent reading back + piping frames
	readSec      float64 // of which: GPU read-back
	repeatsBySec []int
	cues         []tourCue
	openCaption  *tourCue
	openTheme    *tourCue
	themeID      string
	log          func(string, ...any)
	clock        func() float64 // test seam: replaces the wall/recording clock
}

// StartTour arms the scripted tour for the given cut ("desktop" | "phone").
// recordDir == "" plays it live; otherwise it records into recordDir:
// frames.mkv (lossless video), audio/<stamp>/master.wav, <cut>.vtt and
// report.json. The process exits when the tour ends.
func (g *Game) StartTour(cut, recordDir string) error {
	steps, ok := tourScripts[cut]
	if !ok {
		return fmt.Errorf("unknown tour %q (want desktop or phone)", cut)
	}
	if cut == "phone" {
		g.SetForceMobileProfile(true) // the phone cut records the mobile layout
	}
	r := &tourRunner{cut: cut, steps: steps, recordDir: recordDir, start: time.Now(), log: g.logger.Infof}
	if recordDir != "" {
		// The recorder reads every frame back itself, so it never needs the
		// swap to wait for the display — and a compositor that thinks the
		// window is hidden throttles that wait to ~1 Hz (2026-10-24: 81-94%
		// repeated frames with vsync on).
		ebiten.SetVsyncEnabled(false)
	}
	g.tour = r
	return nil
}

// now is the tour clock: the recording's elapsed time while recording (the
// clock the video frames and the cue file are stamped with), else wall time.
func (r *tourRunner) now() float64 {
	if r.clock != nil {
		return r.clock()
	}
	if r.recordDir != "" && audio.IsRecording() {
		return audio.RecordingElapsed().Seconds()
	}
	return time.Since(r.start).Seconds()
}

func (r *tourRunner) installInput(g *Game) {
	origScreen := screenSize
	r.restore = SetInputForTest(
		func() (int, int) { return r.cur.x, r.cur.y },
		func(b ebiten.MouseButton) bool { return r.cur.down && b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, r.cur.wheelY },
		origScreen,
	)
}

func (r *tourRunner) tick(g *Game) bool {
	if r.err != nil {
		return r.finish(g)
	}
	if !r.booted {
		return r.boot(g)
	}
	t := r.now()
	r.trackTheme(g, t)
	if !r.playing && g.Playing() {
		r.playing = true
		r.beat0 = t // the song's first beat on the recording clock
	}
	if len(r.queue) > 0 {
		r.cur, r.queue = r.queue[0], r.queue[1:]
		return false
	}
	r.cur = tourFrame{x: r.parkX, y: r.parkY}
	if t < r.waitUntil {
		return false
	}
	if r.idx > 0 {
		prev := r.steps[r.idx-1]
		if prev.until != nil && !prev.until(r, g) {
			if r.reps < prev.repeat && r.err == nil {
				r.reps++
				r.runAct(prev, g, t)
				return false
			}
			r.errs = append(r.errs, fmt.Sprintf("step %q: did not converge in %d tries", prev.name, prev.repeat+1))
		}
		if prev.reached != nil {
			if err := prev.reached(r, g); err != nil {
				r.errs = append(r.errs, fmt.Sprintf("step %q: %v", prev.name, err))
				r.log("[tour] step %q did not reach its state: %v", prev.name, err)
			}
		}
		if err := r.tourInvariants(g); err != nil {
			r.errs = append(r.errs, fmt.Sprintf("after step %q: %v", prev.name, err))
			r.log("[tour] after step %q: %v", prev.name, err)
		}
	}
	if r.idx >= len(r.steps) {
		return r.finish(g)
	}
	s := r.steps[r.idx]
	if !s.preroll && !r.started {
		// The video starts with the first step after the pre-roll.
		r.started = true
		if r.recordDir != "" {
			if err := r.startRecording(g); err != nil {
				r.err = err
				return false
			}
			t = r.now()
		}
		r.startedAt = t
		// The cue file starts with the video: drop what the pre-roll tracked.
		r.cues, r.openTheme, r.themeID = nil, nil, ""
		r.trackTheme(g, t)
	}
	r.idx++
	r.reps = 0
	r.done = false
	if s.title != "" {
		r.closeCaption(t)
		r.openCaption = &tourCue{start: t, payload: map[string]any{"chapter": s.chapter, "label": tourChapterLabel(s.chapter), "title": s.title, "sub": s.sub, "side": s.side}}
	}
	r.runAct(s, g, t)
	r.log("[tour] %.2fs step %q", t, s.name)
	return false
}

func (r *tourRunner) runAct(s tourStep, g *Game, t float64) {
	r.waitUntil = t + s.beats*r.beatSec
	if s.act != nil {
		if err := s.act(r, g); err != nil {
			r.err = fmt.Errorf("step %q: %w", s.name, err)
			r.log("[tour] %v", r.err)
		}
	}
}

// boot waits for the demo, imports the tour's template (programmatically — the
// template menu would show its title), waits for the voices to warm up, then
// starts recording.
func (r *tourRunner) boot(g *Game) bool {
	if !g.demoBuilt {
		return false
	}
	if r.settle == 0 {
		var data []byte
		for _, tp := range assets.Templates() {
			if tp.Genre == tourTemplateStem {
				data = tp.Bytes
			}
		}
		if data == nil {
			r.err = fmt.Errorf("no template %q", tourTemplateStem)
			return false
		}
		if err := g.Import(data); err != nil {
			r.err = fmt.Errorf("import template: %w", err)
			return false
		}
		// The captions are English, so the recorded UI is too, whatever the
		// machine's language (setup, like the import — never in the video).
		if i18n.ActiveLocale() != i18n.LocaleEN {
			i18n.SetLocale(i18n.LocaleEN)
		}
		r.installInput(g)
		r.parkX, r.parkY = g.winW-2, gridTopOffset()+2
		r.cur = tourFrame{x: r.parkX, y: r.parkY}
	}
	r.settle++
	if !g.playbackReady() || r.settle < 60 {
		return false
	}
	bpm := g.drum.BPM()
	if bpm <= 0 {
		bpm = 140
	}
	r.beatSec = 60 / float64(bpm)
	r.nodes0, r.edges0 = len(g.nodes), len(g.edges)
	r.booted = true
	return false
}

func (r *tourRunner) startRecording(g *Game) error {
	if err := os.MkdirAll(r.recordDir, 0o755); err != nil {
		return err
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("ffmpeg not found: %w", err)
	}
	r.w, r.h = g.winW, g.winH
	r.pix = make([]byte, 4*r.w*r.h)
	r.ff = exec.Command("ffmpeg", "-loglevel", "error", "-y",
		"-f", "rawvideo", "-pix_fmt", "rgba", "-s", fmt.Sprintf("%dx%d", r.w, r.h), "-r", fmt.Sprint(TourFPS), "-i", "-",
		"-c:v", "libx264", "-preset", "ultrafast", "-qp", "0", "-pix_fmt", "yuv444p",
		filepath.Join(r.recordDir, "frames.mkv"))
	r.ff.Stderr = os.Stderr
	in, err := r.ff.StdinPipe()
	if err != nil {
		return err
	}
	r.ffIn = in
	if err := r.ff.Start(); err != nil {
		return fmt.Errorf("start ffmpeg: %w", err)
	}
	r.audioDir = filepath.Join(r.recordDir, "audio")
	return audio.StartRecording(audio.RecordingOptions{OutputDir: r.audioDir, BPM: g.drum.BPM(), Format: audio.FormatWAV16})
}

// frame writes the finished frame into every 1/TourFPS slot of the recording
// clock that has elapsed since the last write.
func (r *tourRunner) frame(g *Game, screen *ebiten.Image) {
	if r.ffIn == nil || !r.booted {
		return
	}
	r.draws++
	want := tourFrameSlots(r.now())
	if want <= r.written {
		return
	}
	c0 := time.Now()
	defer func() { r.captureSec += time.Since(c0).Seconds() }()
	b := screen.Bounds()
	if b.Dx() != r.w || b.Dy() != r.h {
		r.err = fmt.Errorf("window resized during recording: %dx%d", b.Dx(), b.Dy())
		return
	}
	pr, ok := any(screen).(interface{ ReadPixels([]byte) })
	if !ok {
		r.err = fmt.Errorf("this build cannot read frames back (test stub)")
		return
	}
	rp := time.Now()
	pr.ReadPixels(r.pix)
	r.readSec += time.Since(rp).Seconds()
	if err := r.writeSlots(want); err != nil {
		r.err = err
	}
}

// tourFrameSlots is how many 1/TourFPS slots of the recording clock have
// begun by t: slot k covers [k/TourFPS, (k+1)/TourFPS).
func tourFrameSlots(t float64) int { return int(t*TourFPS) + 1 }

// writeSlots brings the video up to `want` slots: the slots no draw arrived
// in hold the PREVIOUS frame (what was on screen then), and the frame just
// drawn (r.pix) goes into its own slot, the last — so a stall freezes the
// picture and never shows a later frame early, ahead of its sound.
func (r *tourRunner) writeSlots(want int) error {
	if r.prev == nil {
		r.prev = make([]byte, len(r.pix))
		copy(r.prev, r.pix)
	}
	if run := want - 1 - r.written; run > r.longestRun {
		r.longestRun = run
	}
	for r.written < want {
		frame := r.pix
		if r.written < want-1 {
			frame = r.prev
			r.repeated++
			sec := r.written / TourFPS
			for len(r.repeatsBySec) <= sec {
				r.repeatsBySec = append(r.repeatsBySec, 0)
			}
			r.repeatsBySec[sec]++
		}
		if _, err := r.ffIn.Write(frame); err != nil {
			return fmt.Errorf("ffmpeg write: %w", err)
		}
		r.written++
	}
	r.pix, r.prev = r.prev, r.pix // the frame just written is the next "previous"
	return nil
}

func (r *tourRunner) closeCaption(t float64) {
	if r.openCaption != nil {
		r.openCaption.end = t
		r.cues = append(r.cues, *r.openCaption)
		r.openCaption = nil
	}
}

// trackTheme opens a theme cue whenever the active preset changes, so the
// landing recolours exactly when the video does.
func (r *tourRunner) trackTheme(g *Game, t float64) {
	id := effectiveTheme(g.userTheme, g.circuitTheme).Preset
	if id == "" {
		id = defaultThemePresetID
	}
	if id == r.themeID {
		return
	}
	r.themeID = id
	if r.openTheme != nil {
		r.openTheme.end = t
		r.cues = append(r.cues, *r.openTheme)
	}
	r.openTheme = &tourCue{start: t, payload: map[string]any{"theme": id}}
}

func (r *tourRunner) finish(g *Game) bool {
	t := r.now()
	r.closeCaption(t)
	if r.openTheme != nil {
		r.openTheme.end = t
		r.cues = append(r.cues, *r.openTheme)
		r.openTheme = nil
	}
	if r.restore != nil {
		r.restore()
		r.restore = nil
	}
	if r.recordDir != "" && r.ffIn != nil {
		_ = r.ffIn.Close()
		ffErr := r.ff.Wait()
		res, recErr := audio.StopRecording()
		if recErr == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			_ = audio.WaitRecordingFinalized(ctx)
			cancel()
		}
		if err := r.writeOutputs(t, res); err != nil && r.err == nil {
			r.err = err
		}
		if ffErr != nil && r.err == nil {
			r.err = fmt.Errorf("ffmpeg: %w", ffErr)
		}
		r.ffIn = nil
	}
	if r.err != nil {
		r.log("[tour] FAILED: %v", r.err)
		fmt.Fprintf(os.Stderr, "[tour] FAILED: %v\n", r.err)
	}
	for _, e := range r.errs {
		fmt.Fprintf(os.Stderr, "[tour] %s\n", e)
	}
	return true
}

func (r *tourRunner) writeOutputs(end float64, res *audio.RecordingResult) error {
	master := ""
	if res != nil {
		for _, ch := range res.Channels {
			if ch.ID == audio.RecordingMasterID {
				master = ch.Path
			}
		}
	}
	vtt, err := r.vtt(end)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(r.recordDir, r.cut+".vtt"), []byte(vtt), 0o644); err != nil {
		return err
	}
	// The mixer renders ahead of what is heard, so the WAV leads the picture.
	// The song's first note is heard when the transport starts (beat0 on the
	// video clock): delaying the audio by beat0 - firstOnset lines them up.
	onset, err := firstOnsetSeconds(master, 0.02)
	if err != nil {
		return fmt.Errorf("measure audio onset: %w", err)
	}
	offset := r.beat0 - onset
	if err := os.WriteFile(filepath.Join(r.recordDir, "audio_offset.txt"), []byte(fmt.Sprintf("%.4f\n", offset)), 0o644); err != nil {
		return err
	}
	report := map[string]any{
		"repeatsBySecond": r.repeatsBySec, "audioFirstOnset": onset, "beat0": r.beat0, "audioOffset": offset,
		"cut": r.cut, "seconds": end, "frames": r.written, "repeatedFrames": r.repeated, "longestFreezeFrames": r.longestRun,
		"repeatedPct": 100 * float64(r.repeated) / float64(max(1, r.written)),
		"drawsPerSec": float64(r.draws) / math.Max(end, 1e-9), "captureMsPerFrame": 1000 * r.captureSec / float64(max(1, r.written-r.repeated)),
		"readbackMsPerFrame": 1000 * r.readSec / float64(max(1, r.written-r.repeated)),
		"masterWav":          master, "width": r.w, "height": r.h, "fps": TourFPS, "stepErrors": r.errs,
	}
	js, _ := json.MarshalIndent(report, "", "  ")
	fmt.Printf("[tour] %s: %.1fs, %d frames, %.1f%% repeated (longest freeze %d), audio offset %.3fs, step errors %d\n",
		r.cut, end, r.written, 100*float64(r.repeated)/float64(max(1, r.written)), r.longestRun, offset, len(r.errs))
	return os.WriteFile(filepath.Join(r.recordDir, "report.json"), js, 0o644)
}

// vtt renders the cue file landing.js reads: one whole-length head cue with
// the song's beat clock and every preset's resolved palette (read from the
// live theme resolver, not hand-copied), then the caption and theme cues, each
// a JSON payload.
func (r *tourRunner) vtt(end float64) (string, error) {
	head := tourCue{start: 0, end: end, payload: map[string]any{
		"song":     map[string]any{"bpm": 60 / r.beatSec, "beat0": r.beat0},
		"palettes": tourPalettes(),
	}}
	cues := append([]tourCue{head}, r.cues...)
	var b strings.Builder
	b.WriteString("WEBVTT\n\n")
	for _, c := range cues {
		if c.end <= c.start {
			c.end = c.start + 0.05
		}
		js, err := json.Marshal(c.payload)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "%s --> %s\n%s\n\n", vttTime(c.start), vttTime(c.end), js)
	}
	return b.String(), nil
}

// firstOnsetSeconds returns the time of the first sample of a 16-bit PCM WAV
// whose magnitude exceeds frac of full scale.
func firstOnsetSeconds(path string, frac float64) (float64, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	if len(b) < 44 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return 0, fmt.Errorf("%s: not a WAV file", path)
	}
	var channels, bits, sr int
	for off := 12; off+8 <= len(b); {
		id, size := string(b[off:off+4]), int(binary.LittleEndian.Uint32(b[off+4:off+8]))
		body := b[off+8 : min(len(b), off+8+size)]
		switch id {
		case "fmt ":
			channels = int(binary.LittleEndian.Uint16(body[2:4]))
			sr = int(binary.LittleEndian.Uint32(body[4:8]))
			bits = int(binary.LittleEndian.Uint16(body[14:16]))
		case "data":
			if bits != 16 || channels <= 0 || sr <= 0 {
				return 0, fmt.Errorf("%s: want 16-bit PCM, got %d-bit/%dch", path, bits, channels)
			}
			thr := int16(frac * 32767)
			for i := 0; i+2 <= len(body); i += 2 {
				v := int16(binary.LittleEndian.Uint16(body[i : i+2]))
				if v > thr || v < -thr {
					return float64(i/2/channels) / float64(sr), nil
				}
			}
			return 0, fmt.Errorf("%s: silent", path)
		}
		off += 8 + size + size%2
	}
	return 0, fmt.Errorf("%s: no data chunk", path)
}

func vttTime(s float64) string {
	ms := int(s*1000 + 0.5)
	return fmt.Sprintf("%02d:%02d:%02d.%03d", ms/3600000, ms/60000%60, ms/1000%60, ms%1000)
}

func hexRGB(c color.RGBA) string { return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B) }

// tourPalettes resolves every preset to the colours the landing needs.
func tourPalettes() []map[string]any {
	var out []map[string]any
	for _, p := range themePresets {
		tok, _ := resolveThemeTokens(theme.Doc{Preset: p.ID})
		out = append(out, map[string]any{
			"id": p.ID, "name": tourPresetName(p.ID),
			"bg": hexRGB(tok["background"]), "acc": hexRGB(tok["primary"]),
			// Six vivid slots (probed per preset: viz-wave-b, node-fill and
			// edge-color resolve near the background and would vanish).
			"cols": []string{hexRGB(tok["focus-ring"]), hexRGB(tok["viz-wave-a"]), hexRGB(tok["viz-bass"]), hexRGB(tok["primary"]), hexRGB(tok["viz-treble"]), hexRGB(tok["primary-bright"])},
		})
	}
	return out
}

func tourPresetName(id string) string {
	parts := strings.Split(id, "-")
	for i, p := range parts {
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}

func tourChapterLabel(id string) string {
	if id == "" {
		return ""
	}
	return strings.ToUpper(id[:1]) + id[1:]
}

// ─── input primitives (queue frames; the next Updates replay them) ───

// A real pointer always arrives before it presses: hover first, so handlers
// that key on the previous frame's hover (the splitter, row dividers) see it.
const tourHoverFrames = 4

func (r *tourRunner) tap(p image.Point) {
	r.hover(p, tourHoverFrames)
	for i := 0; i < 3; i++ {
		r.queue = append(r.queue, tourFrame{x: p.X, y: p.Y, down: true})
	}
	r.queue = append(r.queue, tourFrame{x: p.X, y: p.Y}, tourFrame{x: p.X, y: p.Y})
}

// drag presses at a, glides to b over `frames` frames, releases.
func (r *tourRunner) drag(a, b image.Point, frames int) {
	r.hover(a, tourHoverFrames)
	r.queue = append(r.queue, tourFrame{x: a.X, y: a.Y, down: true}, tourFrame{x: a.X, y: a.Y, down: true})
	for i := 1; i <= frames; i++ {
		u := easeInOut(float64(i) / float64(frames))
		r.queue = append(r.queue, tourFrame{x: a.X + int(float64(b.X-a.X)*u), y: a.Y + int(float64(b.Y-a.Y)*u), down: true})
	}
	r.queue = append(r.queue, tourFrame{x: b.X, y: b.Y, down: true}, tourFrame{x: b.X, y: b.Y})
}

// wheel scrolls `notches` detents at p, one per `every` frames.
func (r *tourRunner) wheel(p image.Point, notches int, every int) {
	dir := 1.0
	if notches < 0 {
		dir, notches = -1, -notches
	}
	for i := 0; i < notches; i++ {
		r.queue = append(r.queue, tourFrame{x: p.X, y: p.Y, wheelY: dir})
		for k := 1; k < every; k++ {
			r.queue = append(r.queue, tourFrame{x: p.X, y: p.Y})
		}
	}
}

// wheelSmooth zooms by `notchesPerFrame` detents a frame for `frames` frames
// (a native detent is a ~0.5% zoom, so a visible zoom needs several a frame).
func (r *tourRunner) wheelSmooth(p image.Point, notchesPerFrame float64, frames int) {
	for i := 0; i < frames; i++ {
		r.queue = append(r.queue, tourFrame{x: p.X, y: p.Y, wheelY: notchesPerFrame})
	}
}

func (r *tourRunner) hover(p image.Point, frames int) {
	for i := 0; i < frames; i++ {
		r.queue = append(r.queue, tourFrame{x: p.X, y: p.Y})
	}
}

func easeInOut(u float64) float64 { return u * u * (3 - 2*u) }

func tourCenter(rc image.Rectangle) image.Point {
	return image.Pt(rc.Min.X+rc.Dx()/2, rc.Min.Y+rc.Dy()/2)
}

func needRect(what string, rc image.Rectangle) (image.Point, error) {
	if rc.Empty() {
		return image.Point{}, fmt.Errorf("%s is not on screen", what)
	}
	return tourCenter(rc), nil
}
