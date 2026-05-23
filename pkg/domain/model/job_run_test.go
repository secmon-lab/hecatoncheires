package model_test

import (
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

func TestJobRunStatus_IsValid(t *testing.T) {
	gt.Bool(t, model.JobRunStatusSuccess.IsValid()).True()
	gt.Bool(t, model.JobRunStatusFailed.IsValid()).True()
	gt.Bool(t, model.JobRunStatus("RUNNING").IsValid()).False()
	gt.Bool(t, model.JobRunStatus("").IsValid()).False()
}

func TestJobRunKey_Validate(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		k := model.JobRunKey{WorkspaceID: "ws", CaseID: 1, JobID: "job"}
		gt.NoError(t, k.Validate())
	})
	t.Run("empty workspace", func(t *testing.T) {
		k := model.JobRunKey{CaseID: 1, JobID: "job"}
		gt.Error(t, k.Validate())
	})
	t.Run("zero case", func(t *testing.T) {
		k := model.JobRunKey{WorkspaceID: "ws", JobID: "job"}
		gt.Error(t, k.Validate())
	})
	t.Run("empty job", func(t *testing.T) {
		k := model.JobRunKey{WorkspaceID: "ws", CaseID: 1}
		gt.Error(t, k.Validate())
	})
}

func TestJobRun_IsLeased(t *testing.T) {
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)

	t.Run("active lease", func(t *testing.T) {
		r := &model.JobRun{LeaseUntil: now.Add(5 * time.Minute)}
		gt.Bool(t, r.IsLeased(now)).True()
	})
	t.Run("expired lease", func(t *testing.T) {
		r := &model.JobRun{LeaseUntil: now.Add(-time.Second)}
		gt.Bool(t, r.IsLeased(now)).False()
	})
	t.Run("zero lease (idle)", func(t *testing.T) {
		r := &model.JobRun{}
		gt.Bool(t, r.IsLeased(now)).False()
	})
	t.Run("nil receiver", func(t *testing.T) {
		var r *model.JobRun
		gt.Bool(t, r.IsLeased(now)).False()
	})
}
