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

// promptVerdict writes the prompt for fam to out, reads exactly one valid
// answer from in (re-prompting on blank/invalid input), and returns the
// parsed result. Free text after the choice token is returned as note.
//
// Choice vocabulary depends on family size:
//   - multi-option (len(fam.Members) > 1): "1".."N" picks that option,
//     "n" = neither is close.
//   - single-option: "y"/"1" = sounds like it, "p" = partly, "n" = no.
//
// "r"/"s"/"q" are always accepted regardless of family size. EOF (an
// exhausted piped stdin) is treated the same as "q" -- there is nothing
// left to answer with, so ending the session is the only sane response
// rather than looping forever on read errors.
//
// in is read ONE BYTE AT A TIME via readLine, deliberately NOT through a
// buffered reader. `ab-all` runs one `refs audition` PROCESS PER FAMILY,
// all sharing the same piped stdin (see the Makefile), so this process
// must consume EXACTLY the bytes belonging to its own answer and nothing
// more -- a bufio.Reader's read-ahead would silently pull the NEXT
// family's answer into an internal buffer that dies with this process,
// starving the next invocation even though its answer is still sitting,
// unread, in the pipe. This was a real bug (see
// TestPromptVerdict_DoesNotOverreadPastCurrentLine): "q" appeared to work
// in manual testing only because an EOF-starved second family produces the
// exact same visible "quit" output as a literal "q" would.
func promptVerdict(in io.Reader, out io.Writer, fam auditionFamily) (kind verdictKind, verdict string, chosen instrument.ID, note string, err error) {
	for {
		fmt.Fprint(out, promptLine(fam))
		line, gotAny := readLine(in)
		if !gotAny {
			// EOF (or any read error) with nothing read: treat as quit
			// rather than erroring the whole tool out from under an
			// interrupted/piped session.
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
			// a synonym for "y" -- a single-option family still has exactly
			// one "option 1", so a reflex numeric answer (as if it were a
			// multi-option family) reads naturally as "yes, that one sounds
			// right". Canonicalized to "y" in the recorded verdict either
			// way.
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

// commentHint tells the listener that a comment is welcome and shows the
// punctuation. The prompt previously accepted free text but advertised
// nothing, so the feature was only discoverable by guessing -- and the first
// natural guess ("n, ...") was rejected.
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

// readLine reads one '\n'-terminated line from r, one byte at a time --
// see promptVerdict's doc comment for why this must not be buffered.
// Returns gotAny=false only when the underlying reader produced nothing at
// all before erroring (a true EOF with no partial line pending); a final
// line with no trailing newline (EOF right after some bytes) still returns
// gotAny=true with whatever was read. Tolerates a trailing '\r' (CRLF).
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

// splitChoice separates the leading choice token from the free-text comment.
//
// It takes the token as the leading run of letters and digits rather than
// splitting on a space, because the natural way to write this answer is
// "n, both are way too airy" -- and splitting on a bare space yielded the token
// "n," which matched nothing, rejecting the answer and throwing away the
// comment. The comment is the only part of a verdict that records WHY, so the
// parser must not make punctuation the price of writing one.
//
// The token is lowercased; the note keeps its original case and its own
// internal punctuation.
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

// canonicalChoice maps the spelled-out forms of each choice onto its single
// letter. Someone answering a question in prose reaches for the word, not the
// mnemonic, and "no" meaning anything other than "n" here is not a reading
// anyone would intend. Anything unrecognised passes through unchanged so the
// caller still rejects it by name.
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

// noteDiscarded warns when a comment was attached to a NAVIGATION answer
// (quit/replay/skip), which records nothing and therefore has nowhere to keep
// it. Echoing the text back is the difference between the listener knowing to
// re-enter it and believing it was saved.
func noteDiscarded(out io.Writer, note, action string) {
	if note == "" {
		return
	}
	fmt.Fprintf(out, "note: %s records no verdict, so this comment was NOT saved: %q\n", action, note)
}
