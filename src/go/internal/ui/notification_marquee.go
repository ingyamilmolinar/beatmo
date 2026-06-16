package ui

const (
	// notifMarqueeHoldFrames is the pause (in frames) held at each end of a
	// marquee cycle so the reader can catch the start and the tail.
	notifMarqueeHoldFrames = 90 // ~1.5s at 60fps
	// notifMarqueePxPeriod advances the scroll by 1px every N frames — the
	// smaller it is, the faster the crawl. 3 → ~20px/s at 60fps (slow).
	notifMarqueePxPeriod = 3
)

// notifMarqueeOffsetX returns the x pixel offset (<= 0) to translate the
// notification text so a too-wide message scrolls slowly leftward, pausing at
// both ends, then looping. Returns 0 when the text fits within areaW.
//
//	travel = textW - areaW + gap   (extra width that doesn't fit, plus a tail gap)
//	cycle  = hold | scroll | hold  → repeat
func notifMarqueeOffsetX(frame, textW, areaW, gap int) int {
	travel := textW - areaW + gap
	if travel <= 0 {
		return 0
	}
	scrollFrames := travel * notifMarqueePxPeriod
	period := notifMarqueeHoldFrames + scrollFrames + notifMarqueeHoldFrames
	p := ((frame % period) + period) % period
	switch {
	case p < notifMarqueeHoldFrames:
		// Hold at the start so the first characters are readable.
		return 0
	case p < notifMarqueeHoldFrames+scrollFrames:
		// Crawl left at a constant slow rate.
		return -((p - notifMarqueeHoldFrames) / notifMarqueePxPeriod)
	default:
		// Hold at the end so the tail is readable before looping.
		return -travel
	}
}
