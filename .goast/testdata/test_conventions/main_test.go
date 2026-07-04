package main

// A main package is not importable, so main_test.go must use package main to
// reach the internals under test — exempt from rule (a).

func mainHelper() int { return 4 }
