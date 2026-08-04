package bqexport

// Test seams: expose the pure, BigQuery-client-free helpers so the schema
// mapping / staging-name / value-encoding policy can be unit-tested without a
// live BigQuery connection.

var (
	ToBQSchemaForTest       = toBQSchema
	RowDescriptorForTest    = rowDescriptor
	EncodeRowForTest        = encodeRow
	EncodeValueForTest      = encodeValue
	StagingTableNameForTest = stagingTableName
	DDLColumnListForTest    = ddlColumnList
	DDLSelectListForTest    = ddlSelectList
)

// AppendMaxRequestBytesForTest lets the live test size its payload so it
// certainly spans several AppendRows calls.
const AppendMaxRequestBytesForTest = appendMaxRequestBytes
