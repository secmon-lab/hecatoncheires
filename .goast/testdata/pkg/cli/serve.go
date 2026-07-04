package cli

import "fmt"

// A different file under pkg/cli is still subject to no_fmt_print.rego, so this
// call must be flagged.
func serve() {
	fmt.Println("serving")
}
