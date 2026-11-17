package core

type spec struct {
	name string
	n    int
}

// Table is data, not logic.
func Table() []spec {
	return []spec{
		{name: "a", n: 1},
		{name: "b", n: 2},
		{name: "c", n: 3},
		{name: "d", n: 4},
		{name: "e", n: 5},
		{name: "f", n: 6},
		{name: "g", n: 7},
		{name: "h", n: 8},
		{name: "i", n: 9},
		{name: "j", n: 10},
	}
}
