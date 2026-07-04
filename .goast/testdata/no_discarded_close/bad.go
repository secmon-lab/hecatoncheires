package sample

type closer interface{ Close() error }

type response struct{ Body closer }

// Four unsafe closes — all flagged: discarded, bare, bare defer, and a
// two-level-receiver bare close (resp.Body.Close()).
func bad(c closer, resp response) {
	_ = c.Close()
	c.Close()
	defer c.Close()
	resp.Body.Close()
}
