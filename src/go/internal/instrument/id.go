package instrument

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// One of four shapes, minted only here: a roster id, UserID ("user.<slug>.<hash>"), UserSampleID ("user.sample.<hash>"),
// Instance ("<base>--N"); catalog samples carry their path's slug. Data goes through ParseID once; "" is no instrument.
// js.ValueOf panics on a named string type, so it crosses into JavaScript as string(id).
type ID string

// ReservedMasterName is the master bus's wire spelling ("main"): no
// instrument may carry it, so a channel name always says which bus it is.
const ReservedMasterName = "main"

// ErrEmptyID / ErrInvalidID are what ParseID rejects.
var (
	ErrEmptyID   = errors.New("instrument id is empty")
	ErrInvalidID = errors.New("invalid instrument id")
)

// ParseID validates an id read from data: non-empty, no whitespace or control
// characters, and not the master bus's name.
func ParseID(s string) (ID, error) {
	if s == "" {
		return "", ErrEmptyID
	}
	if s == ReservedMasterName {
		return "", fmt.Errorf("%w %q: reserved for the master bus", ErrInvalidID, s)
	}
	for _, r := range s {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return "", fmt.Errorf("%w %q: contains whitespace or a control character", ErrInvalidID, s)
		}
	}
	return ID(s), nil
}

// ParseIDs parses a list of ids read from data (a comma-separated flag, a
// JSON array of strings); the first invalid one is the error.
func ParseIDs(ss []string) ([]ID, error) {
	out := make([]ID, len(ss))
	for i, s := range ss {
		id, err := ParseID(s)
		if err != nil {
			return nil, err
		}
		out[i] = id
	}
	return out, nil
}

// No roster id contains it (the generator rejects an empty dash part), so an instance can never shadow a built-in.
const instanceSep = "--"

// Instance mints the id of the n-th row (2..9) of base in one circuit.
func Instance(base ID, n int) (ID, error) {
	if base == "" || n < 2 || n > 9 {
		return "", fmt.Errorf("%w: instance %d of %q (want a base and 2..9)", ErrInvalidID, n, base)
	}
	return ID(fmt.Sprintf("%s%s%d", base, instanceSep, n)), nil
}

// Only a single digit 1..9 after "--" makes an instance; "bass-808" and "x--10" are plain ids.
func (id ID) Base() ID {
	if n := len(id); n > len(instanceSep)+1 && strings.HasSuffix(string(id[:n-1]), instanceSep) {
		if c := id[n-1]; c >= '1' && c <= '9' {
			return id[:n-1-len(instanceSep)]
		}
	}
	return id
}

// IsInstance reports whether id is a circuit instance of another instrument.
func (id ID) IsInstance() bool { return id.Base() != id }

const (
	userPrefix       = "user."
	userSamplePrefix = "user.sample."
)

// Content addressed: the same tone saved under the same name is the same id.
func UserID(name, hashHex string) ID {
	return ID(userPrefix + Slug(name) + "." + hashHex)
}

// UserSampleID mints a user sample's id from a checksum of what identifies it.
func UserSampleID(sum uint32) ID {
	return ID(fmt.Sprintf("%s%08x", userSamplePrefix, sum))
}

// sampleSlugReplacer maps every path/punctuation character to '-'.
var sampleSlugReplacer = strings.NewReplacer(" ", "-", "_", "-", "/", "-", "\\", "-", ":", "-", "+", "-", ".", "-", "(", "-", ")", "-", "[", "-", "]", "-", "{", "-", "}", "-", ",", "-", "'", "-", "\"", "-", "#", "-", "$", "-", "%", "-", "@", "-", "!", "-", "^", "-", "&", "-", "*", "-", "?", "-", "|", "-", "<", "-", ">", "-", "~", "-", "`", "-")

// Separator and punctuation runs collapse to one '-' ("Snare/Hit 01" → "snare-hit-01"), so a sample id never contains
// "--" and is never read as an instance.
func SampleID(relPath string) ID {
	slug := sampleSlugReplacer.Replace(strings.ToLower(relPath))
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	slug = strings.Trim(slug, "-")
	if slug == "" || slug == ReservedMasterName {
		slug = "sample-" + slug
		slug = strings.TrimSuffix(slug, "-")
	}
	return ID(slug)
}

// IsUser reports whether id was minted by UserID or UserSampleID.
func (id ID) IsUser() bool { return strings.HasPrefix(string(id), userPrefix) }

// Slug lowercases name and turns every run of non-[a-z0-9] into one '-',
// trimmed, at most 24 characters ("instrument" when nothing is left).
func Slug(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	s := strings.TrimRight(b.String(), "-")
	if len(s) > 24 {
		s = strings.TrimRight(s[:24], "-")
	}
	if s == "" {
		s = "instrument"
	}
	return s
}

// Strings returns ids as plain strings, for a boundary that takes []string
// (JavaScript, a package that does not know instruments).
func Strings(ids []ID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = string(id)
	}
	return out
}
