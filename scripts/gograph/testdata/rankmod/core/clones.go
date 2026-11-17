package core

// Clones calls everything below so it is all reachable.
func Clones() {
	_ = cloneA([]int{1})
	_ = cloneB([]int{1})
	_ = stringA(1) + stringB(1)
	_ = w1(1) + w2(1) + w3(1) + w4(1) + w5(1)
	longA()
	longB()
	_ = toggleSettings() || toggleLogs()
	aliases()
}

// cloneA and cloneB are copies with renamed locals: one exact clone group.
func cloneA(xs []int) int {
	sum := 0
	for _, v := range xs {
		if v > 10 {
			sum += v * 2
		} else {
			sum += v
		}
	}
	if sum > 100 {
		return 100
	}
	return sum
}

func cloneB(items []int) int {
	acc := 0
	for _, it := range items {
		if it > 10 {
			acc += it * 2
		} else {
			acc += it
		}
	}
	if acc > 100 {
		return 100
	}
	return acc
}

// stringA and stringB share a shape but hold different data: structural only.
func stringA(n int) string {
	switch n {
	case 1:
		return "one"
	case 2:
		return "two"
	case 3:
		return "three"
	default:
		return "many"
	}
}

func stringB(n int) string {
	switch n {
	case 1:
		return "red"
	case 2:
		return "green"
	case 3:
		return "blue"
	default:
		return "grey"
	}
}

// w1..w5 are five identical wrappers: one group, five members.
func w1(x int) int {
	y := x + 1
	if y > 3 {
		y = 3
	}
	if y < 0 {
		y = 0
	}
	return y
}

func w2(x int) int {
	y := x + 1
	if y > 3 {
		y = 3
	}
	if y < 0 {
		y = 0
	}
	return y
}

func w3(x int) int {
	y := x + 1
	if y > 3 {
		y = 3
	}
	if y < 0 {
		y = 0
	}
	return y
}

func w4(x int) int {
	y := x + 1
	if y > 3 {
		y = 3
	}
	if y < 0 {
		y = 0
	}
	return y
}

func w5(x int) int {
	y := x + 1
	if y > 3 {
		y = 3
	}
	if y < 0 {
		y = 0
	}
	return y
}

var sink []int

// longA and longB share a 14-statement run inside otherwise different
// functions: one maximal region, not nine overlapping windows.
func longA() {
	println("A only")
	a := 1
	b := 2
	sink = append(sink, a)
	sink = append(sink, b)
	c := a + b
	sink = append(sink, c)
	d := c * 2
	sink = append(sink, d)
	e := d - a
	sink = append(sink, e)
	f := e + b
	sink = append(sink, f)
	g := f * c
	sink = append(sink, g)
}

func longB() {
	if len(sink) > 1000 {
		sink = nil
	}
	p := 1
	q := 2
	sink = append(sink, p)
	sink = append(sink, q)
	r := p + q
	sink = append(sink, r)
	s := r * 2
	sink = append(sink, s)
	u := s - p
	sink = append(sink, u)
	v := u + q
	sink = append(sink, v)
	w := v * r
	sink = append(sink, w)
}

var settingsOpen, logsOpen bool

// toggleSettings and toggleLogs differ only in which package variable they
// touch: a "renamed" clone (medium confidence), not an exact copy.
func toggleSettings() bool {
	if settingsOpen {
		settingsOpen = false
		println("closed")
		return false
	}
	settingsOpen = true
	println("opened")
	return true
}

func toggleLogs() bool {
	if logsOpen {
		logsOpen = false
		println("closed")
		return false
	}
	logsOpen = true
	println("opened")
	return true
}

var pa, pb, pc, pd, pe, pf int

// aliases is a table of distinct assignments: same shape, different names.
// It must not count as repeated code.
func aliases() {
	pa = scale(sink, 1)
	pb = shade(sink, 1)
	pc = tint(sink, 1)
	pd = mix(sink, 1)
	pe = fade(sink, 1)
	pf = glow(sink, 1)
}

func scale(xs []int, k int) int { return len(xs) * k }
func shade(xs []int, k int) int { return len(xs) + k }
func tint(xs []int, k int) int  { return len(xs) - k }
func mix(xs []int, k int) int   { return len(xs) ^ k }
func fade(xs []int, k int) int  { return len(xs) | k }
func glow(xs []int, k int) int  { return len(xs) & k }
