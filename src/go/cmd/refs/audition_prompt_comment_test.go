//go:build !js

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// audition_prompt_comment_test.go — a verdict may carry a comment, however the
// listener naturally punctuates it.
//
// THE BUG THIS PINS
// -----------------
// The prompt advertises "free text after the choice token", but parsed it with
// strings.SplitN(line, " ", 2) -- so the token had to be followed by a BARE
// SPACE and nothing else. The most natural way to write the answer,
//
//	n, both are way too airy and breathy and soft. They need a crisp tight feel
//
// produced the token "n," which matched nothing, and the listener was told
// `invalid choice "n,"` with their whole comment discarded. Being made to
// re-type a sentence because of a comma is the kind of friction that stops
// people writing the comment at all -- and the comment is the most valuable
// part of the verdict, since it is the only field that says WHY.

func multiFamFixture() auditionFamily {
	return auditionFamily{Key: roster.Clap, Members: []instrument.ID{roster.Clap, roster.ClapTight}}
}

func singleFamFixture() auditionFamily {
	return auditionFamily{Key: roster.Oboe, Members: []instrument.ID{roster.Oboe}}
}

func TestSplitChoice_TolerantOfPunctuation(t *testing.T) {
	for _, tc := range []struct{ in, wantTok, wantNote string }{
		{"n", "n", ""},
		{"n both are airy", "n", "both are airy"},
		{"n, both are airy", "n", "both are airy"},
		{"n,both are airy", "n", "both are airy"},
		{"n: both are airy", "n", "both are airy"},
		{"n - both are airy", "n", "both are airy"},
		{"n. both are airy", "n", "both are airy"},
		{"n; both are airy", "n", "both are airy"},
		{"N,  Both Are Airy ", "n", "Both Are Airy"},
		{"1, closest but dull", "1", "closest but dull"},
		{"2 — too bright", "2", "too bright"},
		{"neither, both wrong", "neither", "both wrong"},
	} {
		tok, note := splitChoice(tc.in)
		if tok != tc.wantTok || note != tc.wantNote {
			t.Errorf("splitChoice(%q) = (%q, %q), want (%q, %q)", tc.in, tok, note, tc.wantTok, tc.wantNote)
		}
	}
}

// The note must keep its own internal punctuation -- only the SEPARATOR
// between token and note is stripped.
func TestSplitChoice_PreservesNotePunctuation(t *testing.T) {
	_, note := splitChoice("n, too airy, breathy, soft. Needs a crisp, tight feel!")
	if want := "too airy, breathy, soft. Needs a crisp, tight feel!"; note != want {
		t.Errorf("note = %q, want %q", note, want)
	}
}

func TestCanonicalChoice_AcceptsSpelledOutWords(t *testing.T) {
	for in, want := range map[string]string{
		"n": "n", "no": "n", "neither": "n",
		"y": "y", "yes": "y",
		"p": "p", "partly": "p", "partial": "p",
		"r": "r", "replay": "r",
		"s": "s", "skip": "s",
		"q": "q", "quit": "q",
		"1": "1", "2": "2",
		"maybe": "maybe", // unknown passes through unchanged, still rejected below
	} {
		if got := canonicalChoice(in); got != want {
			t.Errorf("canonicalChoice(%q) = %q, want %q", in, got, want)
		}
	}
}

// End-to-end through the real prompt: the exact line from the reported session.
func TestPromptVerdict_AcceptsCommaThenComment(t *testing.T) {
	const comment = "both are way too airy and breathy and soft. They need a crisp tight feel"
	in := strings.NewReader("n, " + comment + "\n")
	var out bytes.Buffer

	kind, verdict, chosen, note, err := promptVerdict(in, &out, multiFamFixture())
	if err != nil {
		t.Fatal(err)
	}
	if kind != verdictRecorded {
		t.Fatalf("kind = %v, want verdictRecorded", kind)
	}
	if verdict != "n" {
		t.Errorf("verdict = %q, want %q", verdict, "n")
	}
	if chosen != "" {
		t.Errorf("chosen = %q, want empty for 'neither'", chosen)
	}
	if note != comment {
		t.Errorf("note = %q, want %q", note, comment)
	}
	if strings.Contains(out.String(), "invalid") {
		t.Errorf("the prompt rejected a valid answer:\n%s", out.String())
	}
}

func TestPromptVerdict_AcceptsNumberedChoiceWithComment(t *testing.T) {
	in := strings.NewReader("2, tighter but still soft\n")
	var out bytes.Buffer
	kind, verdict, chosen, note, err := promptVerdict(in, &out, multiFamFixture())
	if err != nil {
		t.Fatal(err)
	}
	if kind != verdictRecorded || verdict != "2" || chosen != roster.ClapTight {
		t.Errorf("got kind=%v verdict=%q chosen=%q", kind, verdict, chosen)
	}
	if note != "tighter but still soft" {
		t.Errorf("note = %q", note)
	}
}

func TestPromptVerdict_SingleOptionAcceptsCommentForms(t *testing.T) {
	for _, line := range []string{"p, close but dull\n", "partly: close but dull\n", "p - close but dull\n"} {
		var out bytes.Buffer
		kind, verdict, _, note, err := promptVerdict(strings.NewReader(line), &out, singleFamFixture())
		if err != nil {
			t.Fatal(err)
		}
		if kind != verdictRecorded || verdict != "p" {
			t.Errorf("%q: got kind=%v verdict=%q", line, kind, verdict)
		}
		if note != "close but dull" {
			t.Errorf("%q: note = %q", line, note)
		}
	}
}

// A control/navigation token must NOT silently swallow a comment as if it were
// a recorded judgement -- "s, come back to this" skips, and the prose has
// nowhere to go, so the listener is told rather than left believing it was kept.
func TestPromptVerdict_WarnsWhenACommentIsDiscarded(t *testing.T) {
	var out bytes.Buffer
	kind, _, _, _, err := promptVerdict(strings.NewReader("s, come back to this one\n"), &out, multiFamFixture())
	if err != nil {
		t.Fatal(err)
	}
	if kind != verdictSkipped {
		t.Fatalf("kind = %v, want verdictSkipped", kind)
	}
	if !strings.Contains(out.String(), "come back to this one") {
		t.Errorf("a discarded comment must be echoed back so it isn't silently lost:\n%s", out.String())
	}
}

// An unparseable line must show the WHOLE line, not a mangled first token, and
// must say that a comment is allowed.
func TestPromptVerdict_InvalidMessageShowsFullLineAndMentionsComments(t *testing.T) {
	var out bytes.Buffer
	promptVerdict(strings.NewReader("dunno, hard to tell\nq\n"), &out, multiFamFixture())
	got := out.String()
	if !strings.Contains(got, "dunno, hard to tell") {
		t.Errorf("the invalid-input message must echo the full line:\n%s", got)
	}
	if !strings.Contains(strings.ToLower(got), "comment") {
		t.Errorf("the invalid-input message must mention that a comment is allowed:\n%s", got)
	}
}
