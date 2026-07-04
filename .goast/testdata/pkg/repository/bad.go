package repo

// A mirror doc type plus its converters — three violations
// (type caseDoc, func toCaseDoc, func fromCaseDoc).
type caseDoc struct {
	ID string
}

func toCaseDoc(id string) caseDoc { return caseDoc{ID: id} }

func fromCaseDoc(d caseDoc) string { return d.ID }
