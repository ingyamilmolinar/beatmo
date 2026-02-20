package audio

import "math"

// Compressor implements the Processor interface with peak-detecting
// compression. It applies gain reduction only when the signal exceeds
// a threshold, using an envelope follower with separate attack and
// release times. Designed for drum bus use: fast attack catches transients,
// moderate release avoids pumping.
//
// Parameters:
//   - ThresholdDB: level above which compression starts (default -6 dB)
//   - Ratio: compression ratio above threshold (default 4:1)
//   - AttackMs: attack time in milliseconds (default 1 ms, fast for drums)
//   - ReleaseMs: release time in milliseconds (default 50 ms)
//   - MakeupDB: post-compression gain boost (default 0 dB, auto-calculated if < 0)
//   - KneeDB: soft knee width in dB (default 0 = hard knee)
type Compressor struct {
	ThresholdDB float64
	Ratio       float64
	AttackMs    float64
	ReleaseMs   float64
	MakeupDB    float64
	KneeDB      float64

	// Internal state
	envelope    float64 // current envelope level (linear)
	attackCoef  float64 // smoothing coefficient for attack
	releaseCoef float64 // smoothing coefficient for release
	makeupGain  float64 // linear makeup gain
	initialized bool
	sampleRate  int
}

// NewCompressor creates a compressor with drum-optimized defaults.
func NewCompressor(sampleRate int) *Compressor {
	c := &Compressor{
		ThresholdDB: -6,
		Ratio:       4,
		AttackMs:    1,
		ReleaseMs:   50,
		MakeupDB:    0,
		KneeDB:      3,
		sampleRate:  sampleRate,
	}
	c.recalc()
	return c
}

func (c *Compressor) recalc() {
	sr := float64(c.sampleRate)
	if sr <= 0 {
		sr = 44100
	}
	// Time constant: coeff = exp(-1 / (time_in_seconds * sample_rate))
	if c.AttackMs > 0 {
		c.attackCoef = math.Exp(-1.0 / (c.AttackMs * 0.001 * sr))
	} else {
		c.attackCoef = 0
	}
	if c.ReleaseMs > 0 {
		c.releaseCoef = math.Exp(-1.0 / (c.ReleaseMs * 0.001 * sr))
	} else {
		c.releaseCoef = 0
	}
	c.makeupGain = math.Pow(10, c.MakeupDB/20)
	c.initialized = true
}

// computeGainDB returns the gain reduction in dB for a given input level in dB.
func (c *Compressor) computeGainDB(inputDB float64) float64 {
	thresh := c.ThresholdDB
	ratio := c.Ratio
	if ratio < 1 {
		ratio = 1
	}
	knee := c.KneeDB

	if knee > 0 {
		// Soft knee: quadratic interpolation in the knee region.
		halfKnee := knee / 2
		if inputDB < thresh-halfKnee {
			return 0 // Below knee: no compression.
		}
		if inputDB > thresh+halfKnee {
			// Above knee: full compression.
			return (inputDB - thresh) * (1 - 1/ratio)
		}
		// In knee region.
		x := inputDB - thresh + halfKnee
		return (1 - 1/ratio) * x * x / (2 * knee)
	}

	// Hard knee.
	if inputDB <= thresh {
		return 0
	}
	return (inputDB - thresh) * (1 - 1/ratio)
}

// masterCompressor is the shared compressor instance on the master channel.
// Set during audio initialization.
var masterCompressor *Compressor

// SetupMasterCompressor installs a compressor on the master audio channel.
// Called during audio init to enable automatic gain management.
func SetupMasterCompressor(sr int) {
	masterCompressor = NewCompressor(sr)
	AddChannelProcessor(mainChannelID, masterCompressor)
}

// ProcessBlockBuf implements BlockProcessor for Compressor. Hoists coefficients
// to locals for a tight inner loop matching ProcessSample math.
func (c *Compressor) ProcessBlockBuf(in, out []float32, samples int) {
	if !c.initialized {
		c.recalc()
	}
	env := c.envelope
	attackCoef := c.attackCoef
	releaseCoef := c.releaseCoef
	makeup := c.makeupGain
	thresh := c.ThresholdDB
	ratio := c.Ratio
	if ratio < 1 {
		ratio = 1
	}
	knee := c.KneeDB
	halfKnee := knee / 2

	for i := 0; i < samples; i++ {
		x := float64(in[i])
		peak := x
		if peak < 0 {
			peak = -peak
		}
		if peak > env {
			env = attackCoef*env + (1-attackCoef)*peak
		} else {
			env = releaseCoef*env + (1-releaseCoef)*peak
		}
		envDB := -96.0
		if env > 1e-6 {
			envDB = 20 * math.Log10(env)
		}
		var reductionDB float64
		if knee > 0 {
			if envDB < thresh-halfKnee {
				reductionDB = 0
			} else if envDB > thresh+halfKnee {
				reductionDB = (envDB - thresh) * (1 - 1/ratio)
			} else {
				kx := envDB - thresh + halfKnee
				reductionDB = (1 - 1/ratio) * kx * kx / (2 * knee)
			}
		} else {
			if envDB <= thresh {
				reductionDB = 0
			} else {
				reductionDB = (envDB - thresh) * (1 - 1/ratio)
			}
		}
		gainLinear := math.Pow(10, -reductionDB/20)
		out[i] = float32(x * gainLinear * makeup)
	}
	c.envelope = env
}

// ProcessSample implements the Processor interface.
func (c *Compressor) ProcessSample(x float64) float64 {
	if !c.initialized {
		c.recalc()
	}

	// Peak detection: use absolute value.
	peak := math.Abs(x)

	// Envelope follower with separate attack/release.
	if peak > c.envelope {
		c.envelope = c.attackCoef*c.envelope + (1-c.attackCoef)*peak
	} else {
		c.envelope = c.releaseCoef*c.envelope + (1-c.releaseCoef)*peak
	}

	// Convert envelope to dB.
	envDB := -96.0 // floor
	if c.envelope > 1e-6 {
		envDB = 20 * math.Log10(c.envelope)
	}

	// Compute gain reduction.
	reductionDB := c.computeGainDB(envDB)
	gainLinear := math.Pow(10, -reductionDB/20)

	return x * gainLinear * c.makeupGain
}
