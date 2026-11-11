package audio

import "testing"

// SampleEditSignature is the Sampler tab's external-change detector; it had
// no direct test (0% in the native coverage run).
func TestSampleEditSignature(t *testing.T) {
	const id = "sig-test-instrument"
	t.Cleanup(func() { ClearSampleEdit(id) })

	if got := SampleEditSignature(id); got != 0 {
		t.Fatalf("signature with no descriptor = %d, want 0", got)
	}

	SetSampleEdit(id, SampleEdit{StartFrac: 0.1, EndFrac: 0.9, GainDB: -3})
	sig1 := SampleEditSignature(id)
	if sig1 == 0 {
		t.Fatal("signature with descriptor = 0, want non-zero")
	}
	if again := SampleEditSignature(id); again != sig1 {
		t.Fatalf("signature not stable: %d then %d", sig1, again)
	}

	// Any field change must change the fingerprint.
	SetSampleEdit(id, SampleEdit{StartFrac: 0.1, EndFrac: 0.9, GainDB: -3, Reverse: true})
	if sig2 := SampleEditSignature(id); sig2 == sig1 {
		t.Fatal("signature unchanged after editing the descriptor")
	}

	ClearSampleEdit(id)
	if got := SampleEditSignature(id); got != 0 {
		t.Fatalf("signature after clear = %d, want 0", got)
	}
}
