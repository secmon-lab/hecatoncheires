package driver

import (
	"context"
	"fmt"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	slackmodel "github.com/secmon-lab/hecatoncheires/pkg/domain/model/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/env"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/evaltype"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/scenario"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/async"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
)

const (
	// runThreadTS is the synthetic parent timestamp of the scenario thread.
	runThreadTS = "1700000000.000100"
	// defaultReporter is used when the scenario does not pin one.
	defaultReporter = "U-EVALUSER"
	evalTeamID      = "T-EVAL"
)

// ThreadInitial drives the thread-mode initial sequence: a top-level post
// starts the initialization (create) agent. The agent investigates and may ask
// the user a question instead of creating a case immediately; when it does, the
// simulator answers and the answer is injected as a thread reply that resumes
// the create agent, looping up to the persona's MaxAnswerTurns until the agent
// commits a case. The produced case is the artifact.
type ThreadInitial struct{}

// NewThreadInitial builds the driver.
func NewThreadInitial() *ThreadInitial { return &ThreadInitial{} }

// Kind implements WorkflowDriver.
func (*ThreadInitial) Kind() string { return "thread_mode_initial" }

// Run implements WorkflowDriver.
func (*ThreadInitial) Run(ctx context.Context, e *env.Env, sc *scenario.Scenario, sim evaltype.Simulator) (evaltype.Artifact, error) {
	logger := logging.From(ctx)
	if lang, err := i18n.ParseLang(sc.Meta.Language); err == nil && sc.Meta.Language != "" {
		ctx = i18n.ContextWithLang(ctx, lang)
	}

	wsID := e.Entry.Workspace.ID
	channel := e.MonitorChannel
	reporter := sc.Input.Reporter
	if reporter == "" {
		reporter = defaultReporter
	}

	now := time.Now().UTC()
	first := slackmodel.NewMessageFromData(runThreadTS, channel, "", evalTeamID, reporter, reporter, sc.Input.Text, runThreadTS, now, nil)

	logger.Info("eval turn: create (case initialization)", "scenario", sc.Meta.ID, "channel", channel)
	if err := e.AgentUC.HandleThreadCaseCreation(ctx, first, e.Entry); err != nil {
		return nil, goerr.Wrap(err, "thread case creation")
	}
	async.Wait()

	transcript := []evaltype.TurnRecord{{Turn: 1, Mode: "create", Input: sc.Input.Text}}

	for turn := 2; turn <= sc.Persona.MaxAnswerTurns+1; turn++ {
		session, err := e.Repo.Session().GetByThread(ctx, channel, runThreadTS)
		if err != nil {
			return nil, goerr.Wrap(err, "load session")
		}
		if session == nil || session.LastAction != model.SessionEndedWithQuestion {
			break // no pending question -> the agent created the case (or gave up)
		}

		questionText := lastThreadReply(e)
		q := evaltype.Question{
			Reason: questionText,
			Items:  []evaltype.QuestionItem{{ID: "q", Type: evaltype.QuestionFreeText, Text: questionText}},
		}
		ans, err := sim.Answer(ctx, q)
		if err != nil {
			return nil, goerr.Wrap(err, "user simulator answer")
		}
		answerText := joinAnswers(ans)

		// The case does NOT exist yet (the agent deferred creation to ask).
		// Inject the answer as a thread reply that resumes the create agent.
		replyTS := fmt.Sprintf("1700000000.%06d", 200+turn)
		reply := slackmodel.NewMessageFromData(replyTS, channel, runThreadTS, evalTeamID, reporter, reporter, answerText, replyTS, time.Now().UTC(), nil)

		logger.Info("eval turn: resume create (answer injected)", "scenario", sc.Meta.ID, "turn", turn)
		if err := e.AgentUC.ResumeThreadCaseCreation(ctx, reply, e.Entry); err != nil {
			return nil, goerr.Wrap(err, "resume thread case creation")
		}
		async.Wait()

		transcript = append(transcript, evaltype.TurnRecord{
			Turn:     turn,
			Mode:     "resume",
			Input:    answerText,
			Question: &q,
			Answer:   &ans,
		})
	}

	finalCase, err := e.Repo.Case().GetBySlackThread(ctx, wsID, channel, runThreadTS)
	if err != nil {
		return nil, goerr.Wrap(err, "load final case")
	}
	if finalCase == nil {
		return nil, errNoCase
	}

	logger.Info("eval: artifact ready", "scenario", sc.Meta.ID, "tool_calls", len(e.Recorder.Records()))
	return &evaltype.CaseArtifact{
		Case:       finalCase,
		Transcript: transcript,
		ToolCalls:  e.Recorder.Records(),
	}, nil
}

// lastThreadReply returns the text of the most recent thread reply the agent
// posted (questions and acks are posted via PostThreadReply).
func lastThreadReply(e *env.Env) string {
	posts := e.Slack.Posts()
	for i := len(posts) - 1; i >= 0; i-- {
		if posts[i].Kind == "thread_reply" {
			return posts[i].Text
		}
	}
	return ""
}

func joinAnswers(ans evaltype.Answers) string {
	out := ""
	for _, a := range ans.Items {
		if a.Value != "" {
			out += a.Value + " "
		}
		for _, v := range a.Values {
			out += v + " "
		}
	}
	if out == "" {
		return "(no answer)"
	}
	return out
}
