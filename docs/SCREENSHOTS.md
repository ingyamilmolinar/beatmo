# Screenshot capture

Beatmo has two screenshot targets:

| Target | Captures | Output |
|--------|----------|--------|
| `make screenshot` | One desktop + two browser layouts (legacy) | `screenshots/desktop.png`, `screenshots/browser_*.png` |
| `make screenshots-all` | Every UI surface in the scene catalog | `screenshots/all/{desktop,mobile}/<scene>.png` + `manifest.json` |

`make screenshots-all` orchestrates the desktop binary with `-scene` and a Playwright browser pass with `runScene(name)`. Both ultimately drive the same Go scene catalog (`src/go/internal/ui/scene_catalog.go`), so adding a scene once captures it on both backends.

## Quick start

```bash
make screenshots-all
MOBILE=1 make screenshots-all
SCENES=context_menu_open,fx_panel_with_3_effects make screenshots-all
```

## How a scene runs

1. CLI flag `-scene <name>` (desktop) or `runScene(name)` JS export (browser) looks up the scene in `sceneCatalog`.
2. The scene's `Setup(*Game)` runs synchronously. It can call any of the public openers/setters added in Phase 1 of the screenshot harness:
   - menu/dialog openers — `dv.OpenContextMenu`, `dv.OpenFXPanel`, `dv.OpenColorMenu`, `dv.OpenInstrumentMenu`, `dv.OpenSubdivMenu`, `dv.OpenMasterVolumePopup`, `dv.OpenVolumePopup`, `dv.OpenOverflowMenu`
   - audio setters — `g.SetMasterVolume`, `g.SetEQHPF`, `g.SetEQLPF`, `g.SetEQBandGain`, `g.SetActiveEQTab`
   - state setters — `g.SetForceMobileProfile`, `g.SetForceAutoSize`, `g.SetDefaultStart`, `g.SetCameraScale`, `g.SetSplitterFrac`, `g.SetViewMode`
   - graph mutations — `g.tryAddNode`, `g.addEdge`, `g.sidebar.Open`
   - recording — `g.StartRecording`, `g.StopRecording`
3. The screenshot loop waits `SettleFrames` (default 90) Update ticks, then captures.

JS-driven scene calls are queued via `Game.QueueAction` and drained by `Game.Update` after `seqMu` is released. This mirrors the existing `pendingImportData` deferred-queue pattern (`game_update.go:466`), avoiding the reentrant-mutex deadlock the original capture-all design would have hit.

## Adding a scene

Edit `src/go/internal/ui/scene_catalog.go` and append an entry:

```go
{Name: "my_new_scene", Description: "what it shows",
    SettleFrames: 120, // optional; default 90
    Mobile:       true, // optional; capture on mobile too
    Setup: func(g *Game) {
        ensureRow(g, 0)
        g.drum.OpenFXPanel(0)
    }},
```

Names are kebab-case. Scenes prefixed `mobile_` only run on the mobile pass.

## `-ui-state` config

For state outside `tunkul.json` (camera, splitter, profile, view mode, sidebar), use `-ui-state path/to/state.json`:

```json
{
  "profile": "mobile",
  "force_auto_size": true,
  "camera": { "scale": 1.6, "center": true },
  "splitter": { "frac": 0.65 },
  "view": { "mode": "audio", "mobile_eq_collapsed": false },
  "sidebar": { "open_node_id": 3 }
}
```

For circuits, instruments, EQ band gains, transport BPM/subdiv/length, master volume, and send effects — those live in `tunkul.json`; load via `-import path/to/project.json`. The boot order is import → ui-state → scene → screenshot countdown.

## Idempotency

Captures are **perceptually stable**, not byte-identical across machines. Different fonts, xvfb backends (llvmpipe vs SwiftShader), and Go/Ebiten versions all change pixels at the byte level. If you compare PNGs across runs, use a tolerance-aware diff (e.g. pixelmatch with 0.5%) like `make test-visual` already does.

## Mobile profile

A mobile capture needs **both**: Playwright's `isMobile: true, hasTouch: true` viewport AND `g.SetForceMobileProfile(true)` (or a scene tagged `mobile_*`). Playwright sets only the browser-level signals; the Go layout reads its own `forceSmallScreenForTest` flag. Both must agree.

## CI cost

Roughly 4–6 minutes per `make screenshots-all` (desktop binary boot + 90-frame settle × ~25 scenes, plus browser pass). Run on demand, not on every PR.

## Cross-references

- Scene catalog: `src/go/internal/ui/scene_catalog.go`
- Per-method JS exports: `src/go/internal/ui/js_exports_scenes.go`
- Action queue (seqMu safety): `src/go/internal/ui/game_pending_actions.go`
- ui-state schema: `src/go/internal/ui/uistate/uistate.go`
- Orchestrator: `scripts/capture_all_ui.sh` + `scripts/capture_browser_runner.mjs`
