// Package objects is the versioned store for game objects (spec
// docs/superpowers/specs/2026-10-31-versioned-objects-and-residency-revisions-design.md
// Part 2): content-addressed objects plus, per identity, the history of its
// saved versions and a current pointer.
//
// A version is the SHA-256 of the object's canonical JSON (Go's
// encoding/json: struct fields in declaration order, map keys sorted), so it is
// immutable, the same on every machine, and the digest a shared address (CIDv1,
// raw codec, sha2-256) wraps. Callers strip identity fields before hashing.
package objects

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/ingyamilmolinar/beatmo/internal/enum"
)

// Kind is what an object is.
type Kind int

const (
	KindNone Kind = iota
	KindBundle
	KindInstrument
	KindCircuit
)

var kindNames = enum.Names[Kind]{Kind: "object kind", Names: []string{"", "bundle", "instrument", "circuit"}}

func (k Kind) String() string                { return enum.String(kindNames, k) }
func (k Kind) MarshalText() ([]byte, error)  { return enum.Marshal(kindNames, k) }
func (k *Kind) UnmarshalText(b []byte) error { return enum.Unmarshal(kindNames, b, k) }

// Digest is "sha256:" + the 64-hex SHA-256 of v's JSON encoding.
func Digest(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic("objects: Digest of an unmarshalable value: " + err.Error())
	}
	return digestBytes(b)
}

func digestBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// Object is one stored version: its kind, canonical body and the digests of
// the objects it points to.
type Object struct {
	Kind Kind            `json:"kind"`
	Body json.RawMessage `json:"body"`
	Refs []string        `json:"refs,omitempty"`
}

// History is an identity's saved versions in save order. Current indexes
// Versions; -1 is the identity's base (its shipped version, or none).
type History struct {
	Versions []string `json:"versions"`
	Current  int      `json:"current"`
}

// Store holds objects by digest and histories by identity.
type Store struct {
	Objects   map[string]Object   `json:"objects"`
	Histories map[string]*History `json:"histories"`
}

func NewStore() *Store {
	return &Store{Objects: map[string]Object{}, Histories: map[string]*History{}}
}

// Put stores body as an object of kind pointing to refs and returns its digest.
func (s *Store) Put(kind Kind, body any, refs ...string) string {
	b, err := json.Marshal(body)
	if err != nil {
		panic("objects: Put of an unmarshalable value: " + err.Error())
	}
	d := digestBytes(b)
	if s.Objects == nil {
		s.Objects = map[string]Object{}
	}
	s.Objects[d] = Object{Kind: kind, Body: b, Refs: slices.Clone(refs)}
	return d
}

func (s *Store) Get(digest string) (Object, bool) {
	o, ok := s.Objects[digest]
	return o, ok
}

// Body decodes the stored body of digest into v.
func (s *Store) Body(digest string, v any) error {
	o, ok := s.Objects[digest]
	if !ok {
		return fmt.Errorf("objects: no object %s", digest)
	}
	return json.Unmarshal(o.Body, v)
}

// Save appends digest to identity's history and makes it current. Saving the
// current version again changes nothing.
func (s *Store) Save(identity, digest string) {
	if s.Histories == nil {
		s.Histories = map[string]*History{}
	}
	h := s.Histories[identity]
	if h == nil {
		h = &History{Current: -1}
		s.Histories[identity] = h
	}
	if h.Current >= 0 && h.Versions[h.Current] == digest {
		return
	}
	h.Versions = append(h.Versions, digest)
	h.Current = len(h.Versions) - 1
}

// Current is identity's current saved version; false when the pointer is on
// the base (or nothing was saved).
func (s *Store) Current(identity string) (string, bool) {
	h := s.Histories[identity]
	if h == nil || h.Current < 0 {
		return "", false
	}
	return h.Versions[h.Current], true
}

// Move points identity at one of its saved versions (past or future).
func (s *Store) Move(identity, digest string) error {
	h := s.Histories[identity]
	if h != nil {
		if i := slices.Index(h.Versions, digest); i >= 0 {
			h.Current = i
			return nil
		}
	}
	return fmt.Errorf("objects: %s has no version %s", identity, digest)
}

// MoveToBase points identity at its base, keeping every saved version.
func (s *Store) MoveToBase(identity string) {
	if h := s.Histories[identity]; h != nil {
		h.Current = -1
	}
}

// Prune keeps, per identity, the newest limit versions and the current one,
// then drops every object no kept version, pinned digest or kept object's
// refs reach.
func (s *Store) Prune(limit int, pinned ...string) {
	var roots []string
	for _, h := range s.Histories {
		cur := ""
		if h.Current >= 0 {
			cur = h.Versions[h.Current]
		}
		cut := max(0, len(h.Versions)-limit)
		kept := slices.Clone(h.Versions[cut:])
		if cur != "" && !slices.Contains(kept, cur) {
			kept = append([]string{cur}, kept...)
		}
		h.Versions = kept
		h.Current = slices.Index(kept, cur)
		roots = append(roots, kept...)
	}
	roots = append(roots, pinned...)
	reached := map[string]bool{}
	for len(roots) > 0 {
		d := roots[len(roots)-1]
		roots = roots[:len(roots)-1]
		if reached[d] {
			continue
		}
		reached[d] = true
		roots = append(roots, s.Objects[d].Refs...)
	}
	for d := range s.Objects {
		if !reached[d] {
			delete(s.Objects, d)
		}
	}
}
