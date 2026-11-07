//go:build !js

package main

// Plays the REFERENCE, the CURRENT shipped instrument and a few candidates back to back, then collects per candidate a
// rating, a relative verdict, the family's 1-5 questions, words from one shared vocabulary and a note: one earSession
// line per family, scored by refs validate.

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/clone"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// ---- prompt helpers (pure) --------------------------------------------------

func readChoice(in io.Reader, out io.Writer, prompt string) (string, bool) {
	fmt.Fprint(out, prompt)
	line, ok := readLine(in)
	return strings.TrimSpace(line), ok
}

// q and s mean the same at every prompt. Replay is handled in place by replayer, so a prompt re-asks the SAME question
// after any number of replays instead of the family starting over.
func controlKind(tok string) (verdictKind, bool) {
	switch tok {
	case "q":
		return verdictQuit, true
	case "s":
		return verdictSkipped, true
	}
	return verdictRecorded, false
}

// Tokens: r reference, c current, a candidate id, all, rel (the clips this question is about). The caller then re-asks
// the same prompt, keeping every answer already given.
type replayer struct {
	p     player
	out   io.Writer
	clips sessionClips
}

// keys lists every replay token, for prompt text.
func (rp *replayer) keys() string {
	var ks []string
	for _, k := range rp.clips.order {
		ks = append(ks, strings.ToLower(k))
	}
	return strings.Join(ks, " ") + " all rel"
}

// A line is a replay only if EVERY token is a replay key ("r a" plays reference then A).
func (rp *replayer) replay(tok string, relevant []string) bool {
	if rp == nil {
		return false
	}
	fields := strings.Fields(strings.ToUpper(tok))
	if len(fields) == 0 {
		return false
	}
	var seq []string
	for _, t := range fields {
		switch t {
		case "ALL":
			seq = append(seq, rp.clips.order...)
			continue
		case "REL":
			if len(relevant) == 0 {
				seq = append(seq, rp.clips.order...)
			} else {
				seq = append(seq, relevant...)
			}
			continue
		}
		known := false
		for _, k := range rp.clips.order {
			if k == t {
				known = true
				break
			}
		}
		if !known {
			return false
		}
		seq = append(seq, t)
	}
	rp.play(seq)
	return true
}

func (rp *replayer) play(seq []string) {
	for _, k := range seq {
		path := ""
		switch k {
		case "R":
			path = rp.clips.refPath
		case "C":
			path = rp.clips.curPath
		default:
			path = rp.clips.candPaths[k]
		}
		if path == "" {
			continue
		}
		fmt.Fprintf(rp.out, "▶ %s\n", k)
		_ = rp.p.Play(path)
	}
}

// promptControls is the shared trailer every prompt prints, so the listener
// sees the same vocabulary everywhere.
func promptControls(rp *replayer) string {
	keys := "r c a b all rel"
	if rp != nil {
		keys = rp.keys()
	}
	return fmt.Sprintf("(replay: %s; s skip family; q quit)", keys)
}

func promptRating(in io.Reader, out io.Writer, rp *replayer, id string) (int, verdictKind) {
	relevant := []string{"R", id}
	for {
		line, ok := readChoice(in, out, fmt.Sprintf("How close is %s to the reference? 1 nothing like it … 5 could be the real thing  %s\n> ", id, promptControls(rp)))
		if !ok {
			return 0, verdictQuit
		}
		if rp.replay(line, relevant) {
			continue
		}
		if k, ctl := controlKind(line); ctl {
			return 0, k
		}
		if n, err := strconv.Atoi(line); err == nil && n >= 1 && n <= 5 {
			return n, verdictRecorded
		}
		fmt.Fprintln(out, "invalid: want 1-5")
	}
}

// + / = / - (or the words), never letters: b is candidate B's replay key and s skips, so typing b to hear B again must
// not record "better".
func promptVsCurrent(in io.Reader, out io.Writer, rp *replayer, id string) (string, verdictKind) {
	relevant := []string{"R", "C", id}
	for {
		line, ok := readChoice(in, out, fmt.Sprintf("%s against the CURRENT version: + better  = same  - worse  %s\n> ", id, promptControls(rp)))
		if !ok {
			return "", verdictQuit
		}
		if rp.replay(line, relevant) {
			continue
		}
		if k, ctl := controlKind(line); ctl {
			return "", k
		}
		switch strings.ToLower(line) {
		case "+", "better":
			return "better", verdictRecorded
		case "=", "same", "even":
			return "same", verdictRecorded
		case "-", "worse":
			return "worse", verdictRecorded
		}
		fmt.Fprintln(out, "invalid: want +, = or - (or a replay key, s, q)")
	}
}

// Should this exact document become the shipped instrument? clone adopt reads the answer.
func promptAccept(in io.Reader, out io.Writer, rp *replayer, id string) (string, verdictKind) {
	relevant := []string{"R", "C", id}
	for {
		line, ok := readChoice(in, out, fmt.Sprintf("Ship candidate %s as the instrument? y yes  n no  p partly (needs work)  %s\n> ", id, promptControls(rp)))
		if !ok {
			return "", verdictQuit
		}
		if rp.replay(line, relevant) {
			continue
		}
		if k, ctl := controlKind(line); ctl {
			return "", k
		}
		switch strings.ToLower(line) {
		case "y", "yes":
			return "y", verdictRecorded
		case "n", "no":
			return "n", verdictRecorded
		case "p", "partly":
			return "p", verdictRecorded
		}
		fmt.Fprintln(out, "invalid: want y, n or p (or a replay key, s, q)")
	}
}

// "1 not X enough · 3 about right · 5 too X", stored centred and turned to the axis (+2 = the synth reads HIGH on
// q.Axis). A bare "-" leaves the clip unanswered (answered false).
func promptScaleOne(in io.Reader, out io.Writer, rp *replayer, q sessionQuestion, id string) (ans int, answered bool, kind verdictKind) {
	relevant := []string{"R", id}
	for {
		line, ok := readChoice(in, out, fmt.Sprintf("%s   ('-' to skip this clip)  %s\n> ", q.prompt(id), promptControls(rp)))
		if !ok {
			return 0, false, verdictQuit
		}
		if line == "-" {
			return 0, false, verdictRecorded
		}
		if rp.replay(line, relevant) {
			continue
		}
		if k, ctl := controlKind(line); ctl {
			return 0, false, k
		}
		if n, err := strconv.Atoi(line); err == nil && n >= 1 && n <= 5 {
			return q.storedAnswer(n), true, verdictRecorded
		}
		fmt.Fprintln(out, "invalid: want a single number 1-5 (or '-', a replay key, s, q)")
	}
}

// promptScale asks one targeted question across every candidate, one clip at
// a time, so the listener only ever types a number. Clips skipped with "-"
// are absent from the returned map.
func promptScale(in io.Reader, out io.Writer, rp *replayer, q sessionQuestion, ids []string) (map[string]int, verdictKind) {
	ans := map[string]int{}
	for _, id := range ids {
		v, answered, k := promptScaleOne(in, out, rp, q, id)
		if k != verdictRecorded {
			return nil, k
		}
		if answered {
			ans[id] = v
		}
	}
	return ans, verdictRecorded
}

// promptAdjectivesOne asks for the words that describe ONE clip as numbers
// from the shared word salad ("3 7"); "-" or an empty line means none.
func promptAdjectivesOne(in io.Reader, out io.Writer, rp *replayer, vocab []string, id string) ([]string, verdictKind) {
	menu := wordMenu(vocab)
	relevant := []string{"R", id}
	for {
		line, ok := readChoice(in, out, fmt.Sprintf("Words for %s — every number that fits ('-' or Enter for none)  %s%s\n> ", id, promptControls(rp), menu))
		if !ok {
			return nil, verdictQuit
		}
		if line == "-" || line == "" {
			return nil, verdictRecorded
		}
		if rp.replay(line, relevant) {
			continue
		}
		if k, ctl := controlKind(line); ctl {
			return nil, k
		}
		var res []string
		bad := false
		for _, t := range strings.Fields(line) {
			n, err := strconv.Atoi(t)
			if err != nil || n < 1 || n > len(vocab) {
				bad = true
				break
			}
			res = append(res, vocab[n-1])
		}
		if bad {
			fmt.Fprintln(out, "invalid: numbers from the list only (or '-', a replay key, s, q)")
			continue
		}
		return res, verdictRecorded
	}
}

// promptAdjectives collects the words per clip, one clip at a time.
func promptAdjectives(in io.Reader, out io.Writer, rp *replayer, vocab []string, ids []string) (map[string][]string, verdictKind) {
	res := map[string][]string{}
	for _, id := range ids {
		adj, k := promptAdjectivesOne(in, out, rp, vocab, id)
		if k != verdictRecorded {
			return nil, k
		}
		if len(adj) > 0 {
			res[id] = adj
		}
	}
	return res, verdictRecorded
}

// Which version should SHIP, asked apart from closeness: the listener may prefer a synth nothing like the reference.
// Digits (0 keep current), because the candidate letters are replay keys.
func promptShip(in io.Reader, out io.Writer, rp *replayer, ids []string) (string, verdictKind) {
	var opts []string
	for i, id := range ids {
		opts = append(opts, fmt.Sprintf("%d ship %s", i+1, id))
	}
	for {
		line, ok := readChoice(in, out, fmt.Sprintf("Regardless of how close it is to the reference, which version should SHIP? 0 keep the current instrument · %s  %s\n> ", strings.Join(opts, " · "), promptControls(rp)))
		if !ok {
			return "", verdictQuit
		}
		if rp.replay(line, nil) {
			continue
		}
		if k, ctl := controlKind(line); ctl {
			return "", k
		}
		if n, err := strconv.Atoi(line); err == nil && n >= 0 && n <= len(ids) {
			if n == 0 {
				return "C", verdictRecorded
			}
			return ids[n-1], verdictRecorded
		}
		fmt.Fprintf(out, "invalid: want 0-%d (or a replay key, s, q)\n", len(ids))
	}
}

// Controls only as a single bare token: a note opening with one of those letters is prose and is kept.
func promptNote(in io.Reader, out io.Writer, rp *replayer) (string, verdictKind) {
	for {
		line, ok := readChoice(in, out, fmt.Sprintf("Anything else? (Enter to skip)  %s\n> ", promptControls(rp)))
		if !ok {
			return "", verdictQuit
		}
		if len(strings.Fields(line)) == 1 {
			if rp.replay(line, nil) {
				continue
			}
			if k, ctl := controlKind(line); ctl {
				return "", k
			}
		}
		return line, verdictRecorded
	}
}

// ---- the session ------------------------------------------------------------

// sessionClips is one family's rendered clip set: R the real reference, C the
// current shipped instrument, then one entry per candidate. order fixes the
// playback sequence so "all" always plays reference-first.
type sessionClips struct {
	refPath, curPath string
	candPaths        map[string]string
	order            []string // "R","C", then candidate ids
}

// promptReplayOrRate is the opening prompt after the first full playback:
// replay anything, or press Enter to start rating.
func promptReplayOrRate(in io.Reader, out io.Writer, rp *replayer) verdictKind {
	for {
		line, ok := readChoice(in, out, fmt.Sprintf("Enter to rate  %s\n> ", promptControls(rp)))
		if !ok {
			return verdictQuit
		}
		if line == "" {
			return verdictRecorded
		}
		if rp.replay(line, nil) {
			continue
		}
		if k, ctl := controlKind(line); ctl {
			return k
		}
		fmt.Fprintln(out, "invalid")
	}
}

// One derivation for prompt keys, clip keys and record ids, so they cannot drift apart.
func sessionIDs(fc familyCandidates) []string {
	ids := make([]string, 0, len(fc.Candidates))
	for _, cand := range fc.Candidates {
		ids = append(ids, strings.ToUpper(cand.ID))
	}
	return ids
}

// sessionRenderMeta is what the audition header and the recorded session need
// from the render besides the paths themselves.
type sessionRenderMeta struct {
	Pitch float64
	Dur   float64
}

// Injected, so the whole prompt/record flow is tested without rendering audio.
type sessionRenderFn func(fc familyCandidates, ref wavref.Ref) (sessionClips, sessionRenderMeta, error)

// Index 0 is the CURRENT render; one call level-matches every clip against a single reference copy.
func renderSessionClips(fc familyCandidates, ref wavref.Ref) (sessionClips, sessionRenderMeta, error) {
	specs := []compareSpec{{Inst: fc.Base}}
	for _, cand := range fc.Candidates {
		sp := compareSpec{Inst: fc.Base, Overrides: cand.Overrides}
		if cand.Doc != nil {
			sp.Doc, sp.DocSHA = cand.Doc, cand.DocSHA
			if !cand.SamePitch {
				p := cand.FitPitch
				sp.Pitch = &p
			}
		}
		specs = append(specs, sp)
	}
	results, err := renderFamilyCompareSpecs(os.Stderr, specs, ref, 0, matchRMS)
	if err != nil {
		return sessionClips{}, sessionRenderMeta{}, err
	}
	clips := sessionClips{refPath: results[0].RealPath, curPath: results[0].SynthPath, candPaths: map[string]string{}, order: []string{"R", "C"}}
	for i, id := range sessionIDs(fc) {
		clips.candPaths[id] = results[i+1].SynthPath
		clips.order = append(clips.order, id)
	}
	return clips, sessionRenderMeta{Pitch: results[0].Pitch, Dur: results[0].Dur}, nil
}

// ---- drafts: nothing the listener has answered is ever lost ----------------

// Saved after EVERY accepted answer, so a quit or crash costs nothing: the next run resumes at the first unanswered
// prompt. Done is keyed by prompt ("rating:A", "vs:A", "scale:<key>:A", "adj:A", "note").
type sessionDraft struct {
	Session earSession      `json:"session"`
	Done    map[string]bool `json:"done"`
}

func draftPath(outPath string, family instrument.ID) string {
	return outPath + "." + string(family) + ".draft.json"
}

// The family's draft when none exists or it is for these candidates; otherwise a draft of its own beside it, so
// another round's answers are never resumed into the wrong sound, overwritten or removed.
func draftFile(outPath string, family instrument.ID, fc familyCandidates) string {
	p := draftPath(outPath, family)
	d, ok := loadDraft(p)
	if _, err := os.Stat(p); err != nil || (ok && draftIsFor(d, fc)) {
		return p
	}
	h := sha256.New()
	for i, cand := range fc.Candidates {
		c := candidateClip(fc, cand, sessionIDs(fc)[i])
		fmt.Fprintf(h, "%s=%s;", c.ID, c.RecipeSHA)
	}
	return fmt.Sprintf("%s.%s.%x.draft.json", outPath, family, h.Sum(nil)[:4])
}

// draftIsFor reports whether d audits exactly fc's candidates.
func draftIsFor(d sessionDraft, fc familyCandidates) bool {
	if len(d.Session.Candidates) != len(fc.Candidates) {
		return false
	}
	for i, cand := range fc.Candidates {
		want := candidateClip(fc, cand, sessionIDs(fc)[i])
		if got := d.Session.Candidates[i]; got.ID != want.ID || got.RecipeSHA != want.RecipeSHA {
			return false
		}
	}
	return true
}

func loadDraft(path string) (sessionDraft, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return sessionDraft{}, false
	}
	var d sessionDraft
	if err := json.Unmarshal(b, &d); err != nil || d.Done == nil {
		return sessionDraft{}, false
	}
	return d, true
}

func saveDraft(path string, d sessionDraft) error {
	b, err := json.Marshal(d)
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, b, 0o644)
}

func removeDraft(path string) { _ = os.Remove(path) }

// candidateClip is how a candidate is recorded: its id and the hash of the
// sound auditioned (a whole document's doc_sha, or the base plus overrides).
func candidateClip(fc familyCandidates, cand candidateSpec, id string) sessionClip {
	if cand.Doc != nil {
		return sessionClip{ID: id, Base: fc.Base, RecipeSHA: cand.DocSHA, DocSHA: cand.DocSHA}
	}
	return sessionClip{ID: id, Base: fc.Base, RecipeSHA: audio.InstrumentSoundHash(fc.Base, cand.Overrides), Overrides: cand.Overrides}
}

// The latest record that reviewed EXACTLY these candidates (ids and sound hashes); a re-tuned candidate is pending
// again, and a deep round counts only deep records.
func reviewedSession(records []earSession, family instrument.ID, fc familyCandidates, deep bool) (earSession, bool) {
	ids := sessionIDs(fc)
	for i := len(records) - 1; i >= 0; i-- {
		s := records[i]
		if s.Family != family || len(s.Candidates) != len(fc.Candidates) || (deep && s.Mode != sessionModeDeep) {
			continue
		}
		same := true
		for j, cand := range fc.Candidates {
			want := candidateClip(fc, cand, ids[j])
			if s.Candidates[j].ID != want.ID || s.Candidates[j].RecipeSHA != want.RecipeSHA {
				same = false
				break
			}
		}
		if same {
			return s, true
		}
	}
	return earSession{}, false
}

// Prompts run in sequence, each re-asking after a replay, and every answer goes to the draft, so nothing is lost across
// replays, quits or crashes. Quick: rating, vs current, the word salad, ship, note; deep adds the instrument's own 1-5
// word ratings.
func runSessionFamily(w io.Writer, in io.Reader, p player, name instrument.ID, fc familyCandidates, ref wavref.Ref, render sessionRenderFn, outPath string, planOnly, deep bool) (verdictKind, error) {
	ids := sessionIDs(fc)
	clips, meta, err := render(fc, ref)
	if err != nil {
		return verdictQuit, err
	}
	fmt.Fprintf(w, "\n=== %s === ref=%s pitch=%+.2f dur=%.2fs\n", name, ref.ID, meta.Pitch, meta.Dur)
	fmt.Fprintf(w, "    R reference  C current (%s)", fc.Base)
	for _, cand := range fc.Candidates {
		fmt.Fprintf(w, "  %s %s", strings.ToUpper(cand.ID), cand.Label)
	}
	fmt.Fprintln(w)
	mode, words := sessionModeQuick, ""
	if deep {
		mode = sessionModeDeep
		for _, q := range deepQuestions(name) {
			words += " " + q.Text
		}
	}
	if deep {
		fmt.Fprintf(w, "    deep round: rate, compare, then 1-5 on%s, the word salad, ship, note\n", words)
	} else {
		fmt.Fprintln(w, "    quick round: rate, compare, the word salad, ship, note (DEEP=1 also rates this instrument's own words 1-5)")
	}
	if planOnly {
		fmt.Fprintf(w, "    R=%s\n    C=%s\n", clips.refPath, clips.curPath)
		for _, id := range ids {
			fmt.Fprintf(w, "    %s=%s\n", id, clips.candPaths[id])
		}
		return verdictRecorded, nil
	}

	draft := draftFile(outPath, name, fc)
	if draft != draftPath(outPath, name) {
		fmt.Fprintf(w, "    %s holds a draft of OTHER candidates (another round); it is kept as it is, and this round drafts to %s\n", draftPath(outPath, name), filepath.Base(draft))
	}
	d, resumed := loadDraft(draft)
	if !resumed {
		d = sessionDraft{
			Session: earSession{
				Session: time.Now().UTC().Format(time.RFC3339), Family: name, Reference: ref.ID, Mode: mode,
				SemisFromA3: ref.Measured.SemisFromA3, DurSec: meta.Dur,
				Current: sessionClip{ID: string(fc.Base), RecipeSHA: audio.InstrumentSoundHash(fc.Base, nil)},
			},
			Done: map[string]bool{},
		}
		for i, cand := range fc.Candidates {
			d.Session.Candidates = append(d.Session.Candidates, sessionCandidate{sessionClip: candidateClip(fc, cand, ids[i]), Label: cand.Label})
		}
	} else {
		if d.Session.Mode == "" {
			d.Session.Mode = mode // a draft from before the two rounds
		}
		fmt.Fprintf(w, "    resuming: %d answers kept from the previous run; picking up at the first unanswered question\n", len(d.Done))
	}
	keep := func() error { return saveDraft(draft, d) }

	rp := &replayer{p: p, out: w, clips: clips}
	rp.play(clips.order)
	if k := promptReplayOrRate(in, w, rp); k != verdictRecorded {
		if k == verdictSkipped {
			removeDraft(draft)
		}
		return k, nil
	}
	// A skip discards the family's draft, a quit keeps it.
	finish := func(k verdictKind) (verdictKind, error) {
		if k == verdictSkipped {
			removeDraft(draft)
		}
		return k, nil
	}
	for i := range d.Session.Candidates {
		c := &d.Session.Candidates[i]
		id := c.ID
		if !d.Done["rating:"+id] {
			rating, k := promptRating(in, w, rp, id)
			if k != verdictRecorded {
				return finish(k)
			}
			c.Rating = rating
			d.Done["rating:"+id] = true
			if err := keep(); err != nil {
				return verdictQuit, err
			}
		}
		if !d.Done["vs:"+id] {
			vs, k := promptVsCurrent(in, w, rp, id)
			if k != verdictRecorded {
				return finish(k)
			}
			c.VsCurrent = vs
			d.Done["vs:"+id] = true
			if err := keep(); err != nil {
				return verdictQuit, err
			}
		}
		if c.DocSHA != "" && !d.Done["accept:"+id] {
			acc, k := promptAccept(in, w, rp, id)
			if k != verdictRecorded {
				return finish(k)
			}
			c.Accept = acc
			d.Done["accept:"+id] = true
			if err := keep(); err != nil {
				return verdictQuit, err
			}
		}
	}
	var deepQs []sessionQuestion
	if d.Session.Mode == sessionModeDeep {
		deepQs = deepQuestions(name)
	}
	for _, q := range deepQs {
		for i := range d.Session.Candidates {
			c := &d.Session.Candidates[i]
			key := "scale:" + q.Key + ":" + c.ID
			if d.Done[key] {
				continue
			}
			v, answered, k := promptScaleOne(in, w, rp, q, c.ID)
			if k != verdictRecorded {
				return finish(k)
			}
			if answered {
				c.Scales = append(c.Scales, sessionScale{Q: q.Text, Axis: q.Axis, Answer: v})
			}
			d.Done[key] = true
			if err := keep(); err != nil {
				return verdictQuit, err
			}
		}
	}
	vocab := saladWords()
	for i := range d.Session.Candidates {
		c := &d.Session.Candidates[i]
		if d.Done["adj:"+c.ID] {
			continue
		}
		adj, k := promptAdjectivesOne(in, w, rp, vocab, c.ID)
		if k != verdictRecorded {
			return finish(k)
		}
		c.Adjectives = adj
		d.Done["adj:"+c.ID] = true
		if err := keep(); err != nil {
			return verdictQuit, err
		}
	}
	if !d.Done["ship"] {
		ship, k := promptShip(in, w, rp, ids)
		if k != verdictRecorded {
			return finish(k)
		}
		d.Session.Ship = ship
		d.Done["ship"] = true
		if err := keep(); err != nil {
			return verdictQuit, err
		}
	}
	if !d.Done["note"] {
		note, k := promptNote(in, w, rp)
		if k != verdictRecorded {
			return finish(k)
		}
		d.Session.Note = note
		d.Done["note"] = true
	}
	if err := appendSession(outPath, d.Session); err != nil {
		return verdictQuit, err
	}
	removeDraft(draft)
	fmt.Fprintf(w, "recorded %s → %s\n", name, outPath)
	return verdictRecorded, nil
}

// docCandidateKind is one source of whole-document candidates: the WAV clones
// (`make clone`, candidate K) and the essence-tuned documents (`make tune`,
// candidate T). Both directories hold candidate.json + meta.json.
type docCandidateKind struct {
	id, label, hypothesis, makeHint string
}

var (
	cloneCandidates = docCandidateKind{id: "k", label: "WAV clone", hypothesis: "analysis → resynthesis clone of the reference", makeHint: "make clone INST=%s"}
	tunedCandidates = docCandidateKind{id: "t", label: "essence-tuned", hypothesis: "the document's own values moved toward every recording of it (refs tune)", makeHint: "make tune INST=%s"}
)

// A family absent from the candidates file auditions the clone alone, against the reference it was fitted to.
func withClone(fc familyCandidates, inFile bool, name instrument.ID, clonesRoot string) (familyCandidates, error) {
	return withDocCandidate(fc, inFile, name, clonesRoot, cloneCandidates)
}

// A tuned candidate (same_pitch) plays at the current voice's note, keeping the shipped pitch frame; a clone at its fit
// pitch.
func withDocCandidate(fc familyCandidates, inFile bool, name instrument.ID, root string, kind docCandidateKind) (familyCandidates, error) {
	dir := filepath.Join(root, string(name))
	b, err := os.ReadFile(filepath.Join(dir, "candidate.json"))
	if err != nil {
		return fc, fmt.Errorf("refs session: %s has no %s candidate (run `"+kind.makeHint+"`): %w", name, kind.label, name, err)
	}
	doc, err := instrument.ParseFile(b)
	if err != nil {
		return fc, fmt.Errorf("refs session: %s/candidate.json: %w", dir, err)
	}
	var meta struct {
		Ref       string  `json:"ref"`
		FitPitch  float64 `json:"fit_pitch"`
		SamePitch bool    `json:"same_pitch"`
		Why       string  `json:"why"`
	}
	mb, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err == nil {
		err = json.Unmarshal(mb, &meta)
	}
	if err != nil {
		return fc, fmt.Errorf("refs session: %s/meta.json: %w", dir, err)
	}
	if !inFile {
		if meta.Ref == "" {
			return fc, fmt.Errorf("refs session: %s's %s candidate was fitted to a -wav file, not a catalogue reference — nothing to audition it against", name, kind.label)
		}
		fc = familyCandidates{Reference: meta.Ref, Base: name}
	}
	hyp := kind.hypothesis
	if meta.Why != "" {
		hyp += ": " + meta.Why
	}
	fc.Candidates = append(append([]candidateSpec(nil), fc.Candidates...), candidateSpec{
		ID: kind.id, Label: kind.label, Hypothesis: hyp,
		Doc: &doc, DocSHA: clone.DocSHA(doc), FitPitch: meta.FitPitch, SamePitch: meta.SamePitch,
	})
	return fc, nil
}

// cloneFamilies lists every instrument with a clone candidate under clonesRoot.
func cloneFamilies(clonesRoot string) []instrument.ID {
	var out []instrument.ID
	ents, _ := os.ReadDir(clonesRoot)
	for _, e := range ents {
		if _, err := os.Stat(filepath.Join(clonesRoot, e.Name(), "candidate.json")); e.IsDir() && err == nil {
			out = append(out, instrument.ID(e.Name())) // a candidate directory is named by its instrument
		}
	}
	slices.Sort(out)
	return out
}

// clonesRoot adds each family's WAV clone (K); tunedRoot auditions its tuned candidate (T) ALONE, played as refs tune
// judged it. resume skips a family already reviewed for exactly its candidates; a named family is always asked; answers
// are APPENDED, nothing rewritten.
func runSession(w io.Writer, in io.Reader, p player, families []instrument.ID, candPath, outPath string, planOnly, deep, resume bool, clonesRoot, tuned string) error {
	all, err := loadCandidates(candPath)
	if err != nil {
		return err
	}
	c, err := loadCatalogue()
	if err != nil {
		return err
	}
	named := len(families) > 0
	var records []earSession
	if resume && !named {
		if records, err = readSessions(outPath); err != nil {
			return err
		}
	}
	var skipped []string
	if len(families) == 0 {
		switch {
		case tuned != "":
			families = cloneFamilies(tuned)
		case clonesRoot != "":
			families = cloneFamilies(clonesRoot)
		default:
			for k := range all {
				families = append(families, k)
			}
			slices.Sort(families)
		}
	}
	for _, name := range families {
		fc, ok := all[name]
		if tuned != "" {
			if fc, err = withDocCandidate(familyCandidates{}, false, name, tuned, tunedCandidates); err != nil {
				return err
			}
			ok = true
		} else if clonesRoot != "" {
			if fc, err = withClone(fc, ok, name, clonesRoot); err != nil {
				return err
			}
			ok = true
		}
		if !ok {
			return fmt.Errorf("refs session: no candidates for %q in %s", name, candPath)
		}
		ref, ok := c.Ref(fc.Reference)
		if !ok {
			return fmt.Errorf("refs session: %s: unknown reference %q", name, fc.Reference)
		}
		if prev, done := reviewedSession(records, name, fc, deep); done {
			skipped = append(skipped, fmt.Sprintf("%s (%s)", name, prev.Session))
			continue
		}
		if len(skipped) > 0 {
			fmt.Fprintf(w, "resume: %d already reviewed for the candidates on disk, skipping: %s  (RESUME=false to hear them again, FAMILIES=<name> for one)\n", len(skipped), strings.Join(skipped, ", "))
			skipped = nil
		}
		render := renderSessionClips
		if tuned != "" {
			render = tunedSessionRender(tuned, w)
		}
		k, err := runSessionFamily(w, in, p, name, fc, ref, render, outPath, planOnly, deep)
		if err != nil {
			return err
		}
		if k == verdictQuit {
			fmt.Fprintln(w, "quitting")
			return nil
		}
	}
	if len(skipped) > 0 {
		fmt.Fprintf(w, "resume: every family is already reviewed for the candidates on disk: %s  (RESUME=false to hear them again, FAMILIES=<name> for one)\n", strings.Join(skipped, ", "))
	}
	return nil
}

func cmdSession(args []string) error {
	fs := flag.NewFlagSet("session", flag.ExitOnError)
	fams := fs.String("families", "", "comma-separated family names (default: every family in the candidates file)")
	cand := fs.String("candidates", "", "candidates JSON (default: internal/audio/testdata/audition_candidates.json)")
	out := fs.String("out", "", "sessions jsonl (default: internal/audio/testdata/ear_sessions.jsonl)")
	plan := fs.Bool("plan", false, "render and print clip paths without prompting")
	cloneFlag := fs.Bool("clone", false, "add each family's WAV clone ($BEATMO_WAV_REFS/clones/<family>/) as candidate K, with a y/n/p ship prompt (`clone adopt` reads it)")
	tunedFlag := fs.Bool("tuned", false, "audition each family's essence-tuned candidate ($BEATMO_WAV_REFS/tuned/<family>/, `refs tune`) as candidate T against the reference and the current voice, with a y/n/p ship prompt (`clone adopt -kind tuned` reads it)")
	resume := fs.Bool("resume", true, "skip every family already reviewed for exactly the candidates on disk (a record in -out); false asks every family again. A family named in -families is always asked; answers are appended, never rewritten")
	deep := fs.Bool("deep", false, "the deep round: also rate each instrument's own words 1-5 (familyDeepWords, \"1 not X enough · 3 about right · 5 too X\"); the default quick round asks the word salad only")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cp := *cand
	if cp == "" {
		var err error
		if cp, err = defaultCandidatesPath(); err != nil {
			return err
		}
	}
	op := *out
	if op == "" {
		var err error
		if op, err = defaultSessionsPath(); err != nil {
			return err
		}
	}
	var families []instrument.ID
	if *fams != "" {
		for _, f := range strings.Split(*fams, ",") {
			if f = strings.TrimSpace(f); f != "" {
				families = append(families, instrument.ID(f))
			}
		}
	}
	if *cloneFlag && *tunedFlag {
		return fmt.Errorf("refs session: -clone and -tuned are separate rounds; pick one")
	}
	clones := ""
	if *cloneFlag {
		clones = filepath.Join(wavref.Root(), "clones")
	}
	tuned := ""
	if *tunedFlag {
		tuned = tunedRoot()
	}
	return runSession(os.Stdout, os.Stdin, defaultPlayer(3.0), families, cp, op, *plan, *deep, *resume, clones, tuned)
}
