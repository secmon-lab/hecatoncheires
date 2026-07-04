package sample

// Two fields carry firestore tags — both flagged.
type record struct {
	ID    string `firestore:"id"`
	Title string `firestore:"title" json:"title"`
}
