package usecase_test

import (
	"reflect"
	"testing"

	"github.com/gollem-dev/gollem/mock"
	"github.com/m-mizutani/gt"

	agentkernel "github.com/secmon-lab/hecatoncheires/pkg/agent/kernel"
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
	"SlackBot", "SlackSearch", "SlackRetriever", "SlackPoster",
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

// A nil service must produce a nil INTERFACE, not a wrapper holding nil: the
// resolver checks Poster != nil, so a typed-nil wrapper would advertise a tool
// that posts nowhere.
func TestNewSlackPoster_NilServiceYieldsNilInterface(t *testing.T) {
	gt.Value(t, usecase.NewSlackPoster(nil)).Nil()
	gt.Value(t, usecase.NewSlackPoster(&stubSlackService{})).NotNil()
}
