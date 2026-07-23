package bqexport

// Test seams: expose the pure, BigQuery-client-free helpers so the schema
// mapping / diff / value-encoding policy can be unit-tested without a live
// BigQuery connection.

var (
	ToBQSchemaForTest             = toBQSchema
	DiffSchemaForTest             = diffSchema
	EncodeValueForTest            = encodeValue
	PreserveExistingSchemaForTest = preserveExistingSchema
	IsSchemaMismatchForTest       = isSchemaMismatch
)
