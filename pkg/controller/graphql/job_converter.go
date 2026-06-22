package graphql

import (
	"time"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	graphql1 "github.com/secmon-lab/hecatoncheires/pkg/domain/model/graphql"
)

// toGraphQLCaseJob maps a workspace Job definition to its read-only
// GraphQL form. The strategy is normalised (empty → SIMPLE) before
// mapping so a Job that omits the TOML field still renders a concrete
// enum value.
func toGraphQLCaseJob(j *model.Job) *graphql1.CaseJob {
	if j == nil {
		return nil
	}
	return &graphql1.CaseJob{
		ID:          j.ID,
		Name:        j.Name,
		Description: j.Description,
		Strategy:    jobStrategyToGraphQL(j.Strategy),
		Quiet:       j.Quiet,
		Prompt:      j.Prompt,
		Trigger:     toGraphQLJobTrigger(j.Events),
	}
}

// jobStrategyToGraphQL maps a model.JobStrategy onto the GraphQL enum,
// normalising the empty (zero) value to SIMPLE first so TOML may omit
// the field. Unknown non-empty values fall back to SIMPLE.
func jobStrategyToGraphQL(s model.JobStrategy) graphql1.JobStrategy {
	switch model.NormaliseJobStrategy(s) {
	case model.JobStrategyPlanexec:
		return graphql1.JobStrategyPlanexec
	default:
		return graphql1.JobStrategySimple
	}
}

// toGraphQLJobTrigger maps a Job's event subscriptions to the GraphQL
// trigger shape. caseEvents is always a non-nil slice (the schema field
// is [CaseLifecycleEvent!]!); schedule is non-null only when the Job has
// a scheduled trigger.
func toGraphQLJobTrigger(ev model.JobEvents) *graphql1.JobTrigger {
	caseEvents := make([]graphql1.CaseLifecycleEvent, 0)
	if ev.Case != nil {
		for _, lc := range ev.Case.On {
			caseEvents = append(caseEvents, caseLifecycleToGraphQL(lc))
		}
	}
	return &graphql1.JobTrigger{
		CaseEvents: caseEvents,
		Schedule:   toGraphQLJobSchedule(ev.Scheduled),
	}
}

func caseLifecycleToGraphQL(lc model.CaseLifecycle) graphql1.CaseLifecycleEvent {
	switch lc {
	case model.CaseLifecycleClosed:
		return graphql1.CaseLifecycleEventClosed
	default:
		return graphql1.CaseLifecycleEventCreated
	}
}

// toGraphQLJobSchedule maps the scheduled-event config to its GraphQL
// form. Returns nil when the Job has no scheduled trigger. everySeconds
// and cron are mutually exclusive, mirroring model.ScheduledEventConfig:
// Every is surfaced as whole seconds, Cron as its original expression.
func toGraphQLJobSchedule(s *model.ScheduledEventConfig) *graphql1.JobSchedule {
	if s == nil {
		return nil
	}
	sched := &graphql1.JobSchedule{}
	switch {
	case s.Every > 0:
		secs := int(s.Every / time.Second)
		sched.EverySeconds = &secs
	case s.CronExpr != "":
		expr := s.CronExpr
		sched.Cron = &expr
	}
	return sched
}
