package ui

import (
	"sort"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/hajimehoshi/ebiten/v2"
)

// Measures pressure on Ebiten's restorable image graph. drawCallsTotal is bumped only on the rows-layer path, so it
// counts that, not all draws.

var (
	imagesAllocatedTotal int64
	drawCallsTotal       int64
	// Naive tiling is quadratic as the camera zooms out (tens of thousands of blits a frame, starving the WASM
	// sequencer); this makes the cost a testable number.
	gridTileBlitsTotal int64

	imagesByTagMu sync.Mutex
	imagesByTag   = map[string]int64{}
)

// For hot-path cache allocations; constant one-shot inits skip the counter as noise. tag is a stable short name so
// per-tag counts diff across builds.
func newTrackedImage(tag string, w, h int) *ebiten.Image {
	atomic.AddInt64(&imagesAllocatedTotal, 1)
	imagesByTagMu.Lock()
	imagesByTag[tag]++
	imagesByTagMu.Unlock()
	return ebiten.NewImage(w, h)
}

// Returns the atlas slot at once instead of waiting for GC finalizers, which on WASM are bursty enough for a
// per-frame reassignment to leak hundreds of MB. Safe on nil; the image stays usable afterwards.
func releaseImage(img *ebiten.Image) {
	if img == nil {
		return
	}
	img.Deallocate()
}

// Sorted by tag for stable output.
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

// Coverage is partial on purpose.
func bumpDrawCall() { atomic.AddInt64(&drawCallsTotal, 1) }

func MetricImagesAllocatedTotal() int64 {
	return atomic.LoadInt64(&imagesAllocatedTotal)
}

// MetricDrawCallsTotal returns the cumulative number of bumpDrawCall calls.
func MetricDrawCallsTotal() int64 { return atomic.LoadInt64(&drawCallsTotal) }

func bumpGridTileBlit() { atomic.AddInt64(&gridTileBlitsTotal, 1) }

func MetricGridTileBlits() int64 { return atomic.LoadInt64(&gridTileBlitsTotal) }

func ResetImageMetrics() {
	atomic.StoreInt64(&imagesAllocatedTotal, 0)
	atomic.StoreInt64(&drawCallsTotal, 0)
	atomic.StoreInt64(&gridTileBlitsTotal, 0)
	imagesByTagMu.Lock()
	for k := range imagesByTag {
		delete(imagesByTag, k)
	}
	imagesByTagMu.Unlock()
}

// A proxy for Ebiten's restorable image count: it ignores constant one-shot allocations to measure the delta
// gameplay state causes.
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
		if dv.toolbarCache != nil {
			n++
		}
	}
	return n
}
