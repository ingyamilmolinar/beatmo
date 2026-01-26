package ui

import "image"

func splitRectHoriz(r image.Rectangle) (image.Rectangle, image.Rectangle) {
	mid := r.Min.X + r.Dx()/2
	if mid < r.Min.X {
		mid = r.Min.X
	}
	if mid > r.Max.X {
		mid = r.Max.X
	}
	left := image.Rect(r.Min.X, r.Min.Y, mid, r.Max.Y)
	right := image.Rect(mid, r.Min.Y, r.Max.X, r.Max.Y)
	return left, right
}

func insetRectSafe(r image.Rectangle, pad int) image.Rectangle {
	if r.Empty() {
		return r
	}
	if pad <= 0 {
		return r
	}
	maxPadX := (r.Dx() - 1) / 2
	maxPadY := (r.Dy() - 1) / 2
	if maxPadX < 0 || maxPadY < 0 {
		return r
	}
	if pad > maxPadX {
		pad = maxPadX
	}
	if pad > maxPadY {
		pad = maxPadY
	}
	return image.Rect(r.Min.X+pad, r.Min.Y+pad, r.Max.X-pad, r.Max.Y-pad)
}
