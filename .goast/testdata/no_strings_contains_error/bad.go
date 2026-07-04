package sample

import "strings"

// Three error-text discriminations — all flagged.
func bad(err error) bool {
	if strings.Contains(err.Error(), "not found") {
		return true
	}
	if strings.HasPrefix(err.Error(), "rpc error") {
		return true
	}
	if strings.EqualFold(err.Error(), "timeout") {
		return true
	}
	return false
}
