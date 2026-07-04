package sample

import "strings"

// Two error-text discriminations — both flagged.
func bad(err error) bool {
	if strings.Contains(err.Error(), "not found") {
		return true
	}
	if strings.HasPrefix(err.Error(), "rpc error") {
		return true
	}
	return false
}
