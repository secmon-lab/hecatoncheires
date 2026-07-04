package sample

type srv struct{}

func (s srv) Close() error { return nil }

// The same unsafe closes inside a _test.go file are exempt: httptest.Server
// teardown and similar are idiomatic and carry no leak risk.
func teardown(s srv) {
	defer s.Close()
	_ = s.Close()
}
