package eval

import "fmt"

// The offline eval harness under pkg/usecase/eval is out of scope for BOTH
// policies: the fmt.Println must not be flagged (no_fmt_print exempts
// usecase/eval) and the exported ctx-less Report must not be flagged
// (usecase_context_first exempts pkg/usecase/eval).
func Report(name string) {
	fmt.Println(name)
}
