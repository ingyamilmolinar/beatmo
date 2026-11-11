package core

// legacyHelper has no caller in any analysed build, but a `js && test` file
// (a build this analysis never compiles) names it.
func legacyHelper() int { return 4 }
