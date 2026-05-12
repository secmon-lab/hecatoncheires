package usecase_test

import (
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

func TestShouldBroadcastActionEvent(t *testing.T) {
	cases := []struct {
		name string
		kind types.ActionEventKind
		want bool
	}{
		{"status changed is broadcast", types.ActionEventStatusChanged, true},
		{"assignee changed is broadcast", types.ActionEventAssigneeChanged, true},
		{"step added is broadcast", types.ActionEventStepAdded, true},
		{"step removed is broadcast", types.ActionEventStepRemoved, true},
		{"step done is broadcast", types.ActionEventStepDone, true},
		{"step reopened is broadcast", types.ActionEventStepReopened, true},
		{"step title changed is broadcast", types.ActionEventStepTitleChanged, true},
		{"created is not broadcast", types.ActionEventCreated, false},
		{"title changed is not broadcast", types.ActionEventTitleChanged, false},
		{"archived is not broadcast", types.ActionEventArchived, false},
		{"unarchived is not broadcast", types.ActionEventUnarchived, false},
		{"unknown kind is not broadcast", types.ActionEventKind("UNKNOWN_KIND"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gt.Value(t, usecase.ShouldBroadcastActionEventForTest(c.kind)).Equal(c.want)
		})
	}
}

func TestShouldBroadcastAnyActionEvent(t *testing.T) {
	t.Run("empty kinds are not broadcast", func(t *testing.T) {
		gt.Bool(t, usecase.ShouldBroadcastAnyActionEventForTest()).False()
	})

	t.Run("all non-broadcast kinds are not broadcast", func(t *testing.T) {
		gt.Bool(t, usecase.ShouldBroadcastAnyActionEventForTest(
			types.ActionEventTitleChanged,
			types.ActionEventCreated,
		)).False()
	})

	t.Run("a single broadcast kind triggers broadcast", func(t *testing.T) {
		gt.Bool(t, usecase.ShouldBroadcastAnyActionEventForTest(
			types.ActionEventStatusChanged,
		)).True()
	})

	t.Run("mixed kinds with at least one broadcast member trigger broadcast", func(t *testing.T) {
		gt.Bool(t, usecase.ShouldBroadcastAnyActionEventForTest(
			types.ActionEventTitleChanged,
			types.ActionEventStatusChanged,
		)).True()
	})
}
