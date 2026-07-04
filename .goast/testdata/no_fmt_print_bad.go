// Sample input for no_fmt_print.rego — every call below must be flagged.
// This file lives under .goast/testdata (a dot dir), so the Go toolchain
// ignores it; it is only ever parsed by `goast eval`.
package testdata

import "fmt"

func bad() {
	fmt.Print("no")     // fmt.Print
	fmt.Printf("%d", 1) // fmt.Printf
	fmt.Println("no")   // fmt.Println
	println("builtin")  // builtin println
	print("builtin")    // builtin print
}
