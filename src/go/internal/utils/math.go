package utils

import "image"

func Abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func CalculateIntermediateGridPoints(node1I, node1J, node2I, node2J int) []image.Point {
	var points []image.Point

	if node1I == node2I { // Vertical line
		step := 1
		if node1J > node2J {
			step = -1
		}
		for j := node1J + step; j != node2J; j += step {
			points = append(points, image.Pt(node1I, j))
		}
	} else if node1J == node2J { // Horizontal line
		step := 1
		if node1I > node2I {
			step = -1
		}
		for i := node1I + step; i != node2I; i += step {
			points = append(points, image.Pt(i, node1J))
		}
	}
	return points
}
