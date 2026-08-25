package usecase_test

import (
	"reflect"
	"testing"

	"github.com/gollem-dev/gollem/mock"
	"github.com/m-mizutani/gt"

	agentkernel "github.com/secmon-lab/hecatoncheires/pkg/agent/kernel"
	slacktool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	slacksvc "github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

// stubSlackService stands in for a configured Slack service. It embeds the
// interface (nil), so any call panics: these tests only observe whether the
// wiring produced a client, never call through it.
type stubSlackService struct{ slacksvc.Service }

// knownToolDepFields is every field of agentkernel.ToolDeps, enumerated so that
// adding one without teaching AgentToolDeps to fill it fails here.
//
// The bug this constructor exists to prevent was exactly that: SlackPoster was
// declared on ToolDeps and set by no caller, so slack__post_to_case_channel was
// advertised to every Job planner and built for nobody. A compiler cannot catch
// an unset struct field, so this test is the check.
var knownToolDepFields = []string{
	"Repo", "Registry",
	"SlackBot", "SlackSearch", "SlackRetriever", "SlackPoster", "SlackLimits",
	"NotionClient", "GitHubClient", "WebFetchClient", "JiraTools",
	"ActionUC", "ActionStepUC", "CaseUC", "CaseRefUC",
	"CaseMultiUC", "CaseMultiActionUC", "MemoUC",
	"KnowledgeAccessor", "KnowledgeMutator",
}

func TestAgentToolDeps_CoversEveryToolDepField(t *testing.T) {
	typ := reflect.TypeOf(agentkernel.ToolDeps{})
	got := make([]string, 0, typ.NumField())
	for i := range typ.NumField() {
		got = append(got, typ.Field(i).Name)
	}
	gt.Value(t, got).Equal(knownToolDepFields)
}

// The fields that do not depend on an optional integration must be populated
// whenever a UseCases exists at all: they are the repository, the registry and
// the usecase adapters every agent tool is built from.
func TestAgentToolDeps_PopulatesTheUnconditionalFields(t *testing.T) {
	uc := usecase.New(memory.New(), model.NewWorkspaceRegistry(),
		usecase.WithLLMClient(&mock.LLMClientMock{}),
		usecase.WithSlackService(&stubSlackService{}),
	)

	deps := uc.AgentToolDeps()
	v := reflect.ValueOf(deps)
	unconditional := []string{
		"Repo", "Registry", "SlackBot", "SlackPoster",
		"ActionUC", "ActionStepUC", "CaseUC", "CaseRefUC",
		"CaseMultiUC", "CaseMultiActionUC", "MemoUC",
		"KnowledgeAccessor", "KnowledgeMutator",
	}
	for _, name := range unconditional {
		field := v.FieldByName(name)
		gt.Bool(t, field.IsValid()).True().Required()
		gt.Bool(t, field.IsZero()).False()
	}
}

// The poster is the field that was missing in production. It must exist exactly
// when a Slack service does, because the tool factory decides whether
// slack__post_to_case_channel exists at all from this value.
func TestAgentToolDeps_SlackPosterFollowsTheSlackService(t *testing.T) {
	withSlack := usecase.New(memory.New(), model.NewWorkspaceRegistry(),
		usecase.WithLLMClient(&mock.LLMClientMock{}),
		usecase.WithSlackService(&stubSlackService{}),
	)
	gt.Value(t, withSlack.AgentToolDeps().SlackPoster).NotNil()

	withoutSlack := usecase.New(memory.New(), model.NewWorkspaceRegistry())
	gt.Value(t, withoutSlack.AgentToolDeps().SlackPoster).Nil()
}

// The Slack read tools' size bounds must reach the tool factory unchanged. They
// are a value, not a client, so the "is it nil" checks above cannot catch a
// wiring that drops them — a lost bound leaves the tools unbounded with no other
// symptom than a large bill.
func TestAgentToolDeps_CarriesTheSlackToolLimits(t *testing.T) {
	uc := usecase.New(memory.New(), model.NewWorkspaceRegistry(),
		usecase.WithSlackToolLimits(slacktool.Limits{MaxTextBytes: 512, MaxResultBytes: 2048}),
	)
	deps := uc.AgentToolDeps()
	gt.Number(t, deps.SlackLimits.MaxTextBytes).Equal(512)
	gt.Number(t, deps.SlackLimits.MaxResultBytes).Equal(2048)

	// Without the option the bounds are the zero value, which the tools read as
	// "disabled" — the CLI flags own the defaults, not this package.
	bare := usecase.New(memory.New(), model.NewWorkspaceRegistry())
	gt.Number(t, bare.AgentToolDeps().SlackLimits.MaxTextBytes).Equal(0)
	gt.Number(t, bare.AgentToolDeps().SlackLimits.MaxResultBytes).Equal(0)
}

// A nil service must produce a nil INTERFACE, not a wrapper holding nil: the
// resolver checks Poster != nil, so a typed-nil wrapper would advertise a tool
// that posts nowhere.
func TestNewSlackPoster_NilServiceYieldsNilInterface(t *testing.T) {
	gt.Value(t, usecase.NewSlackPoster(nil)).Nil()
	gt.Value(t, usecase.NewSlackPoster(&stubSlackService{})).NotNil()
}
