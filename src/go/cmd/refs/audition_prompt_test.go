//go:build !js

package main

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

func multiFam() auditionFamily {
	return auditionFamily{
		Key:     roster.Cello,
		Members: []instrument.ID{roster.Cello, roster.CelloWarm},
		Reference: wavref.Ref{
			ID:       "cello-A3",
			Measured: &wavref.Measured{SemisFromA3: 0, DurationSec: 2.34},
		},
	}
}

func singleFam() auditionFamily {
	return auditionFamily{
		Key:     roster.Bassoon,
		Members: []instrument.ID{roster.Bassoon},
		Reference: wavref.Ref{
			ID:       "bassoon-A2",
			Measured: &wavref.Measured{SemisFromA3: -7, DurationSec: 1.9},
		},
	}
}

func TestPromptVerdict_Multi_NumericChoiceWithNote(t *testing.T) {
	in := bufio.NewReader(strings.NewReader("2 attack too soft, too nasal in the mids\n"))
	var out bytes.Buffer

	kind, verdict, chosen, note, err := promptVerdict(in, &out, multiFam())
	if err != nil {
		t.Fatalf("promptVerdict: %v", err)
	}
	if kind != verdictRecorded {
		t.Fatalf("kind = %v, want verdictRecorded", kind)
	}
	if verdict != "2" {
		t.Errorf("verdict = %q, want %q", verdict, "2")
	}
	if chosen != roster.CelloWarm {
		t.Errorf("chosen = %q, want %q (index 2 -> Members[1])", chosen, "cello-warm")
	}
	if note != "attack too soft, too nasal in the mids" {
		t.Errorf("note = %q", note)
	}
}

func TestPromptVerdict_Multi_NeitherHasNoChosen(t *testing.T) {
	in := bufio.NewReader(strings.NewReader("n\n"))
	var out bytes.Buffer

	kind, verdict, chosen, _, err := promptVerdict(in, &out, multiFam())
	if err != nil {
		t.Fatalf("promptVerdict: %v", err)
	}
	if kind != verdictRecorded {
		t.Fatalf("kind = %v, want verdictRecorded", kind)
	}
	if verdict != "n" || chosen != "" {
		t.Errorf("verdict=%q chosen=%q, want n/\"\"", verdict, chosen)
	}
}

func TestPromptVerdict_Multi_OutOfRangeDigitIsInvalidAndReprompts(t *testing.T) {
	// cello has only 2 options (1, 2); "9" must be rejected and re-prompted,
	// with "1" then accepted.
	in := bufio.NewReader(strings.NewReader("9\n1\n"))
	var out bytes.Buffer

	kind, verdict, chosen, _, err := promptVerdict(in, &out, multiFam())
	if err != nil {
		t.Fatalf("promptVerdict: %v", err)
	}
	if kind != verdictRecorded || verdict != "1" || chosen != roster.Cello {
		t.Errorf("got kind=%v verdict=%q chosen=%q, want recorded/1/cello after reprompt", kind, verdict, chosen)
	}
	if !strings.Contains(out.String(), "invalid") {
		t.Errorf("expected an invalid-choice message in output, got:\n%s", out.String())
	}
}

func TestPromptVerdict_Multi_GarbageInputReprompts(t *testing.T) {
	in := bufio.NewReader(strings.NewReader("banana\n2\n"))
	var out bytes.Buffer

	kind, verdict, _, _, err := promptVerdict(in, &out, multiFam())
	if err != nil {
		t.Fatalf("promptVerdict: %v", err)
	}
	if kind != verdictRecorded || verdict != "2" {
		t.Errorf("got kind=%v verdict=%q, want recorded/2 after reprompt", kind, verdict)
	}
}

func TestPromptVerdict_Multi_Replay(t *testing.T) {
	in := bufio.NewReader(strings.NewReader("r\n"))
	var out bytes.Buffer
	kind, _, _, _, err := promptVerdict(in, &out, multiFam())
	if err != nil {
		t.Fatalf("promptVerdict: %v", err)
	}
	if kind != verdictReplay {
		t.Errorf("kind = %v, want verdictReplay", kind)
	}
}

func TestPromptVerdict_Multi_Skip(t *testing.T) {
	in := bufio.NewReader(strings.NewReader("s\n"))
	var out bytes.Buffer
	kind, _, _, _, err := promptVerdict(in, &out, multiFam())
	if err != nil {
		t.Fatalf("promptVerdict: %v", err)
	}
	if kind != verdictSkipped {
		t.Errorf("kind = %v, want verdictSkipped", kind)
	}
}

func TestPromptVerdict_Multi_Quit(t *testing.T) {
	in := bufio.NewReader(strings.NewReader("q\n"))
	var out bytes.Buffer
	kind, _, _, _, err := promptVerdict(in, &out, multiFam())
	if err != nil {
		t.Fatalf("promptVerdict: %v", err)
	}
	if kind != verdictQuit {
		t.Errorf("kind = %v, want verdictQuit", kind)
	}
}

func TestPromptVerdict_Single_OneIsAcceptedAsSynonymForYes(t *testing.T) {
	// A reflex "1" on a single-option family is canonicalised to "y", so a piped "1 <note>\nq\n" works whichever family
	// the session lands on first.
	in := bufio.NewReader(strings.NewReader("1 spot on\n"))
	var out bytes.Buffer
	kind, verdict, chosen, note, err := promptVerdict(in, &out, singleFam())
	if err != nil {
		t.Fatalf("promptVerdict: %v", err)
	}
	if kind != verdictRecorded || verdict != "y" || chosen != roster.Bassoon || note != "spot on" {
		t.Errorf("got kind=%v verdict=%q chosen=%q note=%q, want recorded/y/bassoon/\"spot on\"", kind, verdict, chosen, note)
	}
}

func TestPromptVerdict_Multi_OutOfRangeStillRejectedForSingleOptionPrompts(t *testing.T) {
	// Only "1" is special-cased for a single-option family; any other
	// digit remains invalid and reprompts.
	in := bufio.NewReader(strings.NewReader("2\ny\n"))
	var out bytes.Buffer
	kind, verdict, chosen, _, err := promptVerdict(in, &out, singleFam())
	if err != nil {
		t.Fatalf("promptVerdict: %v", err)
	}
	if kind != verdictRecorded || verdict != "y" || chosen != roster.Bassoon {
		t.Errorf("got kind=%v verdict=%q chosen=%q, want recorded/y/bassoon after reprompt", kind, verdict, chosen)
	}
}

func TestPromptVerdict_Single_YesRecordsChosen(t *testing.T) {
	in := bufio.NewReader(strings.NewReader("y sounds spot on\n"))
	var out bytes.Buffer
	kind, verdict, chosen, note, err := promptVerdict(in, &out, singleFam())
	if err != nil {
		t.Fatalf("promptVerdict: %v", err)
	}
	if kind != verdictRecorded || verdict != "y" || chosen != roster.Bassoon || note != "sounds spot on" {
		t.Errorf("got kind=%v verdict=%q chosen=%q note=%q", kind, verdict, chosen, note)
	}
}

func TestPromptVerdict_Single_PartlyRecordsChosen(t *testing.T) {
	in := bufio.NewReader(strings.NewReader("p\n"))
	var out bytes.Buffer
	kind, verdict, chosen, _, err := promptVerdict(in, &out, singleFam())
	if err != nil {
		t.Fatalf("promptVerdict: %v", err)
	}
	if kind != verdictRecorded || verdict != "p" || chosen != roster.Bassoon {
		t.Errorf("got kind=%v verdict=%q chosen=%q", kind, verdict, chosen)
	}
}

func TestPromptVerdict_Single_NoHasNoChosen(t *testing.T) {
	in := bufio.NewReader(strings.NewReader("n\n"))
	var out bytes.Buffer
	kind, verdict, chosen, _, err := promptVerdict(in, &out, singleFam())
	if err != nil {
		t.Fatalf("promptVerdict: %v", err)
	}
	if kind != verdictRecorded || verdict != "n" || chosen != "" {
		t.Errorf("got kind=%v verdict=%q chosen=%q", kind, verdict, chosen)
	}
}

func TestPromptVerdict_EmptyLineIsInvalidAndReprompts(t *testing.T) {
	in := bufio.NewReader(strings.NewReader("\nn\n"))
	var out bytes.Buffer
	kind, verdict, _, _, err := promptVerdict(in, &out, singleFam())
	if err != nil {
		t.Fatalf("promptVerdict: %v", err)
	}
	if kind != verdictRecorded || verdict != "n" {
		t.Errorf("got kind=%v verdict=%q, want recorded/n after reprompt", kind, verdict)
	}
}

func TestPromptVerdict_EOFDuringPromptSignalsQuit(t *testing.T) {
	in := bufio.NewReader(strings.NewReader(""))
	var out bytes.Buffer
	kind, _, _, _, err := promptVerdict(in, &out, singleFam())
	if err != nil {
		t.Fatalf("promptVerdict on EOF: %v", err)
	}
	if kind != verdictQuit {
		t.Errorf("kind = %v, want verdictQuit on EOF (e.g. piped stdin ran out)", kind)
	}
}

// ab-all answers each family in a SEPARATE process on one piped stdin: a read-ahead in the first would strand the next
// family's answer and starve its prompt. A shared io.Reader across two calls reproduces that without a subprocess.
func TestPromptVerdict_DoesNotOverreadPastCurrentLine(t *testing.T) {
	shared := strings.NewReader("1 first note\nq\n")
	var out bytes.Buffer

	kind1, verdict1, chosen1, note1, err := promptVerdict(shared, &out, multiFam())
	if err != nil {
		t.Fatalf("promptVerdict (first): %v", err)
	}
	if kind1 != verdictRecorded || verdict1 != "1" || chosen1 != roster.Cello || note1 != "first note" {
		t.Fatalf("first read = kind=%v verdict=%q chosen=%q note=%q, want recorded/1/cello/\"first note\"", kind1, verdict1, chosen1, note1)
	}

	kind2, _, _, _, err := promptVerdict(shared, &out, multiFam())
	if err != nil {
		t.Fatalf("promptVerdict (second): %v", err)
	}
	if kind2 != verdictQuit {
		t.Errorf("second read from the shared stream should see the literal \"q\" still sitting in it, got kind=%v (likely starved by over-read)", kind2)
	}
}
