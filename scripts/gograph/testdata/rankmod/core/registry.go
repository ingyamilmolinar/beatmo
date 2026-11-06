package core

var effects []string

// Two explicit init functions in one package: SSA names them init#1, init#2.
func init() { effects = append(effects, "a") }

func init() { effects = append(effects, "b") }
