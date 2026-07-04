package repo

// No mirror doc type, no converters — the model is persisted directly.
type record struct {
	ID string
}

func loadRecord(id string) record { return record{ID: id} }
