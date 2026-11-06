package scoreref

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// The Mutopia Project's MIDI files are generated from LilyPond sources, and
// the facts a MIDI file cannot carry live in that source: the licence the
// typesetter placed the edition under, and the pickup (\partial) that shifts
// every downbeat. These helpers read exactly those two things — nothing in
// the generator depends on parsing LilyPond music.

var (
	lyHeaderField = regexp.MustCompile(`(?m)^\s*([a-zA-Z]+)\s*=\s*"((?:[^"\\]|\\.)*)"`)
	lyPartial     = regexp.MustCompile(`\\partial\s+([0-9]+\.*(?:\*[0-9]+(?:/[0-9]+)?)?)`)
)

// LilyPondHeader returns the plain-string fields of a LilyPond source
// (title = "…", license = "…"). A field assigned a \markup is not a plain
// string and is absent. The first assignment of a name wins.
func LilyPondHeader(ly []byte) map[string]string {
	out := map[string]string{}
	for _, m := range lyHeaderField.FindAllSubmatch(ly, -1) {
		k := strings.ToLower(string(m[1]))
		if _, seen := out[k]; !seen {
			out[k] = string(m[2])
		}
	}
	return out
}

// LilyPondLicense returns the licence a Mutopia source declares. Mutopia
// spells it `license = "…"` in current files and `copyright = "…"` in older
// ones; a copyright assigned a \markup (the footer) is not a licence.
func LilyPondLicense(ly []byte) string {
	h := LilyPondHeader(ly)
	if v := strings.TrimSpace(h["license"]); v != "" {
		return v
	}
	return strings.TrimSpace(h["copyright"])
}

// LilyPondPickup returns the duration of the first \partial in a LilyPond
// source ("" when it has none). Several staves repeat the same \partial; the
// first is representative.
func LilyPondPickup(ly []byte) string {
	m := lyPartial.FindSubmatch(ly)
	if m == nil {
		return ""
	}
	return string(m[1])
}

// LilyPondDurationQuarters converts a LilyPond duration ("8", "4.", "8*2",
// "2*3/4") to quarter notes. "" is zero.
func LilyPondDurationQuarters(d string) (float64, error) {
	d = strings.TrimSpace(d)
	if d == "" {
		return 0, nil
	}
	mult := 1.0
	if i := strings.Index(d, "*"); i >= 0 {
		num, den := d[i+1:], "1"
		if j := strings.Index(num, "/"); j >= 0 {
			num, den = num[:j], num[j+1:]
		}
		n, err1 := strconv.Atoi(num)
		dn, err2 := strconv.Atoi(den)
		if err1 != nil || err2 != nil || dn == 0 {
			return 0, fmt.Errorf("bad duration multiplier in %q", d)
		}
		mult = float64(n) / float64(dn)
		d = d[:i]
	}
	dots := strings.Count(d, ".")
	d = strings.TrimRight(d, ".")
	base, err := strconv.Atoi(d)
	if err != nil || base <= 0 {
		return 0, fmt.Errorf("bad duration %q", d)
	}
	q := 4.0 / float64(base)
	add := q / 2
	for i := 0; i < dots; i++ {
		q += add
		add /= 2
	}
	return q * mult, nil
}

// LicenseFromLilyPond maps a Mutopia licence string to the catalogue's
// licence vocabulary, or "" when it is not one the allowlist could accept.
func LicenseFromLilyPond(v string) string {
	low := strings.ToLower(v)
	switch {
	case strings.Contains(low, "sharealike"), strings.Contains(low, "share alike"),
		strings.Contains(low, "noncommercial"), strings.Contains(low, "non-commercial"):
		return ""
	case strings.Contains(low, "public domain"):
		return "PD"
	case strings.Contains(low, "cc0"), strings.Contains(low, "zero"):
		return "CC0-1.0"
	case strings.Contains(low, "attribution 4.0"):
		return "CC-BY-4.0"
	}
	return ""
}
