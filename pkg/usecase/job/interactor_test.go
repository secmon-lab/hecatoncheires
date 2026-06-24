package job_test

import (
	"context"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	goslack "github.com/slack-go/slack"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/interaction"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	slacksvc "github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/job"
)

type postedForm struct {
	channelID string
	threadTS  string
	blocks    []goslack.Block
	text      string
}

type updatedForm struct {
	channelID string
	timestamp string
	blocks    []goslack.Block
	text      string
}

type fakeQuestionPoster struct {
	posts     []postedForm
	updates   []updatedForm
	returnTS  string
	returnErr error
}

func (f *fakeQuestionPoster) PostThreadMessage(_ context.Context, channelID, threadTS string, blocks []goslack.Block, text string, _ ...slacksvc.PostThreadOption) (string, error) {
	f.posts = append(f.posts, postedForm{channelID: channelID, threadTS: threadTS, blocks: blocks, text: text})
	if f.returnErr != nil {
		return "", f.returnErr
	}
	return f.returnTS, nil
}

func (f *fakeQuestionPoster) UpdateMessage(_ context.Context, channelID, timestamp string, blocks []goslack.Block, text string) error {
	f.updates = append(f.updates, updatedForm{channelID: channelID, timestamp: timestamp, blocks: blocks, text: text})
	return nil
}

func newRunningLog(key model.JobRunKey, runID string, started time.Time) *model.JobRunLog {
	return &model.JobRunLog{
		WorkspaceID:  key.WorkspaceID,
		CaseID:       key.CaseID,
		JobID:        key.JobID,
		RunID:        runID,
		TraceID:      "trace-" + runID,
		Stage:        model.JobRunStageRunning,
		StartedAt:    started,
		ExecutorKind: "planexec",
	}
}

func jobKey(suffix string) model.JobRunKey {
	return model.JobRunKey{
		WorkspaceID: "ws-" + suffix,
		CaseID:      time.Now().UnixNano(),
		JobID:       "job-" + suffix,
	}
}

func sampleRequest() interaction.Request {
	return interaction.Request{
		Reason: "which environment is affected?",
		Items: []interaction.Item{
			{ID: "env", Text: "Which environment?", Type: interaction.ItemSelect, Options: []string{"prod", "stg"}},
			{ID: "note", Text: "Anything else?", Type: interaction.ItemFreeText},
		},
	}
}

func TestJobInteractor_Solicit(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 24, 9, 0, 0, 0, time.UTC)

	t.Run("suspends run and posts form", func(t *testing.T) {
		repo := memory.New()
		key := jobKey("solicit")
		runID := "run-solicit"
		gt.NoError(t, repo.JobRunLog().Create(ctx, newRunningLog(key, runID, now))).Required()

		poster := &fakeQuestionPoster{returnTS: "1700000000.000999"}
		it := job.NewJobInteractorForTest(repo, poster, key, runID, "C42", "1699999999.000001", "U7",
			newRunningLog(key, runID, now), func() time.Time { return now })

		out, err := it.Solicit(ctx, sampleRequest())
		gt.NoError(t, err).Required()
		gt.Bool(t, out.Paused).True()

		// The form was posted to the case thread.
		gt.Array(t, poster.posts).Length(1).Required()
		gt.String(t, poster.posts[0].channelID).Equal("C42")
		gt.String(t, poster.posts[0].threadTS).Equal("1699999999.000001")

		// The run log is now AWAITING_INPUT with the question + posted coords.
		log, err := repo.JobRunLog().Get(ctx, key, runID)
		gt.NoError(t, err).Required()
		gt.Value(t, log.Stage).Equal(model.JobRunStageAwaitingInput)
		gt.Value(t, log.PendingInteraction).NotNil().Required()
		gt.String(t, log.PendingInteraction.PostedChannelID).Equal("C42")
		gt.String(t, log.PendingInteraction.PostedMessageTS).Equal("1700000000.000999")
		gt.String(t, log.PendingInteraction.Reason).Equal("which environment is affected?")
		gt.Array(t, log.PendingInteraction.Items).Length(2).Required()
		gt.String(t, log.PendingInteraction.Items[0].ID).Equal("env")
		gt.String(t, log.PendingInteraction.Items[0].Type).Equal("select")
		gt.Array(t, log.PendingInteraction.Items[0].Options).Equal([]string{"prod", "stg"})

		// The JobRun is suspended (marker set, lease released).
		run, err := repo.JobRun().Get(ctx, key)
		gt.NoError(t, err).Required()
		gt.String(t, run.SuspendedRunID).Equal(runID)
		gt.Bool(t, run.IsSuspended()).True()
		gt.Bool(t, run.SuspendedAt.Equal(now)).True()
		gt.Bool(t, run.LeaseUntil.IsZero()).True()
	})

	t.Run("no slack thread is a hard error", func(t *testing.T) {
		repo := memory.New()
		key := jobKey("nothread")
		runID := "run-nothread"
		gt.NoError(t, repo.JobRunLog().Create(ctx, newRunningLog(key, runID, now))).Required()

		poster := &fakeQuestionPoster{returnTS: "x"}
		it := job.NewJobInteractorForTest(repo, poster, key, runID, "", "", "U7",
			newRunningLog(key, runID, now), func() time.Time { return now })

		_, err := it.Solicit(ctx, sampleRequest())
		gt.Error(t, err)
		gt.Array(t, poster.posts).Length(0)
	})

	t.Run("invalid request is rejected", func(t *testing.T) {
		repo := memory.New()
		key := jobKey("badreq")
		runID := "run-badreq"
		gt.NoError(t, repo.JobRunLog().Create(ctx, newRunningLog(key, runID, now))).Required()

		poster := &fakeQuestionPoster{returnTS: "x"}
		it := job.NewJobInteractorForTest(repo, poster, key, runID, "C1", "1.1", "U7",
			newRunningLog(key, runID, now), func() time.Time { return now })

		_, err := it.Solicit(ctx, interaction.Request{Items: nil})
		gt.Error(t, err)
		gt.Array(t, poster.posts).Length(0)
	})
}

func TestJobInteractor_RoundTripAnswers(t *testing.T) {
	// Build a form, simulate the user's block-action submission state, parse
	// it back, and assert the answers match what was selected/typed.
	pending := &model.PendingInteraction{
		PostedChannelID: "C1",
		PostedMessageTS: "1.1",
		Reason:          "r",
		Items: []model.PendingInteractionItem{
			{ID: "env", Text: "Which environment?", Type: "select", Options: []string{"prod", "stg"}},
			{ID: "tags", Text: "Tags?", Type: "multi_select", Options: []string{"a", "b", "c"}},
			{ID: "note", Text: "Notes?", Type: "free_text"},
		},
	}

	state := &goslack.BlockActionStates{
		Values: map[string]map[string]goslack.BlockAction{
			"job_question_item:env": {
				"job_question_choice": {SelectedOption: goslack.OptionBlockObject{Value: "prod"}},
			},
			"job_question_item:tags": {
				"job_question_choice": {SelectedOptions: []goslack.OptionBlockObject{{Value: "a"}, {Value: "c"}}},
			},
			"job_question_item:note": {
				"job_question_free_text": {Value: "rollback already started"},
			},
		},
	}

	answers := job.ParseJobQuestionAnswersForTest(pending, state)
	gt.Array(t, answers).Length(3).Required()

	byID := map[string]interaction.Answer{}
	for _, a := range answers {
		byID[a.ID] = a
	}
	gt.String(t, byID["env"].Choice).Equal("prod")
	gt.Array(t, byID["tags"].Choices).Equal([]string{"a", "c"})
	gt.String(t, byID["note"].FreeText).Equal("rollback already started")
}

func TestJobInteractor_ParseSkipsUnanswered(t *testing.T) {
	pending := &model.PendingInteraction{
		PostedChannelID: "C1",
		PostedMessageTS: "1.1",
		Items: []model.PendingInteractionItem{
			{ID: "a", Text: "A?", Type: "free_text"},
			{ID: "b", Text: "B?", Type: "free_text"},
		},
	}
	state := &goslack.BlockActionStates{
		Values: map[string]map[string]goslack.BlockAction{
			"job_question_item:a": {"job_question_free_text": {Value: "answered"}},
			"job_question_item:b": {"job_question_free_text": {Value: "   "}}, // whitespace only → unanswered
		},
	}
	answers := job.ParseJobQuestionAnswersForTest(pending, state)
	gt.Array(t, answers).Length(1).Required()
	gt.String(t, answers[0].ID).Equal("a")
}

func TestJobQuestionRef_RoundTrip(t *testing.T) {
	// Encode happens inside Solicit; here we drive the decode path against a
	// posted form's button value to prove the resume context round-trips.
	ctx := context.Background()
	now := time.Date(2026, 6, 24, 9, 0, 0, 0, time.UTC)
	repo := memory.New()
	key := jobKey("ref")
	runID := "run-ref-123"
	gt.NoError(t, repo.JobRunLog().Create(ctx, newRunningLog(key, runID, now))).Required()
	poster := &fakeQuestionPoster{returnTS: "1.2"}
	it := job.NewJobInteractorForTest(repo, poster, key, runID, "C1", "1.1", "U7",
		newRunningLog(key, runID, now), func() time.Time { return now })
	_, err := it.Solicit(ctx, sampleRequest())
	gt.NoError(t, err).Required()

	// Pull the Submit button value out of the posted blocks.
	var refValue string
	for _, b := range poster.posts[0].blocks {
		if ab, ok := b.(*goslack.ActionBlock); ok {
			for _, el := range ab.Elements.ElementSet {
				if btn, ok := el.(*goslack.ButtonBlockElement); ok && btn.ActionID == job.ActionIDJobQuestionSubmit {
					refValue = btn.Value
				}
			}
		}
	}
	gt.String(t, refValue).NotEqual("")

	ws, caseID, jobID, gotRunID, err := job.DecodeJobQuestionRefForTest(refValue)
	gt.NoError(t, err).Required()
	gt.String(t, ws).Equal(key.WorkspaceID)
	gt.Number(t, caseID).Equal(key.CaseID)
	gt.String(t, jobID).Equal(key.JobID)
	gt.String(t, gotRunID).Equal(runID)
}
