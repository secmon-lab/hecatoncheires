package sample

// A comment mentioning firestore:"..." must NOT be flagged — only real tags are.
// The struct is the Firestore wire format directly: do NOT add firestore:"..." tags.
type model struct {
	ID    string
	Title string `json:"title"`
}
