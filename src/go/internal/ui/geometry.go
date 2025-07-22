package ui

import "image"

// pt is a helper function to check if a point is within a rectangle.
func pt(x, y int, r image.Rectangle) bool {
	return x >= r.Min.X && x < r.Max.X && y >= r.Min.Y && y < r.Max.Y
}