package sample

import "fmt"

// Every call below must be flagged by no_fmt_print.rego.
func bad() {
	fmt.Print("a")
	fmt.Printf("%d\n", 1)
	fmt.Println("c")
	println("builtin")
	print("builtin")
}
