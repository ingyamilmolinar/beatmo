package ui

import (
	"sort"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/hajimehoshi/ebiten/v2"
)

// Image-allocation counters used to measure how much pressure beatmo puts on
// Ebiten's restorable image-dependency graph. Two signals are exposed:
//
//   - imagesAllocatedTotal: cumulative *ebiten.Image allocations through the
//     wrapper. Use ResetImageMetrics at a known boundary (bench start) and
//     read again at bench end to get "images allocated during the window."
//   - drawCallsTotal: optional counter callers may bump from hot-path draw
//     helpers. Today only the rows-layer composite path bumps it; treat it
//     as "rows-layer DrawImage calls" rather than process-wide draw count.
//
// Live image count is computed on demand by snapshotLiveImages — see that
// function for the inventory it consults.

var (
	imagesAllocatedTotal int64
	drawCallsTotal       int64

	imagesByTagMu sync.Mutex
	imagesByTag   = map[string]int64{}
)

// newTrackedImage wraps ebiten.NewImage and bumps the cumulative allocation
// counter. Use at hot-path allocation sites (sprite/layer caches) where the
// allocation rate is sensitive to circuit complexity. One-shot init
// allocations (icon sheets, fonts) intentionally skip the counter — they are
// constant and would only add noise.
//
// The tag identifies the call site. Use a stable short string (e.g.
// "rowsLayer", "rowSprite") so per-tag counters can be diffed across builds.
func newTrackedImage(tag string, w, h int) *ebiten.Image {
	atomic.AddInt64(&imagesAllocatedTotal, 1)
	imagesByTagMu.Lock()
	imagesByTag[tag]++
	imagesByTagMu.Unlock()
	return ebiten.NewImage(w, h)
}

// releaseImage releases the GPU atlas slot held by img. Use before
// overwriting a cache field with a freshly-allocated image so the old
// atlas slot is returned to Ebiten's packer immediately. Without this,
// orphaned images sit in the BSP packing tree until GC finalizers fire
// — on WASM that's bursty and unreliable, and a per-frame reassignment
// pattern (e.g. timeline_zone.go's TlCache before the fix) can leak
// hundreds of MB into the atlas before the heap exhausts.
//
// Safe to call on nil. After release the *ebiten.Image object is still
// valid (ebiten allocates a fresh internal slot on next use) — callers
// nil the field for clarity.
func releaseImage(img *ebiten.Image) {
	if img == nil {
		return
	}
	img.Deallocate()
}

// imagesByTagSnapshot returns a copy of the per-tag allocation counters
// formatted as "tag:count|tag:count|..." sorted by tag for stable output.
func imagesByTagSnapshot() string {
	imagesByTagMu.Lock()
	defer imagesByTagMu.Unlock()
	tags := make([]string, 0, len(imagesByTag))
	for k := range imagesByTag {
		tags = append(tags, k)
	}
	sort.Strings(tags)
	out := ""
	for _, t := range tags {
		if out != "" {
			out += "|"
		}
		out += t
		out += ":"
		out += strconv.FormatInt(imagesByTag[t], 10)
	}
	return out
}

// bumpDrawCall increments the cumulative draw-call counter. Callers in hot
// loops that DrawImage row sprites or stripe sprites should call this once
// per call. Coverage is intentionally partial — see file comment.
func bumpDrawCall() { atomic.AddInt64(&drawCallsTotal, 1) }

// MetricImagesAllocatedTotal returns the cumulative number of *ebiten.Image
// instances created through newTrackedImage since the last ResetImageMetrics.
func MetricImagesAllocatedTotal() int64 {
	return atomic.LoadInt64(&imagesAllocatedTotal)
}

// MetricDrawCallsTotal returns the cumulative number of bumpDrawCall calls.
func MetricDrawCallsTotal() int64 { return atomic.LoadInt64(&drawCallsTotal) }

// ResetImageMetrics zeroes the cumulative counters. Called at bench start and
// once during the bench duration window to scope the numbers to playback.
func ResetImageMetrics() {
	atomic.StoreInt64(&imagesAllocatedTotal, 0)
	atomic.StoreInt64(&drawCallsTotal, 0)
	imagesByTagMu.Lock()
	for k := range imagesByTag {
		delete(imagesByTag, k)
	}
	imagesByTagMu.Unlock()
}

// snapshotLiveImages walks the known long-lived image caches and returns a
// best-effort count of non-nil *ebiten.Image references the application
// holds. This is a proxy for the size of Ebiten's restorable.images map and
// scales with circuit complexity (rows, stripes) plus persistent UI sprites.
//
// The count is NOT a count of all images Ebiten tracks — it deliberately
// ignores one-shot init allocations (sprites, icon sheets) which are constant
// across the whole process lifetime. The intent is to measure the *delta*
// caused by gameplay state.
func (g *Game) snapshotLiveImages() int {
	if g == nil {
		return 0
	}
	n := 0
	if g.gridCache != nil {
		n++
	}
	if g.edgeCache != nil {
		n++
	}
	if g.nodeLayer != nil {
		n++
	}
	n += len(g.nodeSpriteCache)
	if g.drum != nil {
		dv := g.drum
		for _, im := range dv.rowCache {
			if im != nil {
				n++
			}
		}
		for _, im := range dv.rowCacheScratch {
			if im != nil {
				n++
			}
		}
		for _, im := range dv.bgCache {
			if im != nil {
				n++
			}
		}
		if dv.rowsLayer != nil {
			n++
		}
		if dv.rowsLayerScratch != nil {
			n++
		}
		if dv.colorWheelImg != nil {
			n++
		}
		if dv.toolbarCache != nil {
			n++
		}
	}
	return n
}
