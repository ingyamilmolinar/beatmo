package brk

// Used is called from the app in every config.
func Used() int { return 1 }

// Unused is never called.
func Unused() int { return 2 }
