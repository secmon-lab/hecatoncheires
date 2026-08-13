package cli

import (
	"github.com/gollem-dev/gollem"

	notiontool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/notion"
	slacktool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/slack"
	httpctrl "github.com/secmon-lab/hecatoncheires/pkg/controller/http"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	slacksvc "github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

// RegistryHasInteractiveJobForTest exposes registryHasInteractiveJob.
var RegistryHasInteractiveJobForTest = registryHasInteractiveJob

// InProcessExecutorsForTest exposes inProcessExecutors so a test can pin which
// deployments still get one without standing up a server.
var InProcessExecutorsForTest = inProcessExecutors

// BuildJobToolsForTest exposes buildJobTools so tests can assert the
// per-workspace-mode tool composition without standing up a full job runtime.
// Adapters are left zero-valued: buildJobTools only constructs tool structs
// (which hold their deps); the adapters are exercised at tool-call time, not at
// build time.
func BuildJobToolsForTest(c *model.Case, ws *model.WorkspaceEntry) []gollem.Tool {
	return buildJobTools(jobRuntimeDeps{}, jobToolAdapters{}, c, ws)
}

// JobReadToolDepsForTest carries the read-only tool dependencies a test wants
// to inject into buildJobTools. Each is an interface, so a nil value omits the
// corresponding tool exactly as a nil dependency would in production.
type JobReadToolDepsForTest struct {
	Bot       slacksvc.Service
	Search    slacktool.SearchService
	Retriever slacktool.MessageRetriever
	Notion    notiontool.Client
	Jira      []gollem.Tool
}

// BuildJobToolsWithReadDepsForTest exposes buildJobTools with the read-only
// Slack / Notion / Jira dependencies populated, so tests can assert that
// those tools are wired in (and omitted when their deps are nil/empty)
// across both workspace modes. Only construction is exercised; the deps'
// methods are never called.
func BuildJobToolsWithReadDepsForTest(deps JobReadToolDepsForTest, c *model.Case, ws *model.WorkspaceEntry) []gollem.Tool {
	return buildJobTools(jobRuntimeDeps{
		SlackService:   deps.Bot,
		SlackSearch:    deps.Search,
		SlackRetriever: deps.Retriever,
		NotionTool:     deps.Notion,
		JiraTools:      deps.Jira,
	}, jobToolAdapters{}, c, ws)
}

// --- serve.go seams (GraphQL error → HTTP status mapping) ------------------

// ClassifyErrorForTest exposes classifyError.
var ClassifyErrorForTest = classifyError

// StatusForExtensionCodeForTest exposes statusForExtensionCode.
var StatusForExtensionCodeForTest = statusForExtensionCode

// GraphqlErrorStatusMiddlewareForTest exposes graphqlErrorStatusMiddleware.
var GraphqlErrorStatusMiddlewareForTest = graphqlErrorStatusMiddleware

// HTTPStatusForGraphQLErrorCodesForTest builds a GraphQL error-envelope list
// from the given extension codes and runs httpStatusForGraphQLErrors over it,
// so tests can assert the status mapping without naming the internal envelope
// type (gqlErrorEnvelope stays unexported).
func HTTPStatusForGraphQLErrorCodesForTest(codes ...string) int {
	out := make([]gqlErrorEnvelope, len(codes))
	for i, c := range codes {
		out[i].Extensions.Code = c
	}
	return httpStatusForGraphQLErrors(out)
}

// --- dbcheck.go seams (POST /api/validate/db) -----------------------------

// NewDBConsistencyCheckerForTest exposes newDBConsistencyChecker behind the
// interface the HTTP handler consumes, so the concrete type stays unexported.
func NewDBConsistencyCheckerForTest(uc *usecase.UseCases) httpctrl.DBConsistencyChecker {
	return newDBConsistencyChecker(uc)
}

// --- eval.go / diagnosis.go command constructors --------------------------

// CmdEvalForTest exposes cmdEval.
var CmdEvalForTest = cmdEval

// CmdDiagnosisForTest exposes cmdDiagnosis.
var CmdDiagnosisForTest = cmdDiagnosis

// CmdFixUnsentActionForTest exposes cmdFixUnsentAction.
var CmdFixUnsentActionForTest = cmdFixUnsentAction
