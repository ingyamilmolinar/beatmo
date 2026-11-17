package foo

// Extra is a method on Thing declared in a DIFFERENT file than the Thing type
// (which lives in foo.go), exercising the cross-file method re-nesting post-pass.
func (t Thing) Extra() int { return t.N + 1 }
