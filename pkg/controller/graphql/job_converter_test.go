package graphql_test

import (
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/robfig/cron/v3"

	graphqlctrl "github.com/secmon-lab/hecatoncheires/pkg/controller/graphql"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	graphql1 "github.com/secmon-lab/hecatoncheires/pkg/domain/model/graphql"
)

// TestToGraphQLCaseJob pins the Job definition → GraphQL mapping: strategy
// normalisation, the trigger shape for each event domain, and the mutual
// exclusion between interval and cron schedules.
func TestToGraphQLCaseJob(t *testing.T) {
	t.Run("case-event job maps lifecycle events and omits schedule", func(t *testing.T) {
		g := graphqlctrl.ToGraphQLCaseJobForTest(&model.Job{
			ID:          "triage",
			Name:        "Initial triage",
			Description: "evaluate on create",
			Prompt:      "do the thing",
			Strategy:    model.JobStrategyPlanexec,
			Quiet:       false,
			Events: model.JobEvents{
				Case: &model.CaseEventConfig{On: []model.CaseLifecycle{
					model.CaseLifecycleCreated, model.CaseLifecycleClosed,
				}},
			},
		})
		gt.String(t, g.ID).Equal("triage")
		gt.String(t, g.Name).Equal("Initial triage")
		gt.String(t, g.Description).Equal("evaluate on create")
		gt.String(t, g.Prompt).Equal("do the thing")
		gt.Bool(t, g.Quiet).False()
		gt.Value(t, g.Strategy).Equal(graphql1.JobStrategyPlanexec)
		gt.Array(t, g.Trigger.CaseEvents).Length(2).Required()
		gt.Value(t, g.Trigger.CaseEvents[0]).Equal(graphql1.CaseLifecycleEventCreated)
		gt.Value(t, g.Trigger.CaseEvents[1]).Equal(graphql1.CaseLifecycleEventClosed)
		gt.Value(t, g.Trigger.Schedule).Nil()
	})

	t.Run("empty strategy normalises to SIMPLE", func(t *testing.T) {
		g := graphqlctrl.ToGraphQLCaseJobForTest(&model.Job{
			ID:     "stale",
			Name:   "Stale check",
			Prompt: "p",
			Quiet:  true,
			Events: model.JobEvents{Scheduled: &model.ScheduledEventConfig{Every: 3600_000_000_000}}, // 1h in ns
		})
		gt.Value(t, g.Strategy).Equal(graphql1.JobStrategySimple)
		gt.Bool(t, g.Quiet).True()
		// caseEvents is a non-nil empty slice for a schedule-only Job.
		gt.Array(t, g.Trigger.CaseEvents).Length(0)
		gt.Value(t, g.Trigger.Schedule).NotNil()
		gt.Value(t, g.Trigger.Schedule.EverySeconds).NotNil()
		gt.Number(t, *g.Trigger.Schedule.EverySeconds).Equal(3600)
		gt.Value(t, g.Trigger.Schedule.Cron).Nil()
	})

	t.Run("cron schedule maps the original expression and leaves everySeconds null", func(t *testing.T) {
		sched, err := cron.ParseStandard("0 9 * * *")
		gt.NoError(t, err).Required()
		g := graphqlctrl.ToGraphQLCaseJobForTest(&model.Job{
			ID:     "daily",
			Name:   "Daily summary",
			Prompt: "p",
			Events: model.JobEvents{Scheduled: &model.ScheduledEventConfig{Cron: sched, CronExpr: "0 9 * * *"}},
		})
		gt.Value(t, g.Trigger.Schedule).NotNil()
		gt.Value(t, g.Trigger.Schedule.Cron).NotNil()
		gt.String(t, *g.Trigger.Schedule.Cron).Equal("0 9 * * *")
		gt.Value(t, g.Trigger.Schedule.EverySeconds).Nil()
	})

	t.Run("nil job maps to nil", func(t *testing.T) {
		gt.Value(t, graphqlctrl.ToGraphQLCaseJobForTest(nil)).Nil()
	})
}
