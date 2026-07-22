//go:build test

package audio

import (
	"encoding/json"
	"testing"
)

func TestWavRefRoundTripWithSources(t *testing.T) {
	in := WavRef{
		Name: "my-808.wav", Sha256: "abc", Version: 3,
		Sources: []WavSource{
			{Kind: WavSourceFile, Ref: "/samples/my-808.wav"},
			{Kind: WavSourceLocalStorage, Ref: "idb:my-808"},
			{Kind: WavSourceURL, Ref: "https://example.com/my-808.wav"},
		},
	}
	raw, _ := json.Marshal(in)
	var out WavRef
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Sources) != 3 || out.Sources[0].Kind != WavSourceFile || out.Sources[2].Ref != "https://example.com/my-808.wav" {
		t.Errorf("sources not preserved: %+v", out.Sources)
	}
}

func TestResolveWavRefMatchedFirstSource(t *testing.T) {
	data := []byte("RIFF....WAVEfake-bytes")
	sha := Sha256Hex(data)
	ref := &WavRef{Sha256: sha, Sources: []WavSource{
		{Kind: WavSourceFile, Ref: "/x.wav"},
	}}
	res := ResolveWavRef(ref, map[string]WavLocator{
		WavSourceFile: func(string) ([]byte, bool) { return data, true },
	})
	if res.Status != WavResolveMatched {
		t.Fatalf("status = %v, want Matched", res.Status)
	}
	if len(res.Data) != len(data) {
		t.Errorf("bytes not returned: %+v", res)
	}
}

func TestResolveWavRefFallsThroughToSecondSource(t *testing.T) {
	data := []byte("good-wav-bytes")
	sha := Sha256Hex(data)
	ref := &WavRef{Sha256: sha, Sources: []WavSource{
		{Kind: WavSourceFile, Ref: "/missing.wav"},         // locator returns ok=false
		{Kind: WavSourceURL, Ref: "https://host/good.wav"}, // this one has the bytes
	}}
	res := ResolveWavRef(ref, map[string]WavLocator{
		WavSourceFile: func(string) ([]byte, bool) { return nil, false },
		WavSourceURL:  func(string) ([]byte, bool) { return data, true },
	})
	if res.Status != WavResolveMatched || res.Source.Kind != WavSourceURL {
		t.Fatalf("did not fall through to URL source: %+v", res)
	}
}

func TestResolveWavRefMismatch(t *testing.T) {
	want := Sha256Hex([]byte("the-right-bytes"))
	ref := &WavRef{Sha256: want, Sources: []WavSource{{Kind: WavSourceFile, Ref: "/wrong.wav"}}}
	res := ResolveWavRef(ref, map[string]WavLocator{
		WavSourceFile: func(string) ([]byte, bool) { return []byte("different-bytes"), true },
	})
	if res.Status != WavResolveMismatch {
		t.Fatalf("status = %v, want Mismatch", res.Status)
	}
	if res.GotSha == want || res.GotSha == "" {
		t.Errorf("mismatch should report the found hash: %q", res.GotSha)
	}
}

func TestResolveWavRefMissingAndUnknownKind(t *testing.T) {
	ref := &WavRef{Sha256: "x", Sources: []WavSource{
		{Kind: "future-kind", Ref: "?"}, // no locator → skipped
		{Kind: WavSourceFile, Ref: "/gone.wav"},
	}}
	res := ResolveWavRef(ref, map[string]WavLocator{
		WavSourceFile: func(string) ([]byte, bool) { return nil, false },
	})
	if res.Status != WavResolveMissing {
		t.Errorf("status = %v, want Missing", res.Status)
	}
	if ResolveWavRef(nil, nil).Status != WavResolveMissing {
		t.Error("nil ref should be Missing")
	}
}
