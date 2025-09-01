package utils

import "image"

// GCD returns the greatest common divisor of a and b.
func GCD(a, b int) int {
        for b != 0 {
                a, b = b, a%b
        }
        if a < 0 {
                return -a
        }
        return a
}

// Clamp01 clamps v to the [0,1] range.
func Clamp01(v float64) float64 {
        if v < 0 {
                return 0
        }
        if v > 1 {
                return 1
        }
        return v
}

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
