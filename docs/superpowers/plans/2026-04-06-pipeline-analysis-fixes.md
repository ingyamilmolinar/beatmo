# Pipeline Analysis Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the broken StageEQ tap, eliminate master bus clipping, and improve the startup demo mix quality based on scope export flight recorder analysis.

**Architecture:** Add a per-instrument post-EQ scratch buffer to the mixer so the StageEQ tap captures actual post-EQ data. Reduce headroom-stage gain to prevent master bus clipping. Update startup_demo.json with stereo panning, send effects, and rebalanced volumes.

**Tech Stack:** Go 1.23, C DSP (miniaudio), JSON

---

## File Map

| File | Change | Task |
|------|--------|------|
| `src/go/internal/audio/engine_mixer.go` | Add `postEQBuf []float64` scratch buffer to mixer struct | 1 |
| `src/go/internal/audio/engine_stop.go` | Rewrite Phase 2 to use postEQBuf, fix StageEQ tap, fix StageInsertFX tap | 1 |
| `src/go/internal/audio/engine_stop.go` | Reduce headroom multiplier from 0.25 to 0.18 | 2 |
| `src/go/internal/assets/startup_demo.json` | Add panning, send effects, rebalance volumes | 3 |

---

### Task 1: Fix StageEQ Tap — Add Post-EQ Per-Instrument Buffer

**Problem:** `ProcessBlockLocal(instBuf, masterBuf)` reads from instBuf and accumulates into masterBuf. The instBuf is never mutated, so the StageEQ push reads the same pre-EQ data as StageAntiPop.

**Solution:** Add a `postEQBuf` scratch buffer to the mixer. In Phase 2, for each active instrument: zero `postEQBuf`, call `ProcessBlockLocal(instBuf, postEQBuf)`, tap StageEQ from `postEQBuf`, then accumulate `postEQBuf` into `masterBuf`.

**Files:**
- Modify: `src/go/internal/audio/engine_mixer.go:20-44`
- Modify: `src/go/internal/audio/engine_stop.go:468-496`
- Test: `src/go/internal/scopeexport/service_test.go` (new integration test)

- [ ] **Step 1: Add postEQBuf to mixer struct**

In `src/go/internal/audio/engine_mixer.go`, add a new buffer field:

```go
// In mixer struct, after masterBuf:
postEQBuf []float64 // Per-instrument scratch buffer for post-EQ tap
```

In `newMixer()`, allocate it alongside the existing buffers (find the allocation site and add):

```go
postEQBuf: make([]float64, blockSize),
```

- [ ] **Step 2: Find newMixer and add allocation**

Run: `grep -n "func newMixer" src/go/internal/audio/engine_mixer.go`

Read the function and add `postEQBuf: make([]float64, blockSize)` to the struct literal.

- [ ] **Step 3: Rewrite Phase 2 to use postEQBuf**

In `src/go/internal/audio/engine_stop.go`, replace the Phase 2 block processing (lines ~468-496) with:

```go
// === PHASE 2: Per-instrument channel processing → masterBuf ===
if !bypassChannelProc {
    for _, slot := range m.activeSlots {
        id := m.instSlotIDs[slot]
        ch := channelForInstrument(id)
        // Zero the scratch buffer for this instrument.
        for k := 0; k < blockLen; k++ {
            m.postEQBuf[k] = 0
        }
        // Process into scratch buffer (not directly into masterBuf).
        ch.ProcessBlockLocal(m.instBufs[slot][:blockLen], m.postEQBuf[:blockLen])
        // Tap the post-EQ per-instrument signal.
        if scopeSvc != nil {
            scopeSvc.PushSamples(scope.StageEQ, id, m.postEQBuf[:blockLen])
        }
        if exportSvc != nil {
            exportSvc.PushSamples(scope.StageEQ, id, m.postEQBuf[:blockLen])
        }
        // Accumulate into masterBuf.
        for k := 0; k < blockLen; k++ {
            m.masterBuf[k] += m.postEQBuf[k]
        }
    }
} else {
    // Bypass: direct sum from instrument buffers
    for _, slot := range m.activeSlots {
        buf := m.instBufs[slot]
        for j := 0; j < blockLen; j++ {
            m.masterBuf[j] += buf[j]
        }
    }
}
```

Remove the old separate StageEQ push blocks (both scope and export) that came after Phase 2, since they're now inside the loop.

- [ ] **Step 4: Run full test suite**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./...`
Expected: All pass (no test directly exercises the mixer, but parity tests verify audio consistency)

- [ ] **Step 5: Verify with scope export**

Run: `SCOPE_EXPORT=1 SCOPE_EXPORT_INTERVAL=0.5 SCOPE_EXPORT_PATH=/tmp/scope_verify.jsonl timeout 15 /tmp/beatmo -demo -scope-export -log NONE`

Then verify StageEQ now differs from StageAntiPop:

```bash
head -1 /tmp/scope_verify.jsonl | jq -r '.channels[] | .id + ": antipop_peak=" + (.stages.antipop.peak_db | tostring) + " eq_peak=" + (.stages.eq.peak_db | tostring)'
```

Expected: `antipop_peak != eq_peak` for instruments with EQ applied.

- [ ] **Step 6: Commit**

```bash
git add src/go/internal/audio/engine_mixer.go src/go/internal/audio/engine_stop.go
git commit -m "fix(mixer): capture actual post-EQ signal for StageEQ tap

ProcessBlockLocal reads from instBuf and accumulates into masterBuf
without modifying instBuf. Both StageAntiPop and StageEQ were reading
the same unmodified buffer. Add a postEQBuf scratch buffer so Phase 2
writes per-instrument post-EQ data before accumulating into masterBuf."
```

---

### Task 2: Eliminate Master Sends Clipping

**Problem:** 7 instruments sum into masterBuf, producing +0.44 dB peak and 9 clipped samples at the sends stage. The master compressor catches it downstream, but the sends signal is already distorted.

**Solution:** Reduce the per-instrument headroom attenuation from 0.25x (-12 dB) to 0.18x (-14.9 dB). This gives ~3 dB more headroom before clipping when 7 instruments sum. The master compressor's -6 dB threshold will still engage to bring the mix to a comfortable level.

**Files:**
- Modify: `src/go/internal/audio/engine_stop.go` (headroom constant)

- [ ] **Step 1: Find the headroom constant**

Run: `grep -n "headroom\|0\.25" src/go/internal/audio/engine_stop.go | head -10`

Identify the headroom multiplier (likely `0.25` or a named constant).

- [ ] **Step 2: Reduce headroom to 0.18**

Change the headroom multiplier from `0.25` to `0.18`:

```go
// Before: headroom = 0.25 (-12 dB)
// After:  headroom = 0.18 (-14.9 dB, ~3 dB more headroom for dense mixes)
```

- [ ] **Step 3: Run tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./...`

- [ ] **Step 4: Verify no clipping with scope export**

Run the demo again and check sends stage:

```bash
SCOPE_EXPORT=1 SCOPE_EXPORT_INTERVAL=0.5 SCOPE_EXPORT_PATH=/tmp/scope_headroom.jsonl timeout 15 /tmp/beatmo -demo -scope-export -log NONE
jq '.master.stages.sends | {peak_db, clip_count}' /tmp/scope_headroom.jsonl
```

Expected: `clip_count: 0` for all ticks, `peak_db < 0`.

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/audio/engine_stop.go
git commit -m "fix(mixer): reduce headroom to 0.18x to prevent sends-stage clipping

7-instrument demo was producing +0.44 dB at the sends stage (9 clips).
Reducing from 0.25x (-12dB) to 0.18x (-14.9dB) gives ~3dB extra
headroom for dense mixes while the master compressor still normalizes."
```

---

### Task 3: Improve Startup Demo Mix Quality

**Problem:** All instruments are center-panned, no send effects are used, and volumes could be better balanced for a professional-sounding demo.

**Files:**
- Modify: `src/go/internal/assets/startup_demo.json`

- [ ] **Step 1: Update startup_demo.json with improved mix**

Apply these changes to the instruments array in `startup_demo.json`:

**Panning (stereo image):**
- kick-deep: `"pan": 0` (center — anchors the low end)
- snare: `"pan": 0` (center — backbone of the beat)
- hihat: `"pan": 0.35` (slightly right — classic rock/pop placement)
- clap: `"pan": -0.15` (slightly left — complement to snare)
- tom: `"pan": -0.3` (left — standard tom placement)
- cowbell: `"pan": 0.45` (right — percussive accent)
- fm-epiano-1: `"pan": -0.2` (slightly left — melodic balance)

**Send effects (depth and space):**
- snare: `"reverb_send": 0.15` (touch of room ambience)
- clap: `"reverb_send": 0.25, "delay_send": 0.1` (clap benefits from space)
- hihat: `"delay_send": 0.08` (subtle rhythmic delay)
- cowbell: `"reverb_send": 0.2` (metallic reverb tail)
- fm-epiano-1: `"reverb_send": 0.3, "delay_send": 0.15` (lush keyboard sound)

**Volume rebalance (account for reduced headroom from Task 2):**
- kick-deep: `"volume": 0.9` (was 0.85 — dominant element)
- snare: `"volume": 0.8` (was 0.75 — boost to cut through)
- hihat: `"volume": 0.55` (was 0.65 — reduce, was competing)
- clap: `"volume": 0.4` (was 0.45 — slight reduction)
- tom: `"volume": 0.6` (was 0.55 — boost for presence)
- cowbell: `"volume": 0.25` (was 0.3 — reduce, was too prominent)
- fm-epiano-1: `"volume": 0.4` (was 0.35 — boost for melodic content)

- [ ] **Step 2: Verify JSON is valid**

Run: `cat src/go/internal/assets/startup_demo.json | jq . > /dev/null && echo OK`

- [ ] **Step 3: Rebuild and test with scope export**

```bash
cd src/go && CGO_ENABLED=1 ../../.tools/go/bin/go build -o /tmp/beatmo ./cmd
SCOPE_EXPORT=1 SCOPE_EXPORT_INTERVAL=0.5 SCOPE_EXPORT_PATH=/tmp/scope_mix.jsonl timeout 15 /tmp/beatmo -demo -scope-export -log NONE
```

Verify:
```bash
# Check panning is applied
head -1 /tmp/scope_mix.jsonl | jq '.channels[] | {id, pan}'

# Check no clipping at any stage
jq '[.channels[].stages[].clip_count, .master.stages[].clip_count] | add' /tmp/scope_mix.jsonl

# Check EQ stage now differs from AntiPop
head -1 /tmp/scope_mix.jsonl | jq -r '.channels[] | .id + " antipop=" + (.stages.antipop.rms_db | . * 10 | round / 10 | tostring) + " eq=" + (.stages.eq.rms_db | . * 10 | round / 10 | tostring)'
```

- [ ] **Step 4: Run full test suite**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./...`

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/assets/startup_demo.json
git commit -m "mix(demo): add stereo panning, send effects, rebalance volumes

Scope export analysis showed: all instruments center-panned, no send
effects used, unbalanced volumes. Add stereo image (hihat right, tom
left, cowbell far right), reverb/delay sends for depth, and rebalance
volumes to account for the reduced headroom multiplier."
```

---

## Verification

After all tasks, run the complete verification:

```bash
# 1. Full test suite
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./...

# 2. Scope export analysis
cd /home/ymolinar/Repos/beatmo
SCOPE_EXPORT=1 SCOPE_EXPORT_INTERVAL=0.5 SCOPE_EXPORT_PATH=/tmp/scope_final.jsonl timeout 15 /tmp/beatmo -demo -scope-export -log NONE

# 3. Verify all findings are fixed
echo "=== Finding 1: StageEQ should differ from StageAntiPop ==="
head -1 /tmp/scope_final.jsonl | jq -r '.channels[] | .id + " Δ(eq-antipop)=" + ((.stages.eq.rms_db - .stages.antipop.rms_db) | . * 10 | round / 10 | tostring) + "dB"'

echo "=== Finding 2: No sends clipping ==="
jq '.master.stages.sends.clip_count' /tmp/scope_final.jsonl

echo "=== Finding 4: Panning applied ==="
head -1 /tmp/scope_final.jsonl | jq -r '.channels[] | .id + " pan=" + (.pan | tostring)'

echo "=== Finding 5: Send effects active ==="
head -1 /tmp/scope_final.jsonl | jq '.master.stages.sends.rms_db'
```

Expected results:
- Finding 1: non-zero Δ(eq-antipop) for instruments with EQ
- Finding 2: `clip_count: 0` for all ticks
- Finding 4: non-zero pan values for hihat, clap, tom, cowbell, fm-epiano
- Finding 5: sends RMS should be higher than without send effects
