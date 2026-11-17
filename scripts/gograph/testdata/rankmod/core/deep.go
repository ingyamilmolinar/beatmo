package core

// Deep is deliberately deep and branchy: a split-function candidate with a
// flatten note.
func Deep(xs []int, k int) int {
	total := 0
	for _, x := range xs {
		if x > 0 {
			for i := 0; i < k; i++ {
				if i%2 == 0 {
					switch {
					case x > 10 && i > 2:
						if x%3 == 0 || x%5 == 0 {
							total += x
						} else if x%7 == 0 {
							total -= x
						} else {
							total++
						}
					case x > 5:
						for j := 0; j < x; j++ {
							if j == i {
								total += j
							}
						}
					default:
						total--
					}
				} else if i%3 == 0 {
					total *= 2
				}
			}
		} else if x < -5 && k > 1 || x == -1 {
			total = -total
		}
	}
	return total
}
