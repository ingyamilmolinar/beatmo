// Command report is dev tooling: its complexity counts, at a discount.
package main

import "example.com/rankmod/core"

func main() { _ = core.Deep([]int{3}, 1) + branchy([]int{1, 2}, 3) }

func branchy(xs []int, k int) int {
	t := 0
	for _, x := range xs {
		if x > 0 {
			for i := 0; i < k; i++ {
				if i%2 == 0 {
					switch {
					case x > 10 && i > 2:
						if x%3 == 0 || x%5 == 0 {
							t += x
						} else if x%7 == 0 {
							t -= x
						} else {
							t++
						}
					case x > 5:
						for j := 0; j < x; j++ {
							if j == i {
								t += j
							}
						}
					default:
						t--
					}
				} else if i%3 == 0 {
					t *= 2
				}
			}
		} else if x < -5 && k > 1 || x == -1 {
			t = -t
		}
	}
	return t
}
