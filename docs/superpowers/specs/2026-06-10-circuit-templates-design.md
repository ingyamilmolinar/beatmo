# Circuit Templates — Design

**Date:** 2026-06-10
**Status:** Approved design, ready for planning
**Branch:** add-core-node-types

## Goal

Ship a **Load template** feature: a new entry in the overflow ("…") menu that opens
a cascading submenu of first-party, genre-specific circuits. Selecting one imports
it immediately, replacing the current circuit. Seven genres ship at launch: **rock,
hip-hop, pop, funk, salsa, house, techno** — each with an instrument palette and a
graph shape that match the genre.

Each template is its own bundled JSON file under `internal/assets/templates/`,
**generated** from a declarative Go spec (not hand-authored), so geometry, node IDs,
edges, and instrument tuning are correct by construction and round-trip-safe.

## Decisions (locked)

- **Picker UX:** cascading submenu off the overflow menu (reuses the existing portal
  machinery). Not a modal gallery, not a flat inline list.
- **Replace safety:** load immediately, no confirmation — consistent with today's
  Import behaviour (`g.Import` is already replace-not-merge).
- **Musical richness:** rich & authentic — genre-true BPM, swing/groove micro-timing,
  logic nodes for fills/variation, melodic basslines via per-node pitch.
- **Palette:** full builtin palette (drums + `bass-guitar`/`sub-bass` + FM synths +
  `modular`) where genre-appropriate. **No new C instruments** are created.

## How the engine maps a graph to a rhythm (the load-bearing fact)

Confirmed by reading `core/model/graph_traversal.go:71-107` (NOT the node-count model
an exploratory pass first guessed):

- **Distance = time.** Between two explicit nodes joined by an orthogonal edge, the
  traversal synthesizes one *invisible/silent* step per grid unit of i- or j-distance.
- An instrument fires **only** at its explicit `regular` nodes; straight segments
  between them are silent travel time.
- `subdiv` (top-level) = grid units per beat.
- A loop's length in beats = its **total perimeter** (grid units) ÷ `subdiv`.
- Verified against `parity_fixture_simple.json`: an 8×8 square at `subdiv:8` = 32-unit
  perimeter = 4 beats = 1 bar, firing the kick at its 4 corners = four-on-the-floor.

**Authoring primitive — the square loop.** Each instrument row is its own closed
orthogonal loop. For a 1-bar 4/4 loop at `subdiv:16`: 1 beat = 16 units, 1 bar = 64
units. A **16×16 square** has perimeter 64 = 1 bar, and the 16 sixteenth-note slots map
to positions evenly around its perimeter (4 per side). Salsa uses a **32×32 square**
(perimeter 128 = 2 bars) for the son-clave span.

The generator walks the perimeter; at each sixteenth-slot that carries a hit it places
a `regular` node (the instrument fires), and at each unused **corner** it places a
`silent` node (turns the path without firing). Consecutive nodes are joined by edges in
perimeter order, closing the loop. Every edge is therefore colinear/orthogonal, which
the traversal requires.

Each row gets its own square offset into a distinct `j` band so the rows don't overlap
visually; all rows share the same perimeter so they stay phase-locked across the bar
(salsa rows share the 2-bar perimeter).

## Instruments: selection + tuning, not creation

All 44 builtins already exist and are bound to recipes (`AGENTS.synth.md` § Recipe
Registry). Genre-authentic sound is produced through three JSON-supported levers, all
set by the generator per role:

1. **Base recipe choice** — pick the variant whose C character fits the role
   (`kick-tight` acoustic, `kick-deep` 808, `kick-punchy` 909, etc.).
2. **Per-instrument `synth_params`** — the 8 knobs (`pitch, decay, tone, attack, drive,
   body, color, brightness`) shipped as effective-deltas (export format already supports
   `recipe` + `synth_params` per instrument).
3. **Per-node `pitch` / `duration` / `volume`** — confirmed in `startup_demo.json`
   (node 9 `pitch:15`). Melodic rows play real lines; ghosts are low-volume nodes;
   accents are high-volume nodes.

Plus per-node `logic_kind` (`probability` / `every_n_triggers`) for evolving fills and
`groove_kind`/`groove_pct` (`delay`/`rush`) for swing and push/pull micro-timing.

**Pattern & Feel rule applied** (`AGENTS.synth.instruments.md:674-681`): do not fire
hats on every subdivision at high tempo — closed hats ride 8ths + accents + probability.
Funk and techno hats use 8ths + `probability` fill-nodes, not straight 16ths.

### Per-genre instrument map

Notation below: 16 sixteenth steps/bar `|1 e & a|2 e & a|3 e & a|4 e & a|`, `X` = hit,
lower-case = special (swing/ghost/probability as noted). "Open hat" = `hihat` with raised
`decay` (no separate open-hat instrument id exists; matches how `startup_demo` reuses one
hat).

**Rock — 120 BPM, straight, foursquare**
```
Crash    X . . . . . . . . . . . . . . .   beat-1 accent
Hihat    X . X . X . X . X . X . X . X .   driving 8ths
Snare    . . . . X . . . . . . . X . . .   backbeat 2 & 4
Kick     X . . . . . . . X . X . . . . .   1, 3, &-of-3
Bass-gtr X . . . . . . . X . X . . . . .   roots lock to kick (per-node pitch)
```
Recipes: `kick-tight` (drive+.1); `snare` (drive+.25, tone+.2); `hihat` (brightness+.2);
`crash`; `bass-guitar`. Touches: room reverb send on snare, hats panned right.

**Hip-Hop — 88 BPM, swung boom-bap**
```
Hihat    X x X x X x X x X x X x X x X x   16ths, offbeat = swing (groove:delay)
Clap     . . . . X . . . . . . . X . . .   layers snare
Snare    . . . . X . . . . . . . X . . .   2 & 4
Kick     X . . . . . X . . . X . . . . .   1, &-of-2, 3
Sub-bass X . . . . . X . . . X . . . . .   deep, follows kick (per-node pitch)
```
Recipes: `kick-deep` (drive+.15); `snare` (tone-.1); `clap`; `hihat` (brightness-.1);
`sub-bass`. Touches: tape on master (vinyl warmth), short snare reverb.

**Pop — 120 BPM, bright & hooky**
```
Fm-pluck X . X . X . X . X . X . X . X .   8th-note hook (per-node pitch = melody)
Hihat    X . X . X . X . X . X . X . X .   8ths
Clap     . . . . X . . . . . . . X . . .   2 & 4
Kick     X . . . . . . . X . . . X . . .   1, 3, 4
Sub-bass X . . . X . . . X . . . X . . .   on-beat root
```
Recipes: `kick-punchy`; `clap` (bright + reverb); `hihat` (brightness+.15); `fm-pluck`;
`sub-bass`. Touches: fm-pluck wide pan + delay send.

**Funk — 105 BPM, syncopated, "on the one"**
```
Hihat    X . X g X . X . X . X g X . X .   8ths + accents + g = probability 16th
Snare    . . . . X . . . . . . . X . . .   backbeat
Snr-ghost. . . g . . g . g . . . . . g .   ghost notes (low volume)
Kick     X . . . . . . X . . X . . . . .   THE ONE + syncopation
Bass-gtr X . . X . . X X . . X . . X . .   slap-style interlock (per-node pitch)
```
Recipes: `kick-tight`; `snare` (drive+.2, tone+.15); `snare-ghost`; `hihat`;
`bass-guitar` (drive+.15). `g` = `logic_kind:"probability"` (~0.6) or low-vol ghost as
marked. Touches: bass-guitar mid bump, tight hats panned.

**Salsa — ~190 BPM feel, 2-bar son clave (2-3), 32 steps**
```
(|bar1|bar2|)
Clave(ss)  . . X . . . X . . . . . X . . . | . . . . X . X . . . . . . . . .
Cowbell    X . . . X . . . X . . . X . . . | X . . . X . . . X . . . X . . .   campana
Conga(tom) . . X . X . . X . . X . X . . X | . . X . X . . X . . X . X . . X   tumbao (hi/lo per-node pitch)
Fm-epiano  . . . . . . X . . . . . . . X . | . . . . . . X . . . . . . . X .   montuno (per-node pitch)
Bass-gtr   . . . . . . X . . . . . X . . . | . . . . . . X . . . . . X . . .   tumbao (anticipated, per-node pitch)
Shaker     X . X . X . X . X . X . X . X . | X . X . X . X . X . X . X . X .   8ths
```
Recipes: `sidestick` (clave); `cowbell` (brightness+.2, tone+.1); `tom` (conga);
`fm-epiano` (montuno); `bass-guitar` (tumbao); `shaker`. Touches: piano panned, conga
reverb.

**House — 124 BPM, four-on-the-floor**
```
Open-hat . . X . . . X . . . X . . . X .   offbeat "tss" (the &)
Clap     . . . . X . . . . . . . X . . .   2 & 4
Hihat    X . X . X . X . X . X . X . X .   closed 8ths
Kick     X . . . X . . . X . . . X . . .   every beat
Fm-bass  . . X . . . X . . . X . . . X .   rolling offbeat bass (per-node pitch)
Fm-epiano. . X . . . . . . . X . . . . .   off-beat chord stab (per-node pitch)
```
Recipes: `kick` (body+.2); `clap` (+ reverb); `hihat` (open = decay↑; closed = default);
`fm-bass`; `fm-epiano`. Touches: warm sub, open-hat decay.

**Techno — 132 BPM, driving & evolving**
```
Modular  . . . . . . . . X . . . . . . .   sparse texture stab
Open-hat . . X . . . X . . . X . . . X .   offbeat
Clap     . . . . . . . . . . . . X . . .   beat 4 only (sparse)
Hihat    X . X p X . X . X . X p X . X .   8ths + p = probability 16th fill
Kick     X . . . X . . . X . . . X . . .   relentless 4-on-floor
Fm-bass  X . X . X . X . X . X . X . X .   acid line (per-node pitch, resonant)
```
Recipes: `kick-punchy` (body+.25, drive+.1); `clap`; `rimshot` (added perc, optional);
`hihat` (open = decay↑); `fm-bass` (drive+.3, tone+.2 = acid); `modular`. `p` =
`logic_kind:"probability"` (~0.6) so hats evolve each loop. Touches: tight kick + sub,
delay send on modular stab, dark master EQ.

## Architecture

Five focused units, each independently testable:

### 1. Template spec + generator (`src/go/cmd/gen-templates/`)

A new cmd tool mirroring `cmd/gen-chain-spec`. Holds the seven genre definitions as
declarative Go data and emits one JSON file per genre into
`internal/assets/templates/`.

Spec shape (illustrative — not final API):
```go
type templateSpec struct {
    Genre   string   // "rock"
    Display string   // "Rock"
    BPM     int
    Subdiv  int      // 16 (salsa: still 16, with 2-bar perimeter)
    Bars    int      // 1 (salsa: 2)
    Master  masterSpec // master EQ/sends/volume touches
    Rows    []rowSpec
}

type rowSpec struct {
    Instrument  string             // builtin id, e.g. "kick-tight"
    Recipe      string             // optional explicit recipe id
    SynthParams map[string]float64 // 8-knob effective-deltas
    Volume      float64
    Pan         float64
    DelaySend   float64
    ReverbSend  float64
    EQ          *eqSpec            // optional per-instrument EQ
    Hits        []hitSpec
}

type hitSpec struct {
    Step      int     // sixteenth index in the bar(s)
    Pitch     float64 // per-node semitone offset (melody)
    Duration  float64 // per-node note length (default 1.0)
    Volume    float64 // per-node dynamics (ghost/accent); 0 => row default
    LogicKind string  // "", "probability", "every_n_triggers"
    LogicN    int
    LogicP    float64
    GrooveKind string // "", "delay", "rush"
    GroovePct  float64
}
```

A pure layout function turns each `rowSpec` into nodes+edges on a per-row square loop:
- Compute the loop perimeter `P = Bars * Subdiv * 4` (units) and the per-step distance
  `d = P / (Bars*16)`.
- For each step `k` in the bar(s), map cumulative distance `k*d` to an `(i,j)` on the
  square perimeter (helper walks side-by-side around the square).
- Emit a `regular` node at every hit step (carrying its pitch/duration/volume/logic/
  groove); emit `silent` nodes at the four corners when no hit lands there (to enforce
  orthogonal turns); wire consecutive nodes in perimeter order and close the loop.
- The instrument's `origin` = the node at step 0 (or the first hit/corner at/after it).
  Row 0's origin becomes the graph `StartNodeID`.

Output is the existing export-document shape (`version, subdiv, bpm, master_volume,
instruments[], nodes[], eq, send_effects`), marshaled to JSON. The generator builds the
document structs directly — it does **not** spin up an Ebiten `Game` — so it has no GUI
dependency.

Wired into the build the same way as other generators (a `make gen-templates` target;
optionally a `//go:generate` directive). Generated JSON is committed to the repo.

### 2. Asset registry (`src/go/internal/assets/templates.go`)

```go
//go:embed templates/*.json
var templateFS embed.FS

type Template struct {
    Genre   string // "rock"
    Display string // "Rock"
    BPM     int
    Bytes   []byte
}

func Templates() []Template // ordered: rock, hip-hop, pop, funk, salsa, house, techno
```

Order and display metadata come from a small static table in this file (so the submenu
shows BPM without parsing each JSON). Embed works identically for desktop and WASM.

### 3. Menu wiring (`internal/ui/`)

- `drumview_context_menu.go` — add a `Templates` item to `overflowItems()` under the File
  section. New `IconTemplate` (or reuse `IconImport`) per the design-token glyph rules.
- The item's `onClick` opens a **second cascading submenu portal** (extend
  `drumview_portal_open.go`'s portal machinery; add an `openTemplateMenuPortal()` that
  builds entries from `assets.Templates()` — one row per genre, label = `"Rock · 120"`).
- Each submenu entry's `onClick` calls `g.Import(tmpl.Bytes)` then closes both portals
  (via the queued-import path used by the existing Import button, so it lands on the next
  `Update()` under `seqMu`).
- The submenu follows existing portal close/dismiss behaviour (click-out, escape).

No new analyzer/zone is introduced; this reuses the overflow portal that already exists.

### 4. WASM/JS surface

No new export is strictly required (loading happens Go-side via the menu). The existing
`importJSON` bridge is unaffected. The bridge-smoke catalogue is updated only if a new
top-level export is added (it is not, in the baseline plan).

### 5. Tests (all automated — no manual steps)

- **Generator round-trip** (`internal/assets/templates_roundtrip_test.go`, fast `-tags
  test`): for each template — JSON parses; `g.Import` succeeds with no error; every row's
  loop length in beats equals the spec's expected bars; each instrument fires on exactly
  the intended steps (assert against the spec's hit set via the predictor / beat path);
  every melodic row's per-node pitches survive the round-trip.
- **Registry test**: `assets.Templates()` returns 7 entries in the documented order, each
  with non-empty `Bytes` that unmarshal to `version:1`.
- **Menu test** (`internal/ui/`): opening the overflow menu lists a `Templates` entry;
  invoking it lists 7 submenu entries; selecting one replaces the circuit (row count /
  instrument ids match the template). Uses the ebitenstub harness like existing
  `import_button_flow_test.go`.
- **Drift guard**: a committed-vs-generated check (mirroring the gen-design-tokens
  pre-commit philosophy) — regenerating must produce byte-identical JSON, so the
  generator stays the source of truth.
- **Bridge smoke**: unchanged unless a new export is added.

## Data flow

```
gen-templates (Go spec) ──emit──▶ internal/assets/templates/<genre>.json  (committed)
                                            │ //go:embed
                                            ▼
                                   assets.Templates() ──▶ overflow submenu
                                            │ onClick
                                            ▼
                                   g.Import(bytes)  (replace-not-merge, queued)
                                            ▼
                              predictor / drum rows / playback
```

## Error handling

- A malformed or missing template file fails the generator drift test in CI, not at
  runtime — bad JSON never ships.
- At runtime `g.Import` already validates (instrument/node caps, coordinate clamps); a
  template that somehow fails import surfaces the same error path as a user import and
  leaves the prior circuit intact (Import is transactional today).
- `assets.Templates()` returning fewer than 7 (embed glob miss) is caught by the registry
  test.

## Out of scope (YAGNI)

- User-saved / custom templates, template thumbnails or previews, a modal gallery UI.
- New C instruments or new recipes — the seven genres are expressible with the existing
  44 builtins + knob tuning + per-node pitch.
- A confirmation dialog before replace (explicitly declined).
- Per-platform divergence — embed + Import are already platform-agnostic.

## Files touched (estimate)

New:
- `src/go/cmd/gen-templates/main.go` (+ spec data files)
- `src/go/internal/assets/templates.go`
- `src/go/internal/assets/templates/{rock,hip-hop,pop,funk,salsa,house,techno}.json`
- `src/go/internal/assets/templates_roundtrip_test.go`
- `src/go/internal/ui/template_menu_test.go`

Modified:
- `src/go/internal/ui/drumview_context_menu.go` (overflow item)
- `src/go/internal/ui/drumview_portal_open.go` (submenu portal)
- `src/go/internal/ui/icons.go` (optional `IconTemplate`)
- `Makefile` (`gen-templates` target)
