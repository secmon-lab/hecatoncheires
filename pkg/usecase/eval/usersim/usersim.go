// Package usersim implements evaltype.Simulator: a simulated end-user that
// answers the agent's clarification questions, drawing on the scenario persona
// (who they are + what they know). Answers are produced by an LLM so free-text
// items get realistic prose; select / multi_select items are constrained to the
// offered options.
package usersim

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/evaltype"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/scenario"
)

// Simulator answers agent questions as the configured persona.
type Simulator struct {
	completer evaltype.Completer
	persona   scenario.Persona
	language  string
}

var _ evaltype.Simulator = (*Simulator)(nil)

// New builds a Simulator. language is the conversation language the answers are
// written in (the scenario's meta.language); empty leaves it to the model.
func New(completer evaltype.Completer, persona scenario.Persona, language string) *Simulator {
	return &Simulator{completer: completer, persona: persona, language: language}
}

// Answer produces replies to every item of the question.
func (s *Simulator) Answer(ctx context.Context, q evaltype.Question) (evaltype.Answers, error) {
	raw, err := s.completer.Complete(ctx, s.systemPrompt(), buildUserPrompt(q), answerSchema())
	if err != nil {
		return evaltype.Answers{}, goerr.Wrap(err, "usersim completion")
	}

	var parsed struct {
		Answers []struct {
			ID     string   `json:"id"`
			Value  string   `json:"value"`
			Values []string `json:"values"`
		} `json:"answers"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return evaltype.Answers{}, goerr.Wrap(err, "decode usersim answers", goerr.V("raw_len", len(raw)))
	}

	// Index questions for option-constraint enforcement.
	byID := make(map[string]evaltype.QuestionItem, len(q.Items))
	for _, it := range q.Items {
		byID[it.ID] = it
	}

	out := evaltype.Answers{Items: make([]evaltype.Answer, 0, len(parsed.Answers))}
	for _, a := range parsed.Answers {
		ans := evaltype.Answer{ID: a.ID, Value: a.Value, Values: a.Values}
		if it, ok := byID[a.ID]; ok {
			ans = constrain(it, ans)
		}
		out.Items = append(out.Items, ans)
	}
	return out, nil
}

// constrain forces select / multi_select answers into the offered options so a
// hallucinated value never reaches the agent. free_text passes through.
func constrain(it evaltype.QuestionItem, ans evaltype.Answer) evaltype.Answer {
	switch it.Type {
	case evaltype.QuestionSelect:
		if len(it.Options) > 0 && !slices.Contains(it.Options, ans.Value) {
			ans.Value = it.Options[0]
		}
		ans.Values = nil
	case evaltype.QuestionMultiSelect:
		kept := ans.Values[:0]
		for _, v := range ans.Values {
			if slices.Contains(it.Options, v) {
				kept = append(kept, v)
			}
		}
		if len(kept) == 0 && len(it.Options) > 0 {
			kept = append(kept, it.Options[0])
		}
		ans.Values = kept
		ans.Value = ""
	case evaltype.QuestionFreeText:
		ans.Values = nil
	}
	return ans
}

func (s *Simulator) systemPrompt() string {
	var b strings.Builder
	b.WriteString("You are role-playing a Slack user who reported an issue and is now being asked clarifying questions by an assistant. Answer as that user, using only what the persona would plausibly know.\n")
	if s.persona.Description != "" {
		fmt.Fprintf(&b, "\n# Who you are\n%s\n", s.persona.Description)
	}
	if s.persona.Knowledge != "" {
		fmt.Fprintf(&b, "\n# What you know\n%s\n", s.persona.Knowledge)
	}
	b.WriteString("\n# Rules\n")
	b.WriteString("- For select / multi_select questions, choose only from the provided option ids.\n")
	b.WriteString("- For free_text questions, answer briefly and concretely.\n")
	b.WriteString("- If you do not know something, give a plausible best guess consistent with the persona.\n")
	if s.language != "" {
		fmt.Fprintf(&b, "- Write free-text answers in %s.\n", s.language)
	}
	return b.String()
}

func buildUserPrompt(q evaltype.Question) string {
	var b strings.Builder
	if q.Reason != "" {
		fmt.Fprintf(&b, "The assistant asks: %s\n\n", q.Reason)
	}
	b.WriteString("Questions:\n")
	for _, it := range q.Items {
		fmt.Fprintf(&b, "- id=%s type=%s: %s", it.ID, it.Type, it.Text)
		if len(it.Options) > 0 {
			fmt.Fprintf(&b, " options=[%s]", strings.Join(it.Options, ", "))
		}
		b.WriteString("\n")
	}
	b.WriteString("\nReturn an answer for every question id.")
	return b.String()
}

func answerSchema() *gollem.Parameter {
	return &gollem.Parameter{
		Type:        gollem.TypeObject,
		Description: "Answers to each question item.",
		Properties: map[string]*gollem.Parameter{
			"answers": {
				Type:        gollem.TypeArray,
				Description: "One entry per question id.",
				Items: &gollem.Parameter{
					Type: gollem.TypeObject,
					Properties: map[string]*gollem.Parameter{
						"id":     {Type: gollem.TypeString, Description: "The question item id being answered.", Required: true},
						"value":  {Type: gollem.TypeString, Description: "Answer for select / free_text (a single value)."},
						"values": {Type: gollem.TypeArray, Description: "Answer for multi_select (chosen option ids).", Items: &gollem.Parameter{Type: gollem.TypeString}},
					},
				},
			},
		},
	}
}
