//go:build !js

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

func absentLocalRef() wavref.Ref {
	return wavref.Ref{
		ID: "adopted-on-another-machine", Tier: wavref.TierInstrument,
		File:   "adopted-on-another-machine.wav",
		Source: &wavref.Source{Provider: localProvider, URL: "/no/such/dir/take.wav"},
	}
}

func withCatalogue(t *testing.T, refs ...wavref.Ref) {
	t.Helper()
	orig := catalogueLoader
	t.Cleanup(func() { catalogueLoader = orig })
	catalogueLoader = func() (wavref.Cat, error) { return wavref.Cat{Refs: refs}, nil }
	t.Setenv(wavref.EnvVar, t.TempDir())
}

func TestFetchSkipsALocalRefAbsentHereOnlyWhenAsked(t *testing.T) {
	withCatalogue(t, absentLocalRef())
	var out bytes.Buffer
	if err := runFetch(&out, fetchOpts{}); err == nil {
		t.Fatal("a missing local source must fail a plain fetch")
	}
	out.Reset()
	if err := runFetch(&out, fetchOpts{SkipAbsentLocal: true}); err != nil {
		t.Fatalf("-skip-absent-local still failed: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "adopted-on-another-machine") || !strings.Contains(out.String(), "local-only") {
		t.Fatalf("the skipped ref must be named as local-only:\n%s", out.String())
	}
}

func TestFetchStillFailsAPublicRefWhenSkippingLocalOnes(t *testing.T) {
	public := wavref.Ref{
		ID: "public-but-unreachable", Tier: wavref.TierInstrument, File: "p.wav",
		Source: &wavref.Source{Provider: "vcsl", URL: "http://127.0.0.1:1/unreachable.wav"},
	}
	withCatalogue(t, absentLocalRef(), public)
	var out bytes.Buffer
	if err := runFetch(&out, fetchOpts{SkipAbsentLocal: true}); err == nil {
		t.Fatalf("an unreachable public ref must still fail:\n%s", out.String())
	}
}

func TestWholeCorpusChecksSkipWhileARefIsMissing(t *testing.T) {
	present := wavref.Ref{ID: "here", Tier: wavref.TierInstrument, File: "here.wav"}
	missing := wavref.Ref{ID: "not-here", Tier: wavref.TierInstrument, File: "not-here.wav"}
	withCatalogue(t, present, missing)
	if err := os.WriteFile(filepath.Join(wavref.Root(), present.File), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	ran := map[string]bool{}
	orig := corpusChecks
	t.Cleanup(func() { corpusChecks = orig })
	corpusChecks = []corpusCheck{
		{name: "whole", doc: "calibrated over every ref", wholeCorpus: true,
			run: func(*checkReport) error { ran["whole"] = true; return nil }},
		{name: "part", doc: "reads the refs it names",
			run: func(*checkReport) error { ran["part"] = true; return nil }},
	}
	var out bytes.Buffer
	if err := runChecksFrom(&out, nil, false, catalogueLoader); err != nil {
		t.Fatalf("runChecks: %v\n%s", err, out.String())
	}
	if ran["whole"] || !ran["part"] {
		t.Fatalf("ran %v: the whole-corpus check must skip and the other run\n%s", ran, out.String())
	}
	if !strings.Contains(out.String(), "SKIP  whole") || !strings.Contains(out.String(), "not-here") {
		t.Fatalf("the skip must name the missing ref:\n%s", out.String())
	}
}
