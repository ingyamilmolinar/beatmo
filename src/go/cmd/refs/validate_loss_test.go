//go:build !js

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio/clone"
)

func TestLossVerdictNeedsTenPairsToCallATermAnti(t *testing.T) {
	for _, c := range []struct {
		agree, total int
		want         string
	}{{0, 9, "too few pairs"}, {4, 10, "ANTI → weight 0"}, {5, 10, "agrees"}, {9, 12, "agrees"}} {
		if got := lossVerdict(c.agree, c.total); got != c.want {
			t.Errorf("%d/%d: %q, want %q", c.agree, c.total, got, c.want)
		}
	}
}

func TestLossTermsRankLikeTheLoss(t *testing.T) {
	near := lossTermNorms(clone.LossReport{MRSTFT: 0.2, Envelope: 1, Descriptors: 0.1}, 0.5)
	far := lossTermNorms(clone.LossReport{MRSTFT: 0.9, Envelope: 6, Descriptors: 0.8}, 2)
	pairs := []prefPair{{Family: "f"}}
	scores := scorePreferencesFromNorms(pairs, []map[string]float64{near}, []map[string]float64{far})
	for _, id := range lossTermOrder {
		if scores[id] != [2]int{1, 1} {
			t.Errorf("%s: %v, want 1/1", id, scores[id])
		}
	}
	var b bytes.Buffer
	printLossAgreement(&b, 1, scores)
	if !strings.Contains(b.String(), "loss.mrstft") || !strings.Contains(b.String(), "fingerprint.distance") {
		t.Fatalf("table:\n%s", b.String())
	}
}
