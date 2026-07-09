package fingerprint

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// synthKick builds a deterministic kick-shaped signal: a short bright attack
// (fast-decaying broadband click) + a decaying low sine body + an optional long
// low tail. attackBright scales the click's high content; tailAmp adds a low tail.
func synthKick(sr int, f0, attackBright, tailAmp float64) wave.Wave {
	n := sr * 6 / 10 // 0.6 s
	x := make([]float64, n)
	for i := range x {
		t := float64(i) / float64(sr)
		// low sine body, decays over ~120 ms
		body := math.Sin(2*math.Pi*f0*t) * math.Exp(-8*t)
		// bright click: a decaying high tone (~2.5 kHz) in the first ~8 ms
		click := attackBright * math.Sin(2*math.Pi*2500*t) * math.Exp(-350*t)
		// long low tail (reverb-ish), GATED to start after ~140 ms so it is
		// temporally isolated in the TAIL segment (changing tailAmp then only
		// affects the tail region).
		tail := 0.0
		if t > 0.14 {
			tail = tailAmp * math.Sin(2*math.Pi*(f0*0.9)*t) * math.Exp(-6*(t-0.14))
		}
		x[i] = body + click + tail
	}
	return wave.Wave{Samples: x, SampleRate: sr}
}

func TestKickSegmentAnalyze_Structure(t *testing.T) {
	const sr = 44100
	ks := KickSegmentAnalyze(synthKick(sr, 60, 0.5, 0.05))
	if len(ks.Segments) != 3 {
		t.Fatalf("want 3 segments, got %d", len(ks.Segments))
	}
	a, b, c := ks.Segments[0], ks.Segments[1], ks.Segments[2]
	if a.Name != SegAttack || b.Name != SegBody || c.Name != SegTail {
		t.Fatalf("segment names = %s/%s/%s", a.Name, b.Name, c.Name)
	}
	// Ordering: attack < body < tail, strictly increasing, spanning the signal.
	if !(a.StartSec <= a.EndSec && a.EndSec <= b.EndSec && b.EndSec <= c.EndSec) {
		t.Fatalf("segments not ordered: %v %v %v", a, b, c)
	}
	if a.EndSec > 0.08 {
		t.Fatalf("attack segment too long: ends at %.0f ms (want <= 80)", a.EndSec*1000)
	}
	// The attack is the brightest segment (the click lives there) — a genuine
	// per-segment metric (unlike crest, which is confounded by segment length).
	if a.Centroid <= b.Centroid {
		t.Fatalf("attack centroid %.0f should exceed body centroid %.0f (the click is bright)", a.Centroid, b.Centroid)
	}
	// Energy fractions cover the whole signal (~1) and the attack+body hold most.
	sum := a.EnergyFrac + b.EnergyFrac + c.EnergyFrac
	if sum < 0.9 || sum > 1.1 {
		t.Fatalf("energy fractions sum = %.3f, want ~1", sum)
	}
}

func TestKickSegmentDistance_TargetsTheRightSegment(t *testing.T) {
	const sr = 44100
	ref := KickSegmentAnalyze(synthKick(sr, 60, 0.5, 0.05))

	// Identical → ~0.
	if d := KickSegmentDistance(ref, ref); d.Total > 1e-6 {
		t.Fatalf("identity distance = %.6f, want ~0", d.Total)
	}

	// A kick that differs ONLY in the attack brightness → the top-ranked delta
	// must be the ATTACK centroid (the actionable target), and the attack segment
	// must carry more distance than the tail.
	brightAttack := KickSegmentAnalyze(synthKick(sr, 60, 2.0, 0.05))
	d := KickSegmentDistance(ref, brightAttack)
	if len(d.Deltas) == 0 {
		t.Fatal("no deltas")
	}
	top := d.Deltas[0]
	if top.Segment != SegAttack {
		t.Fatalf("top divergence is in %q/%q, want the attack segment", top.Segment, top.Metric)
	}
	if d.PerSegment[SegAttack] <= d.PerSegment[SegTail] {
		t.Fatalf("attack distance %.3f should exceed tail distance %.3f", d.PerSegment[SegAttack], d.PerSegment[SegTail])
	}

	// A kick that differs ONLY in the tail level → the tail segment must carry the
	// most distance (energyFrac / tail metrics), NOT the attack.
	bigTail := KickSegmentAnalyze(synthKick(sr, 60, 0.5, 0.4))
	dt := KickSegmentDistance(ref, bigTail)
	if dt.PerSegment[SegTail] <= dt.PerSegment[SegAttack] {
		t.Fatalf("tail distance %.3f should exceed attack distance %.3f when only the tail changed",
			dt.PerSegment[SegTail], dt.PerSegment[SegAttack])
	}
}
