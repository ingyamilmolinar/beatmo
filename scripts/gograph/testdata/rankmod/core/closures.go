package core

var registry []func(int) int

func register(f func(int) int) { registry = append(registry, f) }

// Reg registers a closure whose control flow belongs to the closure unit.
func Reg() {
	register(func(v int) int {
		if v > 0 {
			for i := 0; i < v; i++ {
				if i == 3 && v > 9 {
					return i
				}
			}
		}
		return 0
	})
	if len(registry) == 0 {
		return
	}
}
