package ui

import "math"

// Rounds correctly for negative values too.
func ToSub(g *Grid, beats float64) int { return int(math.Round(beats * float64(g.MaxDiv()))) }

// PointBeats converts (bx,by) beat coordinates into subdivision-space coords.
func PointBeats(g *Grid, bx, by float64) [2]int { return [2]int{ToSub(g, bx), ToSub(g, by)} }

// PointIJ returns a pair in subdivision-space from JSON export coordinates.
func PointIJ(i, j int) [2]int { return [2]int{i, j} }
