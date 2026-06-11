package graphql

// ToGraphQLCaseForTest exposes the unexported toGraphQLCase converter so the
// external graphql_test package can assert the domain → GraphQL field mapping
// (notably the empty-ReporterID → nil-pointer rule for reporterless thread-mode
// cases).
var ToGraphQLCaseForTest = toGraphQLCase
