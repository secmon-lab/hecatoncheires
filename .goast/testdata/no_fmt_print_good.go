// Sample input for no_fmt_print.rego — nothing below must be flagged.
package testdata

import (
	"fmt"
	"os"
)

func good() string {
	fmt.Fprintln(os.Stderr, "explicit io.Writer is fine") // Fprint* — allowed
	_ = fmt.Errorf("errorf is fine")                      // not a Print* selector
	return fmt.Sprintf("%d", 1)                           // Sprint* only returns a string
}
