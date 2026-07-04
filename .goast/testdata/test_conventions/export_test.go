package sample

// export_test.go legitimately uses the internal package to re-export seams —
// exempt from rule (a).

var HelperForTest = helper2

func helper2() int { return 2 }
