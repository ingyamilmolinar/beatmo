## Drum Synthesis Guidance (Miniaudio)

The C synth in `src/c/drums.c` is shared by desktop (`internal/audio`) and WebAudio (`src/js/audio.js` via `drums.single.js`). When altering drum timbres, keep these principles in mind so future changes stay musical and “organic” instead of brittle or plasticky.

### General

- Prefer **noise-led designs** with subtle pitched components, especially for snare and hats. Let noise carry most of the character; use sines mainly for weight and ring.
- Use **separate envelopes** for tone vs noise paths (and even for different noise bands). This is key for avoiding “digital spit” and for getting punch + tail.
- For anything more than trivial shaping, use **RBJ biquad filters** (low-pass and band-pass) instead of single-pole filters so spectral focus is stable and controllable.
- Keep per-hit **variation**: tiny detune of body oscillators and small LFOs on noise bands help avoid static, machine-like tone.

### Snare (current design)

- **Body (tone)**:
  - Two decaying sines around low snare fundamentals (≈180–220 Hz and ≈280–340 Hz), with a tiny per-hit detune.
  - Apply a **very gentle downward pitch drift** across the hit for realism, but keep the body relatively quiet in the final mix so the snare doesn’t turn into a synth-bass.
  - Decay should be **shorter than the noise tail**: enough ring to feel like a drum, but most of the length should come from noise.

- **Noise (head + wires)**:
  - One white-noise source, split via biquads into:
    - A **low band** (~300–400 Hz) for the head “paper” / thud.
    - One or more **mid bands** (~1.5–3.5 kHz) for snare wires.
    - An optional **higher band** (~4–6 kHz) for crispness; keep this subtle to avoid clappy, brittle tails.
  - Shape the bands with separate envelopes:
    - Low band: slower decay for weight (short thud, not a drone).
    - Mid bands: very fast attack (crack) plus a shorter tail; this tail sets perceived “ring” more than the pitched body.
  - To reduce “digital” feel:
    - Emphasize low/mid noise, de-emphasize the highest band.
    - Keep high-band tails fairly short; long, full-band tails will feel like hiss instead of metal.

- **Attack & mix**:
  - For rock, it’s fine to add a **front-end gain boost** on the mid/high bands for the first few ms; just keep the boost moderate so it doesn’t sound like a clap.
  - Aim for a **noise-led mix**:
    - Low band ~1.0×, wires ~0.8–0.95×, body ~0.5–0.7× is a good ballpark.
  - Use a gentle global fade (based on normalized index) to keep the overall length sensible while still allowing ring.

### Kick (current design)

- **Body (tone)**:
  - Built from a **harmonic sine stack** anchored around ≈55 Hz:
    - Fundamental (`f0`) carries most of the weight.
    - 2nd harmonic (`2·f0`) adds low‑mid punch.
    - 3rd harmonic (`3·f0`) adds a small amount of “knock” at the front.
  - All partials share a **small, fast pitch envelope** (≈+10% at onset, decaying in a few ms) so the kick has punch without a “laser” sweep.
  - Amplitude envelopes are exponential and **shorter for higher harmonics** so the tail becomes round and deep:
    - Fundamental: slowest decay (main thump).
    - 2nd harmonic: medium decay (body).
    - 3rd harmonic: fastest decay (attack edge only).

- **Dark noise thud (rawness)**:
  - A single white‑noise source is **heavily low‑passed** (simple one‑pole) so only low/mid “thud” remains; the high band is effectively removed.
  - A short exponential envelope (tens of ms) shapes this low‑passed noise into a **very dark, quiet thump** that sits under the sines.
  - The noise layer is always **much lower in level** than the tonal body—just enough to add organic, mic‑like complexity and rawness without hiss or 8‑bit fizz.

- **Attack shaping**:
  - A front‑loaded gain boost (a simple exponential shape) is applied to the tonal stack for the first few milliseconds; this makes the kick feel aggressive and “front‑heavy” without lengthening it.
  - The noise thud does not need a separate attack boost; its very short envelope already confines it to the onset.

- **Global envelope & loudness**:
  - A gentle **global fade in normalized time** (based on sample index) ensures the waveform is near zero at the buffer end, avoiding truncation clicks while preserving enough body to feel bombastic.
  - The final mix (tone + dark noise) goes through a **very mild soft saturation**:
    - Drive is intentionally low so tanh only rounds peaks and adds subtle harmonics.
    - This keeps the kick loud and assertive while avoiding obvious digital distortion.

- **Design notes / generalization**:
  - Keep the **fundamental dominant**, with harmonics and noise clearly subordinate. When the 2nd/3rd harmonic or noise level approaches the fundamental, the kick quickly becomes “spitty” or “clicky”.
  - Use **tiny pitch envelopes** on the tone (≤10–15%) for punch; larger sweeps read as electronic/industrial.
  - Noise should be:
    - Strongly low‑passed,
    - Very short,
    - Mixed quietly.
    Otherwise it will sound like hiss or an 8‑bit explosion instead of a skin thud.
  - If a future kick variant needs more aggression, prefer:
    - Slightly stronger 2nd harmonic and a tiny bit more softsat drive,
    - Or a slightly faster pitch envelope,
    rather than adding wideband noise or heavy distortion.

### Hi-hat (current design)

- **Closed hat**:
  - Use an **808-style cluster of inharmonic square oscillators** in the upper mids/highs (roughly 4–12 kHz), with slight per-hit detune and random initial phases.
  - Sum the cluster, then **high-pass** it to remove low hum and DC; optionally add a high-passed white-noise layer for extra sizzle.
  - Envelopes:
    - Fast but not instantaneous attack; you want a clear transient without “click”.
    - Moderate decay for a closed hat in rock grooves; tails should overlap at 8ths but not smear across beats.
  - Avoid overly strong very-high bands (>12 kHz) unless needed; they quickly sound brittle in low-fi speakers or browser audio.

- **Open hat**:
  - Reuse the same source and filters, but:
    - Lengthen both oscillator and noise envelopes.
    - Use sparser patterns (`logic_kind: "probability"` or “every_n”) so open hats are accents, not constant wash.
  - Consider slightly lower center frequencies (e.g., 3–9 kHz) for open hats to avoid harshness when they ring longer.

### Tom (current design)

- **Body (tone)**:
  - Multiple decaying modes built from a downward-sweeping sine stack (fundamental plus a couple of overtones) starting around ≈180–220 Hz and ending near 80–100 Hz.
  - The pitch envelope is slower than a kick’s, so the tom’s note is clearly audible and feels like a shell, not a click.
  - The amplitude envelope is longer than the snare body, giving noticeable ring but still decaying to near-zero by the end of the buffer to avoid truncation artifacts.

- **Noise / attack**:
  - A short, low-passed noise burst simulates the **stick impact**.
  - Its envelope is extremely fast so it only colors the onset and doesn’t read as an explosion or 8-bit noise tail.

- **Mix**:
  - The body dominates; noise is mainly there to provide a transient.
  - Small per-hit detune makes repeated tom hits feel more organic.

### Clap (current design)

- **Structure**:
  - Several tightly spaced **noise bursts** (white noise) at staggered times (≈0, 20, 40 ms, plus a slightly later echo) to approximate multiple hands clapping and a tiny bit of room.
  - A shared exponential decay envelope controls the overall tail.

- **Character**:
  - Everything is noise-led; there is no pitched component.
  - The choice of burst spacing and relative amplitudes sets whether it feels like one clap, multiple hands, or a clap + room reflection.

### Cowbell (current design)

- **Body (tone)**:
  - 3–4 inharmonic sine partials in a mid/high range (roughly 600–2 kHz), per-hit detuned for variation.
  - A relatively slow exponential decay gives a clear **metallic ring**, with the groove pattern (not the voice length) responsible for stopping the sound.

- **Noise / impact**:
  - A short, low-passed noise burst simulates stick impact on the surface.
  - A small amount of **high-passed noise tail** adds metallic rasp without turning into hiss.

- **Mix**:
  - Tone dominates; noise is mostly perceived at the onset.
  - Saturation is used to keep peaks in check and add a bit of harmonic complexity.

### Tin-like / Can Cowbell Variant

During open-hat experiments we arrived at a complementary **tin‑can / sheet‑metal** character that is distinct from the more natural cowbell above:

- **Perceived feel**:
  - Sounds like **hitting a piece of tin** or a thin metal can rather than a tuned cowbell.
  - Sits somewhere between a very bright, treble‑heavy closed hat and a mid‑range cowbell: sharp onset, short-to-medium ring, and a “clangy” mid/high emphasis.

- **Tone structure**:
  - Uses inharmonic partials clustered slightly higher and more tightly than the natural cowbell body, with less focus on a clear fundamental pitch.
  - Decays are shorter/steeper on the lowest partials so the ear latches onto the mid/high bands, reinforcing the tin‑like impression.

- **Noise / impact**:
  - The impact noise is more **mid‑focused and slightly louder** than on the natural cowbell, emphasizing stick-on-thin‑metal rather than a rounded bell body.
  - Little or no sustained high‑passed noise tail is needed; most of the metallic rasp comes from the partials themselves, not from a separate noise band.

- **Use cases / generalization**:
  - Works well as a **digital percussion colour** for syncopated accents (e.g., house/techno‑style open hats or metallic layers), especially when layered with a darker closed hat.
  - To design similar tin‑like instruments:
    - Shift inharmonic partials upward into ~1–5 kHz and tighten their spacing.
    - Shorten low‑frequency decays relative to mid/high decays.
    - Use a slightly stronger, mid‑band impact noise burst to evoke thin metal.
    - Avoid long very‑high‑band tails; they quickly read as hiss instead of metal.

### Learnings from Snare Improvement

Key insight: **Body decay must be faster than noise tail**. When the pitched body rings as long as or longer than the noise, the snare sounds "tonal" or "tom-like" instead of crisp and snappy.

- The snare body serves to provide initial **punch and weight**, not sustain.
- Noise (head + wires) should carry most of the perceived tail.
- Use separate, faster decay on body (≈28× exponential rate) vs noise layers (≈12–18× exponential rate).

### Enhanced Tom Guidance (808-style)

**808 Tom Reference** (from research):

| Tom | Frequency Range | Decay Time |
|-----|-----------------|------------|
| High | 165–220 Hz | ~100ms |
| Mid | 120–160 Hz | ~130ms |
| Low | 80–100 Hz | ~200ms |

**Key 808 characteristics**:
- Bridged-T oscillator creates **resonant attack**.
- Diode-controlled pitch sweep: "bright, resonant attack that quickly transitions into a dull, non-resonant decay".
- Subtle pink noise layer for "artificial reverb effect" / room ambience.
- **Faster pitch envelope** than kick (note is audible, but decays to stable shell resonance quickly).

**Implementation principles**:
1. **Faster pitch envelope** (exp(-18·t) instead of exp(-8·t)) — 808-style quick sweep for punch.
2. **Resonant attack boost** — transient brightness that fades to warm body (1.0 + 0.4·exp(-80·t)).
3. **Mid-band stick noise** — use two-pole lowpass + highpass for bandpass-like character, not just one-pole.
4. **Longer stick envelope** — exp(-120·t) instead of exp(-260·t) for more presence.
5. **Subtle ambient noise** — very quiet, long-decay noise layer for organic room feel.
6. **Longer body decay** — fundamental ~exp(-2.8·t), overtones slightly faster, for more shell ring.

**Anti-pattern**: Aggressive post-processing lowpass filters (e.g., 260 Hz, 320 Hz) destroy transient character and make toms sound muffled/digital. Keep the natural brightness; use gentle saturation instead.

### Pattern & feel (UI/JSON side)

- Avoid firing hats on **every subdivision** at high tempos. In the default rock demo we:
  - Drive closed hats mostly on 8ths (with accents and probabilities), not every 16th.
  - Let open hats sit on off-beats with longer durations and lower probabilities.
- For snares, keep **kick heavy and snare strong but not constant**; use `logic_kind: "probability"` / `every_n_triggers` on ghost notes and occasional accents.

When adjusting drum DSP, test both:
- A simple 1–2 bar groove for feel (rock backbeat, basic hat pattern).
- A more exposed pattern (single snare hits, isolated hats) to judge tails, brightness, and "plasticky versus organic" character.

### String-like / Plucked Synthesis (legacy cowbell variant)

An earlier `render_cowbell` iteration in `src/c/drums.c` ended up sounding closer to a **plucked string / banjo-like tone** than a traditional cowbell. This structure is useful as a reference for future string-like synthesis:

- Two inharmonic sines (≈560 Hz and ≈840 Hz) with mild per-hit detune.
- Moderate exponential decay on the tone (`exp(-14·sec)`), giving a clear note-like ring.
- Short, mid-focused noise “impact” at the front (low-passed white noise with very fast decay) to simulate pick/stick contact.
- Soft saturation after mixing to keep levels musical.

If you want a plucked-string variant of another instrument, follow this pattern:

1. Pick 2–3 inharmonic partials in the desired register.
2. Apply a relatively slow decay to the tone and a very fast decay to the noise impact.
3. Keep noise mostly in mid bands (low-pass or band-pass), not full-band hiss.
4. Use tiny per-hit detune and optional slow LFO on amplitude or phase for organic variance.

