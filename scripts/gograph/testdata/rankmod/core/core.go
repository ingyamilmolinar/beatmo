package core

// Run calls helper from three distinct units (Run, runB, runC).
func Run() {
	helper()
	runB()
	runC()
}

func runB() { helper() }

func runC() { helper() }

func helper() int { return 1 }
