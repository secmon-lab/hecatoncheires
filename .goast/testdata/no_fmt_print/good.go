package sample

import (
	"fmt"
	"os"
)

// None of these may be flagged: Sprint* only returns a string, Fprint* targets
// an explicit io.Writer, and Errorf builds an error value.
func good() {
	s := fmt.Sprintf("%d", 1)
	fmt.Fprintln(os.Stdout, s)
	_ = fmt.Errorf("boom: %s", s)
}
