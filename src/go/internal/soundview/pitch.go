package soundview

import (
	"math"
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// PitchTrack is pitch over time: one estimate per HopSec (10 ms), frame i
// centred at T0 + i*HopSec. Hz[i] == 0 means unvoiced (no clear period, or
// too quiet to judge).
type PitchTrack struct {
	T0     float64
	HopSec float64
	Hz     []float64
}

// MedianHz is the median voiced pitch (0 when nothing is voiced).
func (p PitchTrack) MedianHz() float64 {
	var v []float64
	for _, hz := range p.Hz {
		if hz > 0 {
			v = append(v, hz)
		}
	}
	if len(v) == 0 {
		return 0
	}
	sort.Float64s(v)
	return v[len(v)/2]
}

// VoicedFraction is the share of frames with a pitch.
func (p PitchTrack) VoicedFraction() float64 {
	if len(p.Hz) == 0 {
		return 0
	}
	n := 0
	for _, hz := range p.Hz {
		if hz > 0 {
			n++
		}
	}
	return float64(n) / float64(len(p.Hz))
}

// HzAt is the voiced pitch nearest to time t (0 when none within 50 ms).
func (p PitchTrack) HzAt(t float64) float64 {
	if p.HopSec <= 0 || len(p.Hz) == 0 {
		return 0
	}
	c := int(math.Round((t - p.T0) / p.HopSec))
	for d := 0; d <= 5; d++ {
		for _, i := range []int{c - d, c + d} {
			if i >= 0 && i < len(p.Hz) && p.Hz[i] > 0 {
				return p.Hz[i]
			}
		}
	}
	return 0
}

const (
	pitchMinHz     = 25.0
	pitchMaxHz     = 3000.0
	pitchHopSec    = 0.01
	pitchTargetSR  = 12000
	yinThreshold   = 0.15
	yinUnvoiced    = 0.35
	pitchQuietDrop = 40.0 // a frame 40 dB under the loudest is unvoiced
	// pitchMinPeriods is how many periods a window must hold for its estimate
	// to count (2.5: a kick's 60 Hz mid-glide fits the 43 ms window).
	pitchMinPeriods = 2.5
)

// YIN on a ~12 kHz decimated copy (FFT autocorrelation), MULTI-RESOLUTION: 21, 43 then 85 ms windows, keeping the
// first holding 2.5 periods. A kick's 150 → 44 Hz drop lives inside one long window, while a 44 Hz tail needs the long
// window to be judged at all.
func (a *Analysis) Pitch() PitchTrack {
	a.pitchOnce.Do(a.computePitch)
	return a.pitch
}

var pitchWindows = [...]int{256, 512, 1024}

func (a *Analysis) computePitch() {
	defer a.markDone(NeedPitch)
	d := int(math.Ceil(float64(a.sr) / pitchTargetSR))
	x := decimateYield(a.pcm[:a.analysedLen()], d, func(j int) { a.yieldEvery(j, 2048) })
	fs := float64(a.sr) / float64(d)
	hop := max(1, int(math.Round(fs*pitchHopSec)))
	tr := PitchTrack{T0: float64(pitchWindows[0]/2) / fs, HopSec: float64(hop) / fs}
	if len(x) < pitchWindows[0] {
		a.pitch = tr
		return
	}
	env := a.Envelope()
	quiet := env.MaxRMS() - pitchQuietDrop
	yins := make([]*yinFrame, len(pitchWindows))
	for i, w := range pitchWindows {
		yins[i] = newYinFrame(w, fs)
	}
	// Each frame starts at the window the previous one settled on (one shorter if three periods fit), so a steady tail costs
	// one transform per frame, not three.
	level := 0
	for f := 0; ; f++ {
		c := pitchWindows[0]/2 + f*hop
		if c >= len(x) {
			break
		}
		a.yieldEvery(f, 1)
		hz := 0.0
		if env.RMSAt(float64(c)/fs) >= quiet {
			for i := level; i < len(pitchWindows); i++ {
				w := pitchWindows[i]
				if w > len(x) {
					break
				}
				st := max(0, min(len(x)-w, c-w/2))
				est, ok := yins[i].estimate(x[st : st+w])
				if ok && (est >= pitchMinPeriods*fs/float64(w) || i == len(pitchWindows)-1) {
					hz, level = est, i
					break
				}
			}
		}
		switch {
		case hz <= 0:
			level = 0
		case level > 0 && hz >= pitchMinPeriods*fs/float64(pitchWindows[level-1]):
			level--
		}
		tr.Hz = append(tr.Hz, hz)
	}
	a.pitch = tr
}

// decimate is decimate over the analysed PCM, yielding while Warm runs.
func (a *Analysis) decimate(d int) []float64 {
	return decimateYield(a.pcm, d, func(j int) { a.yieldEvery(j, 2048) })
}

// decimate low-passes (63-tap windowed sinc, cutoff 0.42·fs/d) and keeps every
// d-th sample. d == 1 copies to float64.
func decimate(pcm []float32, d, sr int) []float64 {
	return decimateYield(pcm, d, nil)
}

func decimateYield(pcm []float32, d int, step func(j int)) []float64 {
	if d <= 1 {
		out := make([]float64, len(pcm))
		for i, s := range pcm {
			out[i] = float64(s)
		}
		return out
	}
	const taps = 63
	fc := 0.42 / float64(d) // cycles per input sample
	h := make([]float64, taps)
	sum := 0.0
	for k := range h {
		m := float64(k - taps/2)
		sinc := 2 * fc
		if m != 0 {
			sinc = math.Sin(2*math.Pi*fc*m) / (math.Pi * m)
		}
		h[k] = sinc * (0.54 - 0.46*math.Cos(2*math.Pi*float64(k)/float64(taps-1)))
		sum += h[k]
	}
	for k := range h {
		h[k] /= sum
	}
	n := len(pcm) / d
	out := make([]float64, n)
	for j := range out {
		if step != nil {
			step(j)
		}
		c := j * d
		acc := 0.0
		for k := range h {
			if i := c + k - taps/2; i >= 0 && i < len(pcm) {
				acc += h[k] * float64(pcm[i])
			}
		}
		out[j] = acc
	}
	return out
}

// yinFrame is one window size's reusable YIN state.
type yinFrame struct {
	w              int
	fs             float64
	tauMin, tauMax int
	buf            []complex128
	cum, dp        []float64
}

func newYinFrame(w int, fs float64) *yinFrame {
	tauMax := min(w/2, int(fs/pitchMinHz))
	return &yinFrame{
		w: w, fs: fs,
		tauMin: max(2, int(fs/pitchMaxHz)), tauMax: tauMax,
		buf: make([]complex128, 2*w), cum: make([]float64, w+1), dp: make([]float64, tauMax+2),
	}
}

// estimate returns the frame's pitch and whether it is voiced.
func (y *yinFrame) estimate(frame []float64) (float64, bool) {
	w, n := y.w, 2*y.w
	for i := range y.buf {
		y.buf[i] = 0
	}
	for i, v := range frame {
		y.buf[i] = complex(v, 0)
		y.cum[i+1] = y.cum[i] + v*v
	}
	if y.cum[w] <= 0 {
		return 0, false
	}
	wave.FFT(y.buf)
	for i := range y.buf {
		re, im := real(y.buf[i]), imag(y.buf[i])
		y.buf[i] = complex(re*re+im*im, 0)
	}
	// The power spectrum is real and symmetric, so a forward transform IS
	// the inverse up to 1/n: buf[tau] = n·r(tau), the linear autocorrelation.
	wave.FFT(y.buf)
	dp := y.dp
	dp[0] = 1
	running := 0.0
	for tau := 1; tau <= y.tauMax+1 && tau < w; tau++ {
		dv := y.cum[w-tau] + (y.cum[w] - y.cum[tau]) - 2*real(y.buf[tau])/float64(n)
		if dv < 0 {
			dv = 0
		}
		running += dv
		if running <= 0 {
			dp[tau] = 1
		} else {
			dp[tau] = dv * float64(tau) / running
		}
	}
	best := -1
	for tau := y.tauMin; tau <= y.tauMax; tau++ {
		if dp[tau] < yinThreshold {
			for tau+1 <= y.tauMax && dp[tau+1] < dp[tau] {
				tau++
			}
			best = tau
			break
		}
	}
	if best < 0 {
		m := math.Inf(1)
		for tau := y.tauMin; tau <= y.tauMax; tau++ {
			if dp[tau] < m {
				m, best = dp[tau], tau
			}
		}
		if m > yinUnvoiced {
			return 0, false
		}
	}
	t := float64(best)
	if best > y.tauMin && best < y.tauMax {
		a, b, c := dp[best-1], dp[best], dp[best+1]
		if den := a - 2*b + c; den > 0 {
			t += 0.5 * (a - c) / den
		}
	}
	return y.fs / t, true
}
