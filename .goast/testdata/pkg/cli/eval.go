package cli

import "fmt"

// eval.go under pkg/cli is exempt from no_fmt_print.rego: the eval CLI command
// legitimately writes to stdout. This call must NOT be flagged.
func listTools() {
	fmt.Println("tools")
}
