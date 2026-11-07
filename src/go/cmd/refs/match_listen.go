//go:build !js

package main

// -listen plays each recording then its closest voice (blocking player, never overlapping), asks y/p/n plus a note on the
// SYNTH (the tuning brief), and on a rejection plays the runners-up: "not that one, but this one" is the most useful
// answer. -validate grades the matcher on catalogued references, since a rejection informs only if it is usually right.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// NOT ear_verdicts.jsonl: those grade a family's catalogued reference and feed refs validate.
const matchVerdictsRel = "internal/audio/testdata/wav_match_verdicts.jsonl"

// matchVerdict is one line of wav_match_verdicts.jsonl.
type matchVerdict struct {
	Wav    string  `json:"wav"`
	SHA256 string  `json:"sha256"`
	F0Hz   float64 `json:"f0_hz,omitempty"`
	Note   string  `json:"pitch,omitempty"`
	// Match is the instrument the analysis ranked first; Verdict is the
	// ear's judgement of it (y / p / n).
	Match       instrument.ID `json:"match"`
	MatchScore  float64       `json:"match_score"`
	RenderPitch float64       `json:"render_pitch"`
	Verdict     string        `json:"verdict"`
	// Better is a runner-up the listener heard as closer after rejecting the
	// match ("" = none was).
	Better string `json:"better,omitempty"`
	// Comment is the listener's note about the synth: what to tune, or what
	// a better variant would need.
	Comment string `json:"comment,omitempty"`
	// SoundHash pins the exact shipped document the match was heard as
	// (InstrumentSoundHash), so a later retune can tell its verdict is stale.
	SoundHash string   `json:"sound_hash"`
	Ranking   []string `json:"ranking"`
	// Always "essence" now; older rows may name a removed distance.
	Metric string `json:"metric"`
	Date   string `json:"date"`
}

func loadMatchVerdicts(path string) ([]matchVerdict, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []matchVerdict
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var v matchVerdict
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			return nil, fmt.Errorf("refs match: %s: %w", path, err)
		}
		out = append(out, v)
	}
	return out, sc.Err()
}

// recordMatchVerdict writes v, superseding any earlier verdict for the same
// recording, so re-listening never leaves two rows for one file.
func recordMatchVerdict(path string, v matchVerdict) error {
	all, err := loadMatchVerdicts(path)
	if err != nil {
		return err
	}
	var b strings.Builder
	for _, old := range all {
		if old.SHA256 == v.SHA256 {
			continue
		}
		line, _ := json.Marshal(old)
		b.Write(line)
		b.WriteByte('\n')
	}
	line, err := json.Marshal(v)
	if err != nil {
		return err
	}
	b.Write(line)
	b.WriteByte('\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// matchClips is what one prompt can replay.
type matchClips struct {
	p        player
	out      io.Writer
	ref      string
	match    string
	alts     []string // runner-up clip paths, rank 2..
	altNames []string
}

func (c matchClips) play(label, path string) {
	if path == "" {
		return
	}
	fmt.Fprintf(c.out, "▶ %s\n", label)
	_ = c.p.Play(path)
}

func (c matchClips) playAlts() {
	for i, p := range c.alts {
		c.play(fmt.Sprintf("RUNNER-UP %d: %s", i+2, c.altNames[i]), p)
	}
}

// Letters only, so they never collide with an answer (y/p/n, or a digit).
func (c matchClips) replay(line, matchName string) bool {
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "r", "replay", "again":
		c.play("REFERENCE", c.ref)
		c.play("MATCH: "+matchName, c.match)
	case "ref":
		c.play("REFERENCE", c.ref)
	case "m":
		c.play("MATCH: "+matchName, c.match)
	case "alt":
		c.playAlts()
	default:
		return false
	}
	return true
}

const matchReplayHint = "  replay: r = reference + match, ref, m, alt = runners-up; s = skip, q = quit\n" +
	"  add a note about the SYNTH after your answer, e.g. `p, attack too soft and the tail is too short`\n"

// promptMatch asks for y/p/n on the match. Returns the canonical verdict and
// note, or a control kind (skip/quit).
func promptMatch(in io.Reader, out io.Writer, c matchClips, e matchEntry) (verdictKind, string, string) {
	name := e.Ranking[0].Instrument
	for {
		fmt.Fprintf(out, "Is %s a match? y = approve  p = partly  n = reject\n%s> ", name, matchReplayHint)
		line, ok := readLine(in)
		if !ok {
			return verdictQuit, "", ""
		}
		if c.replay(line, string(name)) {
			continue
		}
		tok, note := splitChoice(line)
		switch canonicalChoice(tok) {
		case "y", "1":
			return verdictRecorded, "y", note
		case "p":
			return verdictRecorded, "p", note
		case "n":
			return verdictRecorded, "n", note
		case "s":
			noteDiscarded(out, note, "skipping")
			return verdictSkipped, "", ""
		case "q":
			noteDiscarded(out, note, "quitting")
			return verdictQuit, "", ""
		}
		fmt.Fprintf(out, "invalid answer %q -- want y, p or n (optionally followed by a note)\n", line)
	}
}

// promptBetter asks, after a rejection, whether a runner-up is closer.
// Returns the chosen instrument ("" = none) and any extra note.
func promptBetter(in io.Reader, out io.Writer, c matchClips, e matchEntry) (verdictKind, string, string) {
	if len(c.alts) == 0 {
		return verdictRecorded, "", ""
	}
	c.playAlts()
	name := e.Ranking[0].Instrument
	for {
		fmt.Fprint(out, "Is a runner-up closer? ")
		for i, n := range c.altNames {
			fmt.Fprintf(out, "%d = %s  ", i+2, n)
		}
		fmt.Fprintf(out, "n = none\n%s> ", matchReplayHint)
		line, ok := readLine(in)
		if !ok {
			return verdictQuit, "", ""
		}
		if c.replay(line, string(name)) {
			continue
		}
		tok, note := splitChoice(line)
		switch t := canonicalChoice(tok); t {
		case "n":
			return verdictRecorded, "", note
		case "s":
			noteDiscarded(out, note, "skipping")
			return verdictSkipped, "", ""
		case "q":
			noteDiscarded(out, note, "quitting")
			return verdictQuit, "", ""
		default:
			for i, n := range c.altNames {
				if t == fmt.Sprint(i+2) {
					return verdictRecorded, n, note
				}
			}
		}
		fmt.Fprintf(out, "invalid answer %q -- want a runner-up number or n\n", line)
	}
}

func joinNotes(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	}
	return a + "; " + b
}

// newPlayer is injected so the whole loop runs under test.
func runMatchListen(out io.Writer, in io.Reader, man matchManifest, verdictsPath string, resume bool, only string, top int, newPlayer func(durSec float64) player) error {
	prior, err := loadMatchVerdicts(verdictsPath)
	if err != nil {
		return err
	}
	answered := map[string]bool{}
	for _, v := range prior {
		answered[v.SHA256] = true
	}
	var pending []matchEntry
	filtered, skipped := 0, 0
	for _, e := range man.Entries {
		if only != "" && !strings.Contains(strings.ToLower(e.Name), strings.ToLower(only)) {
			filtered++
			continue
		}
		if resume && answered[e.SHA256] {
			skipped++
			continue
		}
		if len(e.Ranking) == 0 {
			continue
		}
		pending = append(pending, e)
	}
	if len(pending) == 0 {
		switch {
		case only != "" && filtered == len(man.Entries):
			fmt.Fprintf(out, "nothing to listen to -- no recording's filename contains %q (of %d)\n", only, len(man.Entries))
		case skipped > 0:
			fmt.Fprintf(out, "nothing to listen to -- %d recording(s) already have a verdict (RESUME=false to hear them again)\n", skipped)
		default:
			fmt.Fprintln(out, "nothing to listen to -- no recording has a ranking")
		}
		return nil
	}
	fmt.Fprintf(out, "\n%d recording(s) to judge; verdicts -> %s\n", len(pending), verdictsPath)
	fmt.Fprintln(out, "each clip is level-matched to the reference, so judge timbre, not loudness")
	for n, e := range pending {
		pitch := "unpitched (each instrument tried within an octave of its natural pitch, the closest kept)"
		if e.Pitched {
			pitch = fmt.Sprintf("%s (%.1f Hz) -- every instrument rendered at this note", e.Note, e.F0Hz)
		}
		fmt.Fprintf(out, "\n=== [%d/%d] %s ===\n    pitch: %s\n", n+1, len(pending), e.Name, pitch)
		for i, r := range e.Ranking {
			if i >= top && r.Instrument != e.LabelBest {
				continue
			}
			tag := ""
			if r.Instrument == e.LabelBest {
				tag = "  <- what the recording is labelled as"
			}
			fmt.Fprintf(out, "    #%-3d %-22s score %.3f%s\n", i+1, r.Instrument, r.Score, tag)
		}
		c := matchClips{p: newPlayer(e.DurationSec), out: out, ref: e.Reference, match: e.Ranking[0].Clip}
		for i, r := range e.Ranking {
			if i == 0 || r.Clip == "" || (i >= top && r.Instrument != e.LabelBest) {
				continue
			}
			c.alts = append(c.alts, r.Clip)
			c.altNames = append(c.altNames, string(r.Instrument))
		}
		c.replay("r", string(e.Ranking[0].Instrument))

		kind, verdict, note := promptMatch(in, out, c, e)
		if kind == verdictQuit {
			fmt.Fprintln(out, "quitting -- answers so far are saved")
			return nil
		}
		if kind == verdictSkipped {
			fmt.Fprintln(out, "skipped")
			continue
		}
		better := ""
		if verdict == "n" {
			k, b, more := promptBetter(in, out, c, e)
			if k == verdictQuit {
				fmt.Fprintln(out, "quitting -- this recording was NOT saved; answers before it are")
				return nil
			}
			if k == verdictSkipped {
				fmt.Fprintln(out, "skipped")
				continue
			}
			better, note = b, joinNotes(note, more)
		}
		var names []string
		for _, r := range e.Ranking {
			names = append(names, string(r.Instrument))
		}
		if len(names) > 10 {
			names = names[:10]
		}
		top1 := e.Ranking[0]
		v := matchVerdict{
			Wav: e.Name, SHA256: e.SHA256, F0Hz: e.F0Hz, Note: e.Note,
			Match: top1.Instrument, MatchScore: top1.Score, RenderPitch: top1.RenderPitch,
			Verdict: verdict, Better: better, Comment: note,
			SoundHash: audio.InstrumentSoundHash(top1.Instrument, nil),
			Ranking:   names, Metric: "essence", Date: time.Now().Format("2006-01-02"),
		}
		if err := recordMatchVerdict(verdictsPath, v); err != nil {
			return err
		}
		msg := map[string]string{"y": "approved", "p": "partly approved", "n": "rejected"}[verdict]
		fmt.Fprintf(out, "recorded: %s %s", top1.Instrument, msg)
		if better != "" {
			fmt.Fprintf(out, ", %s is closer", better)
		}
		if note != "" {
			fmt.Fprintf(out, " (note: %q)", note)
		}
		fmt.Fprintln(out)
	}
	fmt.Fprintln(out, "\ndone -- every recording in this pass has an answer")
	return nil
}

// matchCorpusRow is one catalogued reference ranked against the roster.
type matchCorpusRow struct {
	id, class, kind string
	f0              float64
	// want is the reference's own family (its instruments and every family
	// member of them).
	want   map[instrument.ID]bool
	ranked []matchRender
}

// collectMatchCorpus plays the whole roster against every catalogued
// instrument reference present on this machine, exactly as `refs match`
// plays a download.
func collectMatchCorpus(diag io.Writer, workers int, settle func(id instrument.ID) bool) ([]matchCorpusRow, error) {
	c, err := loadCatalogue()
	if err != nil {
		return nil, err
	}
	fams, err := buildAuditionFamilies(c)
	if err != nil {
		return nil, err
	}
	famOf := map[instrument.ID][]instrument.ID{}
	for _, f := range fams {
		for _, m := range f.Members {
			famOf[m] = f.Members
		}
	}
	ids := rosterIDs()
	refs := append([]wavref.Ref(nil), c.Refs...)
	sort.Slice(refs, func(i, j int) bool { return refs[i].ID < refs[j].ID })
	var rows []matchCorpusRow
	for _, r := range refs {
		if r.Status != "" || r.Tier != "instrument" || len(r.Instruments) == 0 {
			continue
		}
		path, ok := wavref.Resolve(r.ID)
		if !ok {
			continue
		}
		pcm, rate, err := audio.DecodeWAVAtRate(path, wavref.Rate)
		if err != nil {
			continue
		}
		s := make([]float64, len(pcm))
		for i, v := range pcm {
			s[i] = float64(v)
		}
		ref := wave.Wave{Samples: trimMatchClip(s, rate, adoptMaxSecDefault), SampleRate: rate, Label: r.ID}
		f0 := matchRefPitch(ref)
		ranked, err := rankRoster(ref, f0, ids, workers, settle)
		if err != nil {
			fmt.Fprintf(diag, "skip %s: %v\n", r.ID, err)
			continue
		}
		want := map[instrument.ID]bool{}
		for _, inst := range r.Instruments {
			want[inst] = true
			for _, m := range famOf[inst] {
				want[m] = true
			}
		}
		rows = append(rows, matchCorpusRow{
			id: r.ID, class: matchClassOf(ref), kind: broadClass(r.Instruments[0]), f0: f0, want: want, ranked: ranked,
		})
		fmt.Fprintf(diag, ".")
	}
	fmt.Fprintln(diag)
	return rows, nil
}

// rawEssenceRows is each row's raw essence distance per instrument.
func rawEssenceRows(rows []matchCorpusRow) []map[instrument.ID]float64 {
	out := make([]map[instrument.ID]float64, len(rows))
	for i, row := range rows {
		out[i] = map[instrument.ID]float64{}
		for _, x := range row.ranked {
			out[i][x.cand.Instrument] = x.rawEssence
		}
	}
	return out
}

// Own family first / in the top 3, MRR, and the first voice's broad KIND (a cello matched to the viola is a family miss
// but the right kind). LEAVE-ONE-OUT: hubs measured over the OTHER references.
func runMatchValidate(out, diag io.Writer, workers int) error {
	rows, err := collectMatchCorpus(diag, workers, nil)
	if err != nil {
		return err
	}
	raw := rawEssenceRows(rows)
	type tally struct {
		n, hit1, hit3, kind1, kind3 int
		rr                          float64
	}
	tallies := map[string]*tally{}
	add := func(key string, famRank, kindRank int) {
		t := tallies[key]
		if t == nil {
			t = &tally{}
			tallies[key] = t
		}
		t.n++
		if famRank == 1 {
			t.hit1++
		}
		if famRank >= 1 && famRank <= 3 {
			t.hit3++
		}
		if famRank >= 1 {
			t.rr += 1 / float64(famRank)
		}
		if kindRank == 1 {
			t.kind1++
		}
		if kindRank >= 1 && kindRank <= 3 {
			t.kind3++
		}
	}
	for ri, row := range rows {
		loo := computeHubs(raw, matchHubnessK, ri)
		score := func(x matchRender) float64 { return hubCorrected(x.rawEssence, loo[x.cand.Instrument]) }
		list := append([]matchRender(nil), row.ranked...)
		sort.SliceStable(list, func(i, j int) bool { return score(list[i]) < score(list[j]) })
		famRank, kindRank := 0, 0
		for i, x := range list {
			if famRank == 0 && row.want[x.cand.Instrument] {
				famRank = i + 1
			}
			if kindRank == 0 && row.kind != "" && broadClass(x.cand.Instrument) == row.kind {
				kindRank = i + 1
			}
		}
		add("essence", famRank, kindRank)
		add("essence "+row.class, famRank, kindRank)
		fmt.Fprintf(out, "%-26s %-10s %-4s %-10s | essence: #1 %-20s family %3d kind %3d\n",
			row.id, row.class, noteNameForHz(row.f0), row.kind, list[0].cand.Instrument, famRank, kindRank)
	}
	pct := func(k, n int) float64 { return 100 * float64(k) / float64(max(n, 1)) }
	fmt.Fprintf(out, "\n%-26s %4s  %-14s %-14s %5s  %-14s %-14s\n", "", "refs", "family #1", "family top-3", "MRR", "kind #1", "kind top-3")
	for _, key := range []string{"essence", "essence " + matchClassSustained, "essence " + matchClassPercussive} {
		t := tallies[key]
		if t == nil {
			continue
		}
		fmt.Fprintf(out, "%-26s %4d  %3d (%3.0f%%)     %3d (%3.0f%%)     %5.3f  %3d (%3.0f%%)     %3d (%3.0f%%)\n",
			key, t.n, t.hit1, pct(t.hit1, t.n), t.hit3, pct(t.hit3, t.n), t.rr/float64(max(t.n, 1)),
			t.kind1, pct(t.kind1, t.n), t.kind3, pct(t.kind3, t.n))
	}
	return nil
}
