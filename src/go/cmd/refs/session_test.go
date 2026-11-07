//go:build !js

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// testReplayer returns a replayer over fixed clip paths plus the fake player
// it plays through, so prompt tests can assert exactly which clips a replay
// token produced.
func testReplayer() (*replayer, *fakePlayer) {
	p := &fakePlayer{}
	rp := &replayer{p: p, out: &bytes.Buffer{}, clips: sessionClips{
		refPath:   "/clips/R.wav",
		curPath:   "/clips/C.wav",
		candPaths: map[string]string{"A": "/clips/A.wav", "B": "/clips/B.wav"},
		order:     []string{"R", "C", "A", "B"},
	}}
	return rp, p
}

func TestPromptScale_ParsesAndCentres(t *testing.T) {
	q := sessionQuestion{Key: "warm", Text: "warm", Axis: "timbre.brightness_ratio", Falls: true}
	rp, _ := testReplayer()
	ans, kind := promptScale(strings.NewReader("1\n5\n"), &bytes.Buffer{}, rp, q, []string{"A", "B"})
	if kind != verdictRecorded {
		t.Fatalf("kind %v", kind)
	}
	// Warm falls on brightness: "not warm enough" (1) is a synth that reads too bright, +2.
	if ans["A"] != 2 || ans["B"] != -2 {
		t.Fatalf("got %+v", ans)
	}
	ans, kind = promptScale(strings.NewReader("-\n4\n"), &bytes.Buffer{}, rp, q, []string{"A", "B"})
	if kind != verdictRecorded || len(ans) != 1 || ans["B"] != -1 {
		t.Fatalf("dash must skip only that clip: %+v %v", ans, kind)
	}
	_, kind = promptScale(strings.NewReader("q\n"), &bytes.Buffer{}, rp, q, []string{"A"})
	if kind != verdictQuit {
		t.Fatal("q must quit")
	}
}

// A replay token at a scale question plays the clips and re-asks the SAME
// question; "rel" for a per-clip question is the reference plus the clips
// being rated.
func TestPromptScale_ReplayReasksSameQuestion(t *testing.T) {
	q := sessionQuestion{Key: "warm", Text: "warm", Axis: "timbre.brightness_ratio", Falls: true}
	rp, p := testReplayer()
	var out bytes.Buffer
	// A: rel (R+A), then "a", then answer 4; B: all, then answer 2.
	ans, kind := promptScale(strings.NewReader("rel\na\n4\nall\n2\n"), &out, rp, q, []string{"A", "B"})
	if kind != verdictRecorded || ans["A"] != -1 || ans["B"] != 1 {
		t.Fatalf("got %+v %v", ans, kind)
	}
	want := []string{"/clips/R.wav", "/clips/A.wav", "/clips/A.wav",
		"/clips/R.wav", "/clips/C.wav", "/clips/A.wav", "/clips/B.wav"}
	if strings.Join(p.played, ",") != strings.Join(want, ",") {
		t.Fatalf("played %v, want %v", p.played, want)
	}
	if strings.Count(out.String(), "warm — clip A") != 3 || strings.Count(out.String(), "warm — clip B") != 2 {
		t.Fatalf("the same clip's question must be re-asked after each replay:\n%s", out.String())
	}
}

func TestPromptRatingAndVs(t *testing.T) {
	rp, p := testReplayer()
	r, kind := promptRating(strings.NewReader("7\nrel\n4\n"), &bytes.Buffer{}, rp, "A")
	if kind != verdictRecorded || r != 4 {
		t.Fatalf("rating: %d %v (7 must be rejected, rel replays, then 4 accepted)", r, kind)
	}
	if want := "/clips/R.wav,/clips/A.wav"; strings.Join(p.played, ",") != want {
		t.Fatalf("rel at a rating prompt plays the reference and that candidate, got %v", p.played)
	}
	p.played = nil
	vs, kind := promptVsCurrent(strings.NewReader("rel\n+\n"), &bytes.Buffer{}, rp, "A")
	if kind != verdictRecorded || vs != "better" {
		t.Fatalf("vs: %q %v", vs, kind)
	}
	if want := "/clips/R.wav,/clips/C.wav,/clips/A.wav"; strings.Join(p.played, ",") != want {
		t.Fatalf("rel at a vs prompt plays reference, current and that candidate, got %v", p.played)
	}
}

func TestPromptAdjectives(t *testing.T) {
	rp, _ := testReplayer()
	got, kind := promptAdjectives(strings.NewReader("3 7\n-\n"), &bytes.Buffer{}, rp, saladWords(), []string{"A", "B"})
	if kind != verdictRecorded || len(got["A"]) != 2 || got["A"][0] != saladWords()[2] || len(got["B"]) != 0 {
		t.Fatalf("got %+v", got)
	}
}

// Controls only as a BARE single token: a note opening with one of those letters is the only record of WHY.
func TestPromptNote_HonoursControls(t *testing.T) {
	for _, tc := range []struct {
		in     string
		want   verdictKind
		note   string
		played int
	}{
		{"r\nfine\n", verdictRecorded, "fine", 1},
		{"all\nfine\n", verdictRecorded, "fine", 4},
		{"s\n", verdictSkipped, "", 0},
		{"q\n", verdictQuit, "", 0},
		{"really close now\n", verdictRecorded, "really close now", 0},
		{"all good\n", verdictRecorded, "all good", 0},
		{"still too dull\n", verdictRecorded, "still too dull", 0},
		{"\n", verdictRecorded, "", 0},
	} {
		rp, p := testReplayer()
		note, kind := promptNote(strings.NewReader(tc.in), &bytes.Buffer{}, rp)
		if kind != tc.want || note != tc.note || len(p.played) != tc.played {
			t.Errorf("%q: got (%q, %v, %d plays), want (%q, %v, %d plays)", tc.in, note, kind, len(p.played), tc.note, tc.want, tc.played)
		}
	}
}

// ---- end-to-end session flow (no rendering) --------------------------------

// fakePlayer records what it was asked to play, in order, so a test can assert
// the clips a listener would actually have heard.
type fakePlayer struct{ played []string }

func (f *fakePlayer) Play(path string) error {
	f.played = append(f.played, path)
	return nil
}

// sessionFixture returns a two-candidate family plus a stub render that hands
// back fixed paths, so the whole prompt/record flow runs without synthesising
// or decoding a single sample.
func sessionFixture(t *testing.T) (familyCandidates, wavref.Ref, sessionRenderFn) {
	t.Helper()
	fc := familyCandidates{
		Reference: "fake-ref",
		Base:      roster.Oboe,
		Candidates: []candidateSpec{
			{ID: "a", Label: "brighter", Hypothesis: "cutoff too low", Overrides: map[string]float64{"filter_cutoff": 6900}},
			{ID: "b", Label: "darker", Hypothesis: "cutoff too high", Overrides: map[string]float64{"filter_cutoff": 7100}},
		},
	}
	ref := wavref.Ref{ID: "fake-ref", Measured: &wavref.Measured{SemisFromA3: 12, DurationSec: 1.5}}
	render := func(familyCandidates, wavref.Ref) (sessionClips, sessionRenderMeta, error) {
		return sessionClips{
			refPath:   "/clips/R.wav",
			curPath:   "/clips/C.wav",
			candPaths: map[string]string{"A": "/clips/A.wav", "B": "/clips/B.wav"},
			order:     []string{"R", "C", "A", "B"},
		}, sessionRenderMeta{Pitch: 12, Dur: 1.5}, nil
	}
	// Two deep words of our own so the flow is pinned to this test rather
	// than to whatever the shipped table happens to ask this round.
	familyDeepWords["test-fam"] = []string{"bright", "breathy"}
	t.Cleanup(func() { delete(familyDeepWords, "test-fam") })
	return fc, ref, render
}

func readOneSession(t *testing.T, path string) earSession {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 1 {
		t.Fatalf("want exactly one record, got %d:\n%s", len(lines), b)
	}
	var s earSession
	if err := json.Unmarshal([]byte(lines[0]), &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return s
}

func TestRunSessionFamily_HappyPathWritesOneRecord(t *testing.T) {
	fc, ref, render := sessionFixture(t)
	out := filepath.Join(t.TempDir(), "ear_sessions.jsonl")
	in := strings.Join([]string{
		"",       // Enter to rate
		"4", "+", // A: rating 4, better than current
		"2", "-", // B: rating 2, worse than current
		"5", "1", // bright: A +2, B -2
		"3", "-", // breathy: A 0, B unanswered
		"1 3", "13", // adjectives for A, then B
		"1", // ship A
		"close on A",
	}, "\n") + "\n"
	p := &fakePlayer{}
	k, err := runSessionFamily(&bytes.Buffer{}, strings.NewReader(in), p, "test-fam", fc, ref, render, out, false, true)
	if err != nil || k != verdictRecorded {
		t.Fatalf("runSessionFamily: %v %v", k, err)
	}
	if want := []string{"/clips/R.wav", "/clips/C.wav", "/clips/A.wav", "/clips/B.wav"}; strings.Join(p.played, ",") != strings.Join(want, ",") {
		t.Fatalf("played %v, want %v", p.played, want)
	}

	s := readOneSession(t, out)
	if s.Family != "test-fam" || s.Reference != "fake-ref" || s.SemisFromA3 != 12 || s.DurSec != 1.5 {
		t.Fatalf("header: %+v", s)
	}
	if s.Current.ID != "oboe" || s.Current.RecipeSHA == "" {
		t.Fatalf("current: %+v", s.Current)
	}
	if s.Note != "close on A" {
		t.Fatalf("note %q", s.Note)
	}
	if s.Ship != "A" {
		t.Fatalf("ship %q, want A", s.Ship)
	}
	if len(s.Candidates) != 2 {
		t.Fatalf("want 2 candidates, got %d", len(s.Candidates))
	}
	a, b := s.Candidates[0], s.Candidates[1]
	if a.ID != "A" || a.Base != roster.Oboe || a.Label != "brighter" || a.Rating != 4 || a.VsCurrent != "better" {
		t.Errorf("A: %+v", a)
	}
	if b.ID != "B" || b.Base != roster.Oboe || b.Label != "darker" || b.Rating != 2 || b.VsCurrent != "worse" {
		t.Errorf("B: %+v", b)
	}
	if a.RecipeSHA == "" || a.RecipeSHA == b.RecipeSHA || a.RecipeSHA == s.Current.RecipeSHA {
		t.Errorf("recipe hashes must differ per override set: %q %q %q", s.Current.RecipeSHA, a.RecipeSHA, b.RecipeSHA)
	}
	if a.Overrides["filter_cutoff"] != 6900 || b.Overrides["filter_cutoff"] != 7100 {
		t.Errorf("overrides: %+v %+v", a.Overrides, b.Overrides)
	}
	// Scales are centred (n-3) and carry the listener-facing question text.
	wantA := []sessionScale{
		{Q: "bright", Axis: "timbre.brightness_ratio", Answer: 2},
		{Q: "breathy", Axis: "noise.persistence", Answer: 0},
	}
	if len(a.Scales) != 2 || a.Scales[0] != wantA[0] || a.Scales[1] != wantA[1] {
		t.Errorf("A scales: %+v, want %+v", a.Scales, wantA)
	}
	if len(b.Scales) != 1 || b.Scales[0] != (sessionScale{Q: "bright", Axis: "timbre.brightness_ratio", Answer: -2}) {
		t.Errorf("B scales: %+v (an unanswered clip records nothing for that question)", b.Scales)
	}
	if len(a.Adjectives) != 2 || a.Adjectives[0] != saladWords()[0] || a.Adjectives[1] != saladWords()[2] {
		t.Errorf("A adjectives: %+v", a.Adjectives)
	}
	if len(b.Adjectives) != 1 || b.Adjectives[0] != saladWords()[12] {
		t.Errorf("B adjectives: %+v", b.Adjectives)
	}
}

// A replay at ANY prompt returns to the same unanswered question with every answer kept, and one record at the end.
func TestRunSessionFamily_ReplayResumesAtTheSameQuestion(t *testing.T) {
	fc, ref, render := sessionFixture(t)
	out := filepath.Join(t.TempDir(), "ear_sessions.jsonl")
	in := strings.Join([]string{
		"c",        // opening prompt: hear the current version again
		"",         // Enter to rate
		"rel", "4", // A rating: hear R and A, then answer
		"+",
		"2", "b", "-", // B rating 2; at vs-current, "b" replays candidate B (not "better"), then worse
		"all", "5", "1", // brightness: replay everything, then answer A, then B
		"3", "-", // breathy: A 0, B unanswered
		"r", "5", "", // adjectives: hear the reference, A gets 5, B none
		"c", "0", // ship: hear the current version again, then keep it
		"a", "second pass", // note: hear A, then the note
	}, "\n") + "\n"
	p := &fakePlayer{}
	k, err := runSessionFamily(&bytes.Buffer{}, strings.NewReader(in), p, "test-fam", fc, ref, render, out, false, true)
	if err != nil || k != verdictRecorded {
		t.Fatalf("runSessionFamily: %v %v", k, err)
	}
	want := []string{
		"/clips/R.wav", "/clips/C.wav", "/clips/A.wav", "/clips/B.wav", // opening playback
		"/clips/C.wav",                 // c
		"/clips/R.wav", "/clips/A.wav", // rel at A's rating
		"/clips/B.wav",                                                 // b at B's vs-current
		"/clips/R.wav", "/clips/C.wav", "/clips/A.wav", "/clips/B.wav", // all
		"/clips/R.wav", // r at adjectives
		"/clips/C.wav", // c at ship
		"/clips/A.wav", // a at the note
	}
	if strings.Join(p.played, ",") != strings.Join(want, ",") {
		t.Fatalf("played %v\nwant   %v", p.played, want)
	}
	s := readOneSession(t, out)
	if s.Note != "second pass" {
		t.Fatalf("note %q", s.Note)
	}
	if s.Ship != "C" {
		t.Fatalf("ship %q, want C (keep current)", s.Ship)
	}
	if s.Candidates[0].Rating != 4 || s.Candidates[0].VsCurrent != "better" || s.Candidates[1].Rating != 2 || s.Candidates[1].VsCurrent != "worse" {
		t.Fatalf("answers given before and after replays must all be kept: %+v", s.Candidates)
	}
	if s.Candidates[0].Scales[0].Answer != 2 || s.Candidates[1].Scales[0].Answer != -2 || s.Candidates[0].Scales[1].Answer != 0 {
		t.Fatalf("scales: %+v", s.Candidates)
	}
	if len(s.Candidates[0].Adjectives) != 1 || s.Candidates[0].Adjectives[0] != saladWords()[4] {
		t.Fatalf("adjectives: %+v", s.Candidates[0].Adjectives)
	}
}

func TestRunSessionFamily_QuitMidQuestionsWritesNothing(t *testing.T) {
	fc, ref, render := sessionFixture(t)
	out := filepath.Join(t.TempDir(), "ear_sessions.jsonl")
	in := "\n4\n+\n2\n-\nq\n" // q at the first scale question
	p := &fakePlayer{}
	k, err := runSessionFamily(&bytes.Buffer{}, strings.NewReader(in), p, "test-fam", fc, ref, render, out, false, true)
	if err != nil {
		t.Fatalf("err %v", err)
	}
	if k != verdictQuit {
		t.Fatalf("kind %v, want quit", k)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		b, _ := os.ReadFile(out)
		t.Fatalf("quitting mid-session must write nothing, found: %s", b)
	}
}

// TestPromptVsCurrent_Keys pins the comparison vocabulary: `+`/`=`/`-` (or
// the words), `s` skips, `q` quits, and a candidate's letter REPLAYS that
// candidate instead of being mistaken for "better".
func TestPromptVsCurrent_Keys(t *testing.T) {
	for _, c := range []struct {
		in       string
		wantVs   string
		wantKind verdictKind
		played   int
	}{
		{"+\n", "better", verdictRecorded, 0},
		{"better\n", "better", verdictRecorded, 0},
		{"=\n", "same", verdictRecorded, 0},
		{"-\n", "worse", verdictRecorded, 0},
		{"s\n", "", verdictSkipped, 0},
		{"q\n", "", verdictQuit, 0},
		{"b\n=\n", "same", verdictRecorded, 1},
		{"r\nc\na\n+\n", "better", verdictRecorded, 3},
	} {
		rp, p := testReplayer()
		vs, kind := promptVsCurrent(strings.NewReader(c.in), &bytes.Buffer{}, rp, "A")
		if vs != c.wantVs || kind != c.wantKind || len(p.played) != c.played {
			t.Errorf("input %q -> (%q, %v, %d plays), want (%q, %v, %d plays)", strings.TrimSpace(c.in), vs, kind, len(p.played), c.wantVs, c.wantKind, c.played)
		}
	}
}

// The prompt text has to advertise the tokens it actually accepts, or the
// listener learns the wrong ones.
func TestPromptVsCurrent_PromptTextNamesTheRealTokens(t *testing.T) {
	rp, _ := testReplayer()
	var buf bytes.Buffer
	promptVsCurrent(strings.NewReader("=\n"), &buf, rp, "A")
	out := buf.String()
	for _, want := range []string{"+ better", "= same", "- worse", "s skip", "replay: r c a b all rel"} {
		if !strings.Contains(out, want) {
			t.Errorf("prompt does not advertise %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "b better") || strings.Contains(out, "s same") {
		t.Errorf("prompt still advertises a letter that now means something else:\n%s", out)
	}
}

// An invalid token must be re-prompted, and the error message must name the
// real vocabulary.
func TestPromptVsCurrent_InvalidTokenReprompts(t *testing.T) {
	rp, _ := testReplayer()
	var buf bytes.Buffer
	vs, kind := promptVsCurrent(strings.NewReader("x\n=\n"), &buf, rp, "A")
	if vs != "same" || kind != verdictRecorded {
		t.Fatalf("got (%q, %v)", vs, kind)
	}
	if !strings.Contains(buf.String(), "want +, = or -") {
		t.Errorf("error message does not name the accepted tokens:\n%s", buf.String())
	}
}

// Several replay keys on one line play in order; a line mixing a replay key
// with anything else is not a replay line.
func TestReplayer_MultiTokenLine(t *testing.T) {
	rp, p := testReplayer()
	if !rp.replay("r a", []string{"R", "A"}) {
		t.Fatal("\"r a\" must be a replay line")
	}
	if want := "/clips/R.wav,/clips/A.wav"; strings.Join(p.played, ",") != want {
		t.Fatalf("played %v", p.played)
	}
	p.played = nil
	if rp.replay("r 4", nil) {
		t.Fatal("a line with a non-replay token must not replay")
	}
	if len(p.played) != 0 {
		t.Fatalf("nothing may play for a rejected line, got %v", p.played)
	}
}

// Quick is the same short steps for every instrument; the mode is recorded, so a missing scale reads "not asked", never
// "about right".
func TestRunSessionFamily_QuickRoundAsksNoWordRatings(t *testing.T) {
	fc, ref, render := sessionFixture(t)
	out := filepath.Join(t.TempDir(), "ear_sessions.jsonl")
	in := strings.Join([]string{
		"",       // Enter to rate
		"4", "+", // A
		"2", "-", // B
		"1 3", "", // words for A, none for B
		"1", // ship A
		"quick note",
	}, "\n") + "\n"
	var w bytes.Buffer
	k, err := runSessionFamily(&w, strings.NewReader(in), &fakePlayer{}, "test-fam", fc, ref, render, out, false, false)
	if err != nil || k != verdictRecorded {
		t.Fatalf("runSessionFamily: %v %v\n%s", k, err, w.String())
	}
	if strings.Contains(w.String(), "about right") {
		t.Fatalf("the quick round asked a word rating:\n%s", w.String())
	}
	s := readOneSession(t, out)
	if s.Mode != sessionModeQuick || s.Note != "quick note" || s.Ship != "A" {
		t.Fatalf("record: mode %q note %q ship %q", s.Mode, s.Note, s.Ship)
	}
	a := s.Candidates[0]
	if len(a.Scales) != 0 || len(a.Adjectives) != 2 || a.Adjectives[0] != saladWords()[0] || a.Adjectives[1] != saladWords()[2] {
		t.Fatalf("A: scales %+v words %+v", a.Scales, a.Adjectives)
	}
	// The word menu is the shared salad, one group per line, whatever the
	// instrument: every word is offered, under its group.
	for _, wd := range wordSalad {
		if !strings.Contains(w.String(), " "+wd.Word) {
			t.Errorf("the menu does not offer %q", wd.Word)
		}
	}
	if !strings.Contains(w.String(), "\n  "+wordSalad[0].Group+":") {
		t.Errorf("the menu must print each group on its own line:\n%s", w.String())
	}

	out2 := filepath.Join(t.TempDir(), "ear_sessions.jsonl")
	deep := strings.Join([]string{"", "4", "+", "2", "-", "5", "1", "3", "-", "", "", "1", "deep note"}, "\n") + "\n"
	if k, err := runSessionFamily(&bytes.Buffer{}, strings.NewReader(deep), &fakePlayer{}, "test-fam", fc, ref, render, out2, false, true); err != nil || k != verdictRecorded {
		t.Fatalf("deep: %v %v", k, err)
	}
	if s := readOneSession(t, out2); s.Mode != sessionModeDeep || len(s.Candidates[0].Scales) != 2 {
		t.Fatalf("deep record: mode %q scales %+v", s.Mode, s.Candidates[0].Scales)
	}
}

// A quit keeps a draft; the rerun resumes at the first unanswered question, writes one record, removes the draft.
func TestRunSessionFamily_QuitThenResumeKeepsAnswers(t *testing.T) {
	fc, ref, render := sessionFixture(t)
	out := filepath.Join(t.TempDir(), "ear_sessions.jsonl")
	// First run: rate both, answer brightness for A, then quit at brightness B.
	in1 := strings.Join([]string{"", "4", "+", "2", "-", "5", "q"}, "\n") + "\n"
	p := &fakePlayer{}
	k, err := runSessionFamily(&bytes.Buffer{}, strings.NewReader(in1), p, "test-fam", fc, ref, render, out, false, true)
	if err != nil || k != verdictQuit {
		t.Fatalf("first run: %v %v", k, err)
	}
	if _, err := os.Stat(draftPath(out, "test-fam")); err != nil {
		t.Fatalf("a quit must leave a draft: %v", err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("a quit must not write a record")
	}
	// Second run: only the unanswered prompts are asked: brightness B,
	// breathiness A and B, adjectives A and B, note.
	var w bytes.Buffer
	in2 := strings.Join([]string{"", "1", "3", "-", "6", "", "2", "resumed fine"}, "\n") + "\n"
	k, err = runSessionFamily(&w, strings.NewReader(in2), p, "test-fam", fc, ref, render, out, false, true)
	if err != nil || k != verdictRecorded {
		t.Fatalf("second run: %v %v\n%s", k, err, w.String())
	}
	if !strings.Contains(w.String(), "resuming: 5 answers kept") {
		t.Fatalf("resume banner missing:\n%s", w.String())
	}
	if strings.Contains(w.String(), "How close is A") || strings.Contains(w.String(), "bright — clip A") {
		t.Fatalf("answered prompts must not be asked again:\n%s", w.String())
	}
	s := readOneSession(t, out)
	a, b := s.Candidates[0], s.Candidates[1]
	if a.Rating != 4 || a.VsCurrent != "better" || b.Rating != 2 || b.VsCurrent != "worse" {
		t.Fatalf("first-run answers lost: %+v", s.Candidates)
	}
	if len(a.Scales) != 2 || a.Scales[0].Answer != 2 || a.Scales[1].Answer != 0 || len(b.Scales) != 1 || b.Scales[0].Answer != -2 {
		t.Fatalf("scales: A %+v B %+v", a.Scales, b.Scales)
	}
	if len(a.Adjectives) != 1 || a.Adjectives[0] != saladWords()[5] || len(b.Adjectives) != 0 {
		t.Fatalf("adjectives: %+v %+v", a.Adjectives, b.Adjectives)
	}
	if s.Note != "resumed fine" {
		t.Fatalf("note %q", s.Note)
	}
	if s.Ship != "B" {
		t.Fatalf("ship %q, want B", s.Ship)
	}
	if _, err := os.Stat(draftPath(out, "test-fam")); !os.IsNotExist(err) {
		t.Fatal("the draft must be removed once the record is written")
	}
}

// Skipping a family discards its draft: the listener asked to drop it.
func TestRunSessionFamily_SkipDiscardsDraft(t *testing.T) {
	fc, ref, render := sessionFixture(t)
	out := filepath.Join(t.TempDir(), "ear_sessions.jsonl")
	in := "\n4\n+\ns\n"
	k, err := runSessionFamily(&bytes.Buffer{}, strings.NewReader(in), &fakePlayer{}, "test-fam", fc, ref, render, out, false, true)
	if err != nil || k != verdictSkipped {
		t.Fatalf("%v %v", k, err)
	}
	if _, err := os.Stat(draftPath(out, "test-fam")); !os.IsNotExist(err) {
		t.Fatal("skip must remove the draft")
	}
}

// promptShip: digits pick the version to ship, independent of closeness;
// the candidate letters keep meaning replay here.
func TestPromptShip(t *testing.T) {
	for _, c := range []struct {
		in     string
		want   string
		kind   verdictKind
		played int
	}{
		{"0\n", "C", verdictRecorded, 0},
		{"1\n", "A", verdictRecorded, 0},
		{"2\n", "B", verdictRecorded, 0},
		{"a\n2\n", "B", verdictRecorded, 1},
		{"3\n0\n", "C", verdictRecorded, 0},
		{"s\n", "", verdictSkipped, 0},
		{"q\n", "", verdictQuit, 0},
	} {
		rp, p := testReplayer()
		got, kind := promptShip(strings.NewReader(c.in), &bytes.Buffer{}, rp, []string{"A", "B"})
		if got != c.want || kind != c.kind || len(p.played) != c.played {
			t.Errorf("%q: got (%q, %v, %d plays), want (%q, %v, %d)", strings.TrimSpace(c.in), got, kind, len(p.played), c.want, c.kind, c.played)
		}
	}
}

// Another round's draft (other candidates) is never resumed, overwritten or deleted, whatever this round answers.
func TestADraftOfOtherCandidatesIsNeverResumedOrLost(t *testing.T) {
	fc, ref, render := sessionFixture(t)
	out := filepath.Join(t.TempDir(), "ear_sessions.jsonl")
	stale := sessionDraft{
		Session: earSession{Session: "2026-09-06T19:22:14Z", Family: "test-fam", Reference: "fake-ref",
			Candidates: []sessionCandidate{{sessionClip: sessionClip{ID: "A", Base: roster.Oboe, RecipeSHA: "another-round"}, Rating: 1, VsCurrent: "better"}}},
		Done: map[string]bool{"rating:A": true, "vs:A": true},
	}
	if err := saveDraft(draftPath(out, "test-fam"), stale); err != nil {
		t.Fatal(err)
	}
	staleBytes, _ := os.ReadFile(draftPath(out, "test-fam"))
	unchanged := func(when string) {
		t.Helper()
		if b, err := os.ReadFile(draftPath(out, "test-fam")); err != nil || string(b) != string(staleBytes) {
			t.Fatalf("%s: the other round's draft was changed or removed (%v)", when, err)
		}
	}
	run := func(in string) (verdictKind, string) {
		var w bytes.Buffer
		k, err := runSessionFamily(&w, strings.NewReader(in), &fakePlayer{}, "test-fam", fc, ref, render, out, false, false)
		if err != nil {
			t.Fatal(err)
		}
		return k, w.String()
	}
	if k, log := run("\ns\n"); k != verdictSkipped || strings.Contains(log, "resuming") {
		t.Fatalf("skip: %v\n%s", k, log)
	}
	unchanged("a skip")
	// Quit after one answer: this round's draft is kept beside the old one.
	if k, _ := run("\n4\nq\n"); k != verdictQuit {
		t.Fatalf("quit: %v", k)
	}
	unchanged("a quit")
	if k, log := run("\n+\n2\n-\n\n\n1\nnote\n"); k != verdictRecorded || !strings.Contains(log, "resuming: 1 answers kept") {
		t.Fatalf("this round must resume ITS OWN draft: %v\n%s", k, log)
	}
	unchanged("a recorded answer")
	if s := readOneSession(t, out); s.Candidates[0].Rating != 4 || s.Candidates[0].ID != "A" || s.Candidates[0].RecipeSHA == "another-round" {
		t.Fatalf("record: %+v", s.Candidates[0])
	}
	if m, _ := filepath.Glob(out + ".test-fam.*.draft.json"); len(m) != 0 {
		t.Fatalf("this round's draft must be removed once recorded: %v", m)
	}
}
