package foo

// Thing is an exported type used across packages.
type Thing struct{ N int }

// Process is an exported method; call sites on an imported Thing value must
// resolve to this package (the AST-only case that type resolution catches).
func (t Thing) Process() int { return t.N * 2 }

// Deep calls another in-module function, so its call-graph depth is 2.
// It exercises the method-receiver join (AST recv name must match the
// types-derived recv name) with a depth that differs from the leaf default.
func (t Thing) Deep() int { return Helper(t.N) }

// Helper is a free exported function.
func Helper(x int) int {
	if x > 0 {
		for i := 0; i < x; i++ {
			if i%2 == 0 {
				x += i
			}
		}
	}
	return x
}
