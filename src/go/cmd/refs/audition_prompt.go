//go:build !js

package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// verdictKind classifies what happened when prompting for one family's
// verdict.
type verdictKind int

const (
	// verdictRecorded means the user gave a real answer (a numbered
	// option, "n"/"y"/"p") -- the caller should persist it.
	verdictRecorded verdictKind = iota
	// verdictSkipped means "s": move on, but leave the family unanswered
	// so a future -resume run revisits it.
	verdictSkipped
	// verdictReplay means "r": play the same clips again, then prompt
	// again for the SAME family.
	verdictReplay
	// verdictQuit means "q" (or EOF on stdin, e.g. a piped-answers session
	// running out of input): stop the whole session.
	verdictQuit
)

// Multi-option: "1".."N" picks, "n" = neither is close; single-option: "y"/"1", "p", "n". r/s/q always; EOF = quit.
// Read ONE BYTE AT A TIME: ab-all runs a process per family on one piped stdin, and a bufio read-ahead would strand the
// next family's answer in a buffer that dies with this process.
func promptVerdict(in io.Reader, out io.Writer, fam auditionFamily) (kind verdictKind, verdict string, chosen instrument.ID, note string, err error) {
	for {
		fmt.Fprint(out, promptLine(fam))
		line, gotAny := readLine(in)
		if !gotAny {
			// Nothing read: quit rather than erroring out from under a piped or interrupted session.
			return verdictQuit, "", "", "", nil
		}
		line = strings.TrimSpace(line)
		if line == "" {
			fmt.Fprintln(out, "invalid: empty input, try again")
			continue
		}

		rawToken, note := splitChoice(line)
		token := canonicalChoice(rawToken)

		switch token {
		case "q":
			noteDiscarded(out, note, "quitting")
			return verdictQuit, "", "", "", nil
		case "r":
			noteDiscarded(out, note, "replaying")
			return verdictReplay, "", "", "", nil
		case "s":
			noteDiscarded(out, note, "skipping")
			return verdictSkipped, "", "", "", nil
		}

		if len(fam.Members) > 1 {
			switch token {
			case "n":
				return verdictRecorded, "n", "", note, nil
			}
			if idx, convErr := strconv.Atoi(token); convErr == nil && idx >= 1 && idx <= len(fam.Members) {
				return verdictRecorded, token, fam.Members[idx-1], note, nil
			}
			fmt.Fprintf(out, "invalid choice %q -- want 1-%d, n, r, s, or q, "+
				"optionally followed by a comment (e.g. `n, both too airy`)\n", line, len(fam.Members))
			continue
		}

		switch token {
		case "y", "1": // "1" is advertised (see promptLine) and accepted as
			// A reflex "1" on a single-option family reads as "yes, that one"; recorded as "y".
			return verdictRecorded, "y", fam.Members[0], note, nil
		case "p":
			return verdictRecorded, token, fam.Members[0], note, nil
		case "n":
			return verdictRecorded, "n", "", note, nil
		}
		fmt.Fprintf(out, "invalid choice %q -- want y/1, p, n, r, s, or q, "+
			"optionally followed by a comment (e.g. `p, close but dull`)\n", line)
	}
}

// Advertised, because a comment is the only part of a verdict that records WHY.
const commentHint = "  (add a comment after your answer, e.g. `n, both too airy and soft`)\n"

// promptLine renders the choice menu for fam. Kept separate from
// promptVerdict so the header/menu text can be reused (e.g. by tests that
// only care about the wording) without driving the read loop.
func promptLine(fam auditionFamily) string {
	var b strings.Builder
	if len(fam.Members) > 1 {
		fmt.Fprint(&b, "choose closest: ")
		for i, m := range fam.Members {
			fmt.Fprintf(&b, "%d=%s  ", i+1, m)
		}
		fmt.Fprint(&b, "n=neither  r=replay  s=skip  q=quit\n")
		fmt.Fprint(&b, commentHint)
		fmt.Fprint(&b, "> ")
		return b.String()
	}
	fmt.Fprintf(&b, "sounds like the real %s? y/1=yes  p=partly  n=no  r=replay  s=skip  q=quit\n", fam.Key)
	fmt.Fprint(&b, commentHint)
	fmt.Fprint(&b, "> ")
	return b.String()
}

// Unbuffered (see promptVerdict). gotAny=false only on a true EOF with nothing read; a last line without a newline
// still counts; a trailing '\r' is tolerated.
func readLine(r io.Reader) (line string, gotAny bool) {
	var b strings.Builder
	one := make([]byte, 1)
	for {
		n, err := r.Read(one)
		if n > 0 {
			gotAny = true
			if one[0] == '\n' {
				return strings.TrimRight(b.String(), "\r"), true
			}
			b.WriteByte(one[0])
		}
		if err != nil {
			return b.String(), gotAny
		}
	}
}

// --- Answer parsing -------------------------------------------------------

// choiceSeparators are the characters a listener naturally puts between their
// choice and their reasoning. Stripped only where they SEPARATE the two; the
// note's own punctuation is untouched.
const choiceSeparators = " \t,:;.-\u2013\u2014!)"

// The token is the leading run of letters and digits, not the text before a space: "n, both too airy" must parse, and
// the comment (the only record of WHY) keeps its case and punctuation.
func splitChoice(line string) (token, note string) {
	line = strings.TrimSpace(line)
	end := 0
	for end < len(line) {
		c := line[end]
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			end++
			continue
		}
		break
	}
	token = strings.ToLower(line[:end])
	note = strings.TrimLeft(line[end:], choiceSeparators)
	return token, strings.TrimSpace(note)
}

// A listener answering in prose writes "no", not "n"; anything unrecognised passes through to be rejected by name.
func canonicalChoice(token string) string {
	switch token {
	case "no", "neither", "none":
		return "n"
	case "yes", "yeah", "yep":
		return "y"
	case "partly", "partial", "partially", "sorta", "somewhat":
		return "p"
	case "replay", "again", "repeat":
		return "r"
	case "skip", "next":
		return "s"
	case "quit", "exit", "stop":
		return "q"
	}
	return token
}

// A navigation answer records nothing, so its comment is echoed back for the listener to re-enter.
func noteDiscarded(out io.Writer, note, action string) {
	if note == "" {
		return
	}
	fmt.Fprintf(out, "note: %s records no verdict, so this comment was NOT saved: %q\n", action, note)
}
