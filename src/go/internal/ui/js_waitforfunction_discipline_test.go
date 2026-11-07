//go:build test

package ui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Playwright's signature is (fn, arg, options): options in the second slot become the page function's
// ARGUMENT and the wait silently runs on the 30 s default with rAF polling. The wrong call never throws or
// times out early, so only a guard surfaces it.
func TestWaitForFunctionOptionsAreNotPassedAsTheArgument(t *testing.T) {
	jsFiles, err := filepath.Glob(repoPath("js", "*.js"))
	if err != nil || len(jsFiles) == 0 {
		t.Fatalf("no JS files found (err=%v)", err)
	}
	call := regexp.MustCompile(`waitForFunction\s*\(`)
	optionsObject := regexp.MustCompile(`^\s*\{[^{}]*\b(timeout|polling)\s*:`)
	var bad []string
	for _, f := range jsFiles {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		src := string(data)
		for _, loc := range call.FindAllStringIndex(src, -1) {
			args, ok := jsCallArgs(src, loc[1]-1)
			// A trailing comma leaves an empty last argument; still the two-argument form.
			if ok && len(args) > 0 && strings.TrimSpace(args[len(args)-1]) == "" {
				args = args[:len(args)-1]
			}
			if !ok || len(args) != 2 {
				continue
			}
			if optionsObject.MatchString(args[1]) {
				line := 1 + strings.Count(src[:loc[0]], "\n")
				bad = append(bad, filepath.Base(f)+":"+itoa(line))
			}
		}
	}
	for _, b := range bad {
		t.Errorf("%s: waitForFunction(fn, {timeout|polling…}) passes the options as the page ARGUMENT — use waitForFunction(fn, null, {…})", b)
	}
}

// Chromium reports "Promise was collected" when the engine warm resolves audioReady under a pending evaluate,
// and Playwright rewrites it into a false "context destroyed by a navigation". waitForAudioReady(page) observes
// settlement through an in-page flag instead.
func TestBrowserTestsNeverAwaitAudioReadyOverTheProtocol(t *testing.T) {
	jsFiles, err := filepath.Glob(repoPath("js", "*.browser.test.js"))
	if err != nil || len(jsFiles) == 0 {
		t.Fatalf("no browser tests found (err=%v)", err)
	}
	call := regexp.MustCompile(`\.evaluate\s*\(`)
	awaited := regexp.MustCompile(`await\s+window\.audioReady\b|=>\s*\(?\s*window\.audioReady\s*\)?\s*$|return\s+window\.audioReady\b`)
	var bad []string
	for _, f := range jsFiles {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		src := string(data)
		for _, loc := range call.FindAllStringIndex(src, -1) {
			args, ok := jsCallArgs(src, loc[1]-1)
			if !ok || len(args) == 0 {
				continue
			}
			if awaited.MatchString(strings.TrimSpace(args[0])) {
				line := 1 + strings.Count(src[:loc[0]], "\n")
				bad = append(bad, filepath.Base(f)+":"+itoa(line))
			}
		}
	}
	for _, b := range bad {
		t.Errorf("%s: an evaluate awaits window.audioReady over CDP (\"Promise was collected\" → fake \"navigation\" error) — use waitForAudioReady(page)", b)
	}
}

// Skips strings, template literals (with ${…}) and comments; ok=false when the call is unterminated.
func jsCallArgs(s string, open int) ([]string, bool) {
	depth, start := 0, open+1
	var args []string
	for j := open; j < len(s); j++ {
		switch c := s[j]; {
		case c == '"' || c == '\'' || c == '`':
			q := c
			for j++; j < len(s) && s[j] != q; j++ {
				if s[j] == '\\' {
					j++
				} else if q == '`' && s[j] == '$' && j+1 < len(s) && s[j+1] == '{' {
					d := 1
					for j += 2; j < len(s) && d > 0; j++ {
						if s[j] == '{' {
							d++
						} else if s[j] == '}' {
							d--
						}
					}
					j--
				}
			}
		case c == '/' && j+1 < len(s) && s[j+1] == '/':
			for j < len(s) && s[j] != '\n' {
				j++
			}
		case c == '/' && j+1 < len(s) && s[j+1] == '*':
			end := strings.Index(s[j+2:], "*/")
			if end < 0 {
				return nil, false
			}
			j += end + 3
		case c == '(' || c == '[' || c == '{':
			depth++
		case c == ')' || c == ']' || c == '}':
			depth--
			if depth == 0 {
				return append(args, s[start:j]), true
			}
		case c == ',' && depth == 1:
			args = append(args, s[start:j])
			start = j + 1
		}
	}
	return nil, false
}
