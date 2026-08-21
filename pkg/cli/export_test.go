package cli

import (
	"context"

	"github.com/gollem-dev/gollem"
	"github.com/urfave/cli/v3"

	agentkernel "github.com/secmon-lab/hecatoncheires/pkg/agent/kernel"
	notiontool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/notion"
	slacktool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/cli/config"
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

// TickIntegrationConfigsForTest exposes the sweep's integration configuration
// bag so a test can drive configureTickIntegrations.
type TickIntegrationConfigsForTest = tickIntegrationConfigs

// ConfigureTickIntegrationsForTest runs configureTickIntegrations and returns
// what a caller can observe: the Slack service and Jira tools (which no usecase
// accessor exposes) plus the usecase options, so a test can apply them through
// usecase.New and assert on the built UseCases rather than on opaque funcs.
func ConfigureTickIntegrationsForTest(ctx context.Context, cfg tickIntegrationConfigs) (slacksvc.Service, []gollem.Tool, []usecase.Option, error) {
	out, err := configureTickIntegrations(ctx, cfg)
	return out.slack, out.jiraTools, out.ucOpts, err
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

// --- agent_models.go seams (model resolution at startup) ------------------

// JobModelRefsForTest exposes jobModelRefs.
var JobModelRefsForTest = jobModelRefs

// LLMSetupForTest is what buildLLMSetup returns, flattened so a test can read it
// without the unexported struct: the default client (nil when no LLM is
// configured) and the policy every run is resolved through.
type LLMSetupForTest struct {
	Default gollem.LLMClient
	Policy  agentkernel.ModelPolicy
}

// BuildLLMSetupForTest runs buildLLMSetup through a real flag parse, so the test
// exercises the same --global-config / --llm-model path production does. args are
// appended after the command name.
func BuildLLMSetupForTest(ctx context.Context, registry *model.WorkspaceRegistry,
	args ...string,
) (LLMSetupForTest, error) {
	var appCfg config.AppConfig
	var llmCfg config.LLM
	var agentCfg config.Agent

	flags := appCfg.GlobalConfigFlags()
	flags = append(flags, llmCfg.Flags()...)
	flags = append(flags, agentCfg.Flags()...)

	var out LLMSetupForTest
	var outErr error
	cmd := &cli.Command{
		Name:  "test",
		Flags: flags,
		Action: func(_ context.Context, c *cli.Command) error {
			setup, err := buildLLMSetup(ctx, c, &appCfg, &llmCfg, registry, agentCfg.BudgetOr)
			out = LLMSetupForTest(setup)
			outErr = err
			return nil
		},
	}
	if err := cmd.Run(ctx, append([]string{"test"}, args...)); err != nil {
		return LLMSetupForTest{}, err
	}
	return out, outErr
}

// --- eval.go / diagnosis.go command constructors --------------------------

// CmdEvalForTest exposes cmdEval.
var CmdEvalForTest = cmdEval

// CmdDiagnosisForTest exposes cmdDiagnosis.
var CmdDiagnosisForTest = cmdDiagnosis

// CmdFixUnsentActionForTest exposes cmdFixUnsentAction.
var CmdFixUnsentActionForTest = cmdFixUnsentAction
