# Layered Kick DSP (kick variant 5 "Hybrid") — Design

**Date:** 2026-06-28
**Status:** Approved (user chose "build dedicated layered-kick DSP")

## Problem

Tuning the existing kick voice (and insert FX) toward `403244 yellowtree
hybrid-kick-1` hit a hard ceiling after 7 architectures. The reference is a
layered sample whose defining qualities can't be made by a single synth voice:

| quality | reference | best single-voice synth | why it fails |
|---|---|---|---|
| **crest** (sharp impulse = punch) | 12.7 | ~2.6 | distortion/saturation flatten the transient peak |
| **centroid / brightness** (raw top) | 6346 Hz | ~2250 Hz | distorting a sub tops out ~2 kHz |
| **flatness** (broadband grit) | 0.50 | ~0.21 | the only path to 0.5 is white noise → reads as **hiss** |

The two requirements are in direct tension for one voice: "raw" grit needs
broadband noise (→ hiss), and "punch" needs an un-clipped spike (distortion
flattens it).

## Design — a layered kick (new variant 5 in `modular_gen_slot_kick`)

Three layers, combined so the tensions resolve:

1. **Pitch-swept harmonic body** — `sin(φ) + h2·sin(2φ) + h3·sin(3φ)` with a fast
   deep pitch envelope (`pe_amt`/`pe_rate`) and amp env `exp(-env0·t)`. The pitch
   sweep is the kick's "click"/punch tonally.
2. **Envelope-tracked grit, fused via distortion** — `grit = white()·noiseAmt·bodyEnv`
   (noise AMPLITUDE-MODULATED by the body envelope, so it decays WITH the body —
   no isolated hiss tail), summed with the body and then **waveshaped**:
   `distorted = softclip((body·bodyEnv + grit)·drive)`. Distorting tone+noise
   together spreads energy broadband (raw flatness/brightness) AND the grit
   inherits the body's decay → the tail is clean, not hiss. `drive` is heavy.
3. **Sharp transient click, added POST-distortion** — `click = hpNoise·clickAmt·exp(-clickRate·t)`
   (clickRate fast → ~3 ms tick), added AFTER the waveshaper so its peak is NOT
   flattened by the distortion. This brief loud spike on top of the lower
   distorted body gives the **high crest** (punch) — and being ~3 ms it's a
   tick, not a hiss.

```
out = (distorted · attackBoost + click) · globalFade ; clamp
```

- **Crest (punch):** `out` peak = the click spike ≫ distorted-body RMS → high crest.
- **No hiss:** grit ∝ bodyEnv (decays with body); click is brief.
- **Raw/bright:** waveshaping body+grit → broadband harmonic grit.

## Params — reuse the existing kick knobs (NO ABI change)

Variant 5 reads the existing per-slot kick params (re-purposed for this voice):
`kick_pe_amt`/`kick_pe_rate` (pitch sweep), `kick_env0` (body decay), `kick_h2`/`kick_h3`
(body harmonics), `kick_sat` (**distortion drive**, heavy), `kick_noise` (**grit**
amount into the distortion), `kick_click` (**post-distortion transient** amount),
`kick_attack` (body attack boost), `kick_fade` (global fade). Click decay rate is
baked fast (~3 ms). So: **no new struct fields, no ABI regen** — only:
- bump the `kick_variant` ParamDef Max 4 → 5 (`kickBounds` in `modular_recipe.go`)
  + add the "Hybrid" enum label (`kickStageMeta`),
- add the `variant == 5` branch in `modular_gen_slot_kick` (`src/c/modular_stages.c`),
- `punchyKickSeed` → `gen1_kick_variant: 5` + tune.

## Tests (TDD)

- **Crest**: variant-5 render has crest ≥ ~6 (well above the ~3 ceiling of the
  other variants) — the post-distortion click spike.
- **Grit decays with body (no hiss tail)**: HF fraction in the late tail (>120 ms)
  is LOW (≤ ~a few %) — the grit isn't an independent sustained hiss.
- **Brightness**: spectral centroid materially higher than a clean kick.
- **Finite/bounded**: the completeness test renders `kick_variant`=MAX(5) at
  extremes → must stay finite (guard the waveshaper/noise).
- Clean: raw render 0 hard-clipped samples at the tuned seed.

## Out of scope

- New ABI params (reuse existing knobs).
- Changing the other 5 kick variants (additive).
