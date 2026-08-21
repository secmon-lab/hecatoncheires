package graphql

// ToGraphQLCaseForTest exposes the unexported toGraphQLCase converter so the
// external graphql_test package can assert the domain → GraphQL field mapping
// (notably the empty-ReporterID → nil-pointer rule for reporterless thread-mode
// cases).
var ToGraphQLCaseForTest = toGraphQLCase

// ToGraphQLCaseJobForTest exposes the unexported toGraphQLCaseJob converter so
// the external graphql_test package can assert the Job definition → GraphQL
// mapping (strategy normalisation, trigger shape, schedule mutual exclusion).
var ToGraphQLCaseJobForTest = toGraphQLCaseJob

// ToGraphQLJobRunEventForTest exposes the unexported toGraphQLJobRunEvent
// converter so the external graphql_test package can assert the payload JSON
// the run-detail UI and the exported run file both read.
var ToGraphQLJobRunEventForTest = toGraphQLJobRunEvent

// ToGraphQLJobRunLogForTest exposes the unexported toGraphQLJobRunLog so a test
// can pin the wire form of a run record — the cost in particular, which is
// stored in nano-USD and read in dollars.
var ToGraphQLJobRunLogForTest = toGraphQLJobRunLog

// ToGraphQLActionCommentForTest exposes the unexported toGraphQLActionComment
// converter so the external graphql_test package can assert the domain →
// GraphQL mapping, notably that `edited` is derived from the timestamps and
// that `author` is left for the dataloader-backed resolver to fill.
var ToGraphQLActionCommentForTest = toGraphQLActionComment

// ToGraphQLFieldTypeForTest exposes the unexported toGraphQLFieldType converter
// so the external graphql_test package can assert the domain → GraphQL field
// type enum bridge (notably the markdown mapping).
var ToGraphQLFieldTypeForTest = toGraphQLFieldType
