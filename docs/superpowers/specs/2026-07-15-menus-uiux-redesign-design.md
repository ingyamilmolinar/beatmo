# Menus UI/UX Redesign — Instrument Picker · FX Panel · Templates

Date: 2026-07-15. Scope: the three list-menu surfaces (instrument picker,
per-row insert-FX panel, overflow template page) — visual consistency,
user-flow clarity, and the design-system (DESIGN.md) vocabulary they need.
Baseline screenshots: `screenshots/all/desktop/{instrument_menu_open,
instrument_menu_categories,fx_panel_open_empty,fx_panel_with_3_effects,
template_menu_open}.png` (2026-07-15 capture).

## Part 1 — Ruthless critique

### Cross-cutting (all three surfaces)

1. **The search field is dead chrome.** A bare rectangle with a muted
   "Search" hint. No leading magnifier (the single most recognized search
   affordance in existence), no clear-× once a query is typed (the only way
   to clear is select-all-delete or Esc — invisible), no result feedback.
   Both menus share `menu_search_field.go`, so the fix lands everywhere at
   once. The icon set doesn't even have a magnifier — telling.
2. **Menu titles are three different dialects.** DESIGN.md declares "every
   menu TITLE is unified (one shared drawMenuTitle)… + a leading instrument
   swatch when instrument-scoped." Reality: the instrument menu renders a
   breadcrumb strip instead, the template page renders `drawMenuTitle`
   correctly, and the FX panel — the most instrument-scoped surface in the
   app — draws a raw `fmt.Sprintf("FX: %s")` with **no swatch**, no unified
   role, and a byte-slice truncation (`title[:30]`) that can split a UTF-8
   rune.
3. **Rows waste their metadata.** `assets.Template` carries BPM and a
   curated grouping; instrument rows carry an 8-px color dot that is
   supposed to be "the only identity signal" per the design philosophy but
   is barely a speck at arm's length. `MenuRowSpec.TrailLabel` exists and is
   used by exactly one surface (global-search category trail).
4. **Selection state is inconsistent.** The instrument menu shows an
   active stripe + tint; the template menu never passes `menuItemActive`
   — no indication of what's currently loaded; the FX panel has no notion
   of row states at all (its cards blur into one pink wall).
5. **Empty states are an afterthought.** FX empty = a lone "+ Add Effect";
   inst menu "No matches" / "No favorites yet…" are hardcoded English in an
   otherwise i18n'd app.

### Instrument picker

- **The color dot is 8 px** (`instMenuSwatchSz = 8`) while the title swatch
  is 10 px — two sizes for the same concept, both too small for the
  "color carries identity" gold standard the design doc brags about.
- **Category rows are bare text.** No count ("Kick — how many?"), no
  drill-in affordance (global-search category rows DO get a chevron —
  inconsistent), and the Favorites pseudo-category shows a count but no
  star. A first-time user cannot tell categories are folders.
- **Audition behavior is invisible.** Clicking a row plays + selects and
  keeps the menu open (great flow!) — but nothing tells the user this.
  The selected row's stripe is the only feedback.
- **Dead chrome in code**: a full pagination API + `MenuStyle` tokens
  (`PaginationStripH`, `ChipMinW`, `JumpInputW`) that no draw path renders;
  stale duplicate constants in drumview.go.
- **Favorite star column** is functional but visually mute: outline stars
  in `on-surface-muted`; hover doesn't preview the star filling.

### FX panel

- **A wall of same-color rows.** Effect headers, param labels, and sliders
  all run together with no card separation (see 3-effects screenshot: it
  reads as one 9-row list, not 3 effects × 3 params). Disabled effects only
  parenthesize the name — "(Reverb)" — instead of dimming the card.
- **Cramped, dangerous right cluster.** Reorder ↑↓ and destructive ✕ sit
  3 px apart at 24 px; the code even documents a historical
  overlap-with-scrollbar bug. Chevron glyphs mean "reorder" here and
  "expand" one row over (mobile) — same glyph, two meanings.
- **Two number dialects.** Expanded label says `Drive: 8.19`; collapsed
  summary says `Drive 8.2` — the value visibly *changes* when you expand.
  Mobile truncates labels to "Warmth: 0…." (baseline screenshot).
- **Add Effect flow is a style orphan**: inline rows in `InstButtonStyle`
  with a `DisabledButtonStyle` Cancel (a *disabled-looking* button that is
  clickable!), instead of the shared flat menu-row language.
- **Functional gaps:** Add and Remove record **no undo step** while
  toggle/reorder/param do; the header double-draw + magic numbers
  (`lineH/headerH/paramH` redeclared in build AND draw) are drift bombs.

### Template page

- **31 masterpieces in an anonymous flat list.** The data model has
  curated groups (Classical / Bossa·Jazz·Blues / Funk·Disco / …) that exist
  only as comments in `templateOrder`. No genre headers, no BPM caption —
  just the same `IconTemplate` stamped 31 times, which makes the icon
  column pure indentation noise.
- **No "currently loaded" state** — pick Vivaldi, reopen the menu, and
  nothing marks it.
- **Wrong accent plumbing**: the search band underline borrows
  `contextMenuAccent()` — the accent of whatever row's kebab menu was last
  targeted — for a global menu. Semantically random color.
- **Back is a scrolling row**, so on a scrolled list the way out drifts
  off-screen while header + search stay pinned.

## Part 2 — Design decisions

Approaches considered: (A) minimal polish inside current vocabulary,
(B) per-surface bespoke redesigns, (C) **expand the shared menu vocabulary
in DESIGN.md and re-express all three surfaces in it** — chosen: it fixes
the inconsistency at the root, keeps the minimalist-flat doctrine, and every
future menu inherits the wins.

### Design-system additions (DESIGN.md)

- **`IconSearch`** (magnifier) joins the icon set; Icon Map gains a row.
- **Swatch scale**: `spacing.swatch-sm: 10` (title swatch),
  `spacing.swatch-md: 12` (list-row swatch) — one token pair replaces the
  8/10 split; row swatches grow to 12 px with the existing faint halo.
- **Menu search field anatomy** (new prose + `menu-search-field`
  component): leading `IconSearch` in `on-surface-muted`, query text,
  trailing clear-× (`IconClose`, `on-surface-muted` → `on-surface` hover)
  rendered only when the query is non-empty; focus keeps the hot-pink
  focus ring. One shared widget; byte-identical across menus.
- **Menu group headers** (new prose + `menu-group-header` component): a
  non-interactive, scrolling row at `caption` role, uppercase, muted, with
  a leading hairline — for template genres and any future grouped list.
- **Metadata captions**: `MenuRowSpec.TrailLabel` is promoted to the
  standard "secondary fact" slot (category for instruments, BPM for
  templates, param digest for collapsed FX cards).
- **FX effect card** (new prose + `fx-card` component): each insert effect
  is a `surface-2` card with a header band tinted by the instrument color
  at `alpha.faint`, `rounded.sm`, hairline border, `spacing.sm` gap between
  cards; disabled card dims content to `alpha.subtle` (same scrim idea as
  synth stages). Reorder/remove separate: remove ✕ gets `spacing.md`
  clearance from the chevrons.
- **Unified title rule enforcement**: FX panel adopts `drawMenuTitle` with
  the instrument swatch.
- **Empty-state pattern**: centered icon + `body` line + `caption` line,
  both i18n'd.

### Per-surface changes

**Shared**: search field upgrade (icon + clear + shared layout fn);
swatch tokens; group-header renderer; trailing-icon slot in `MenuRowSpec`
(`TrailIconID`) for drill-in chevrons.

**Instrument picker**: 12-px swatches; category rows gain instrument-count
trail captions + trailing chevron; Favorites row gains a gold filled-star
leading icon; empty states i18n'd (`menu.no-matches`, `menu.no-favorites`)
and centered; dead pagination chrome + stale constants deleted.

**FX panel**: card-per-effect visual grouping with instrument-tinted
header band; unified title (swatch + section-header role, rune-safe
truncation); one value formatter for label and summary; Add Effect list
re-expressed as flat menu rows (leading `IconPlus`, hover accent, Cancel
as a normal muted row with `IconClose`); undo steps for add/remove;
`fxPanelMetrics()` extracted so build & draw share one geometry source.

**Template page**: genre groups become real (`Template.Group` in assets,
groups drive headers when the query is empty; search results stay flat,
score-sorted); rows gain `♩ BPM` trail captions; the currently-loaded
template renders `menuItemActive`; search underline accent fixed to the
neutral gold `colAccent`; Back joins the pinned band so the exit never
scrolls away.

### Testing

Existing pin tests updated where they encode old geometry/strings; new
fast-path tests: search-field clear button (shared), template group
headers + BPM captions + active row, FX card undo add/remove, FX value
format parity, category count captions. `TestDesignMDDrift` want-maps and
`token_discipline` budgets updated with the new tokens. Screenshots
re-captured for the five scenes + compared by eye.

### Non-goals

Audition/preview redesign (flow already good), mobile knob-vs-slider
switch, template previews/thumbnails, favorites reordering, menu keycap
restyle (flat doctrine stays), any playback-engine change.

---

## Part 3 — Amendment (2026-07-16): post-implementation audit

Re-audited against the **production** build (`/tmp/beatmo_screenshot -scene …`,
not `-tags test`) after Part 2 landed. Parts 1–2 are substantially **done** and
verified rendering: shared search anatomy (magnifier + clear-×), category
count captions + drill-in chevrons + gold Favorites star, template genre
group headers + `BPM` trails + `menuItemActive` on the loaded row, FX cards
with instrument-tinted header bands, `fxPanelMetrics()` as the single
geometry source, rune-safe title (`clipTextToWidthMeasure`), the FX empty
state ("No effects yet — shape this sound below"), and `fxParamLabel`
delegating to `fxFormatVal` so label and summary can no longer disagree.

The items below are **not covered by Parts 1–2** and remain open.

### 3.1 Value formatting is chosen by magnitude, not by parameter (open)

Part 2 unified *where* values are formatted (one `fxFormatVal`); it did not
change *how*. `fxFormatVal` still branches on the value's magnitude:

```go
case val >= 1000: "%.0f"   // Tone: 4000 Hz  — should read 4.0 kHz
case val >= 10:   "%.1f"   // Time: 250.0 ms — fake decimal
case val == int:  "%.0f"   // Mix: 1
default:          "%.2f"   // Mix: 0.30
```

Consequences, all live in the production build: the **same** param renders
`Mix: 1` and `Mix: 0.30`, so **the digit count changes while the user drags**
(the label jitters and values never column-align); `Tone: 4000 Hz` never rolls
up to `4.0 kHz`; `Time: 250.0 ms` carries a decimal it cannot use.

`audio.EffectParamDef` already carries `Min`, `Max`, and `Unit`
(`internal/audio/insert_effects.go`) — every input needed to decide precision
**once per parameter**. Nothing reads them for formatting.

**Decision:** precision and unit rendering are a property of the *parameter
definition*, never of the current value. Derive decimals from the param's
range (`Max-Min`) and unit; roll `Hz ≥ 1000` to `kHz`; render a fixed decimal
count per param so digits never shift mid-drag. `fxFormatVal(val)` becomes
`fxFormatParam(def, val)`. This is a **DESIGN.md rule**, not an FX-local fix
(see 3.6) — the FX bug survived precisely because there was nothing to drift
from.

### 3.2 Enum params render as bare integers on continuous sliders (open)

Filter `mode` (0–2), Waveshaper `curve` (0–3), Tremolo/RingMod `shape`,
Phaser `stages`, Bitcrusher `bits` are discrete, but render as continuous
sliders labelled `Mode: 0`. That is not a control, it is a riddle.

**Decision:** `EffectParamDef` gains an optional `Enum []string`. Params with
`Enum` render as a labeled segmented control / stepper showing the **name**
("Low-pass"), never the index. Covered by the DESIGN.md rule in 3.6.

### 3.3 Desktop cannot collapse an effect (open)

`fxShowParams` returns `true` unconditionally when `!Profile().UseBottomSheet`
— the accordion is mobile-only. Compressor has 6 params and Tape 5, so a
3-effect chain already overflows the panel (this is the real cause of the
"wall of sliders", not the card styling). `fxExpandedSlots` already exists and
is already honoured on mobile.

**Decision:** honour `fxExpandedSlots` on desktop too; collapsed cards show
the existing `fxParamSummary` digest. Default the **first** card expanded and
the rest collapsed so the panel opens legible at any chain length.

### 3.4 Template load silently destroys the user's circuit (open — data loss)

`import.go` calls `undoManager.OnExternalLoad()`, which **clears undo/redo**
("you cannot undo across a document swap"). Loading a template is one tap on
a 44 px row, with no preview and no confirmation, and the current circuit is
then unrecoverable. This is the highest-severity issue in the menus.

**Decision:** make the swap undoable. `UndoManager` is a full-document
snapshot journal, and its `committed` baseline already holds the pre-import
document, so the template path calls `commitNow("Load template")` instead of
`OnExternalLoad()` — this pushes the pre-import doc as one undo step.

**Cost is neutral and the known perf trap does not reopen:** `OnExternalLoad`
already performs exactly one `m.capture()`; so does `commitNow`. The O(N²)
undo-capture storm (7.0s → 0.05s fix, see `import_template_hang_test.go`) is
prevented by the existing `if g.importing { return nil }` guard in
`undoCapture`, and `import.go`'s LIFO defer order (`OnExternalLoad` registered
before `importing=false`) already guarantees the single post-import capture
serializes correctly. The template row already defers through
`pendingImportData`, so the `seqMu` re-entrancy rule is respected.

Scope: the in-app template path. A user file-import may follow the same
argument but is out of scope here.

### 3.5 The instrument picker is a 33k-item browser in a 4-row window (open)

**The largest UX problem in these menus, and Parts 1–2 do not address it.**
The production catalog is not 91 instruments — `InitCatalogFromDir` walks the
assets tree and finds **33,330 WAV files** (Kick ≈ 7.9k, Snare ≈ 8k, Hi-Hat
≈ 6.7k). `-tags test` swaps in a stub catalog with 91, so this is invisible on
the fast path (see [[project_prod_instrument_catalog_24k]]).

Meanwhile the picker anchors to `vertBounds = dv.widgetRects[WidgetRack]` (the
short row-rack rect) and shows **4 rows** in the default desktop layout, and
`instMenuMaxVisibleRowsComp = 10` is hardcoded — mobile adapts to the viewport,
desktop does not. Browsing 7,898 kicks four at a time is ~1,975 scroll-pages.
DESIGN.md menu-anatomy **slot 3 (group headers)** is used by the template page
but **not** in instruments mode, so those 7,898 kicks are one flat run.

**Conflict with Part 2:** Part 2 deletes the "dead pagination chrome". That
API exists *for this list*. Do not delete it before the drill-down below
lands; deleting it removes the only coarse navigation the big list has.

**Decision:** the assets tree already encodes the missing axis —
`assets/<Category>/<Pack>/<Producer>/<file>.wav`, surfaced as
`SoundMeta.RelPath` (e.g. `Kick/Kick/Kane Beatz/Kick (1)`). Measured shape:
Kick → `808 Kick` (104) · `909 Kick` (2) · `Kick` (7,252) → **111 producers**
→ ~65 samples each. Every level is browsable.

Extend the breadcrumb to N levels — `Categories › Kick › Kick › Kane Beatz` —
reusing the machinery Part 2 already built (`BreadcrumbPath()`,
`popBreadcrumbTo`, `menuCatMeta` counts, drill-in chevrons, fuzzy search).
Search stays global, flat and score-sorted with headers hidden, per the
anatomy. Additionally, anchor the picker to the drum pane's height rather than
the row-rack rect so desktop stops rendering a 4-row porthole.

**Validation constraint:** every claim here must be checked against a
production screenshot, never the `-tags test` path (250× list-size difference).

### 3.6 DESIGN.md gaps — the reason 3.1/3.2 were invisible (open)

DESIGN.md governs hue exceptionally (three reds/three roles, the two-accent
invariant, a ratchet test) and, since Part 2, menu **anatomy**. It has no rule
for anything numeric or scalar, which is exactly where the surviving bugs live.

Add:

1. **§Value formatting** — precision/unit derive from the parameter's
   declared range and unit, never from the instantaneous value; fixed decimals
   per param (digits must never shift while dragging); `Hz ≥ 1000` → `kHz`.
2. **§Enum params** — a discrete param renders its **name**, never its index;
   segmented control / stepper, never a continuous slider.
3. **§List scale tiers** — `< 20` flat; `20–200` group headers + search;
   `> 200` **requires** drill-down + counts + a viewport that uses available
   height. (The 33k rule; makes 3.5 a system rule, not a one-off.)

**Guards** (Part 2 shipped anatomy as prose with no enforcement, which is how
the FX panel violated three slots silently):

- `menu_anatomy_discipline_test.go` — enumerate every list menu × the 5
  anatomy slots. Known deviations get an explicit allowlist entry with a
  rationale, and the allowlist **ratchets** (mirrors `allowedLiteralBudget`
  in `token_discipline_test.go`: below-budget must also fail).
- A formatting guard asserting precision is a pure function of the param
  definition — table-driven over `InsertEffectCatalog()`.

### 3.7 `+ Add Effect` ships a raw `+` glyph (open — spec violation)

`AddButton()`'s contract reads *"never the raw `+` character"*, and DESIGN.md
icon rule #1 lists `+` explicitly among glyphs that must use an `IconID`. Yet
`KeyAddEffect: "+ Add Effect"` (`catalog_en.go:47`) and
`KeyAddEffect: "+ Añadir efecto"` (`catalog_es.go:47`) both embed a literal
`+` rendered as text.

**Decision:** drop the `+` from both catalog strings; render a leading
`IconPlus` on the row (Part 2 already specified "leading `IconPlus`" — the
i18n strings were missed).

### 3.8 Notes for whoever picks this up

- **The `-tags test` catalog lies about scale** (91 vs 33,415). Validate any
  menu-scale change with `make screenshots-all SCENES=instrument_menu_*`.
- `template_menu_open` scene added to `scene_catalog.go` (the template page
  had no scene; it is `overflowPage = 1`).
- Mid-list group headers depend on `overflowPinnedCount`'s **leading-run**
  invariant: the pinned band is the leading run of header/search rows only.
  `overflowPopupBtns` and `drawOverflowMenu` must keep advancing one row per
  item while skipping headers in the button mapping. N-level drill-down must
  not break this.
- `fx_panel_knob_drawer_open` is a **misnomer**: there is no knob mode in the
  FX panel: the scene forces mobile + `fxExpandedSlots[0]` and still builds
  sliders. Rename or implement.
