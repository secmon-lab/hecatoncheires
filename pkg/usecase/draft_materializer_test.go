package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/m-mizutani/gollem"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

func newTestDraft() *model.CaseDraft {
	d := model.NewCaseDraft(time.Now().UTC(), "U_alice")
	d.MentionText = "please open a case for the suspicious login"
	d.Source = model.DraftSource{TeamID: "T1", ChannelID: "C1", MentionTS: "1700000010.000000"}
	d.RawMessages = []model.DraftMessage{
		{UserID: "U1", Text: "we saw 5 failed logins from a new ASN", TS: "1700000001.000000"},
		{UserID: "U2", Text: "okta MFA also bypassed", TS: "1700000002.000000"},
	}
	return d
}

func newRichSchema() *config.FieldSchema {
	return &config.FieldSchema{
		Fields: []config.FieldDefinition{
			{ID: "severity", Name: "Severity", Type: types.FieldTypeSelect, Required: true,
				Options: []config.FieldOption{{ID: "low"}, {ID: "med"}, {ID: "high"}}},
			{ID: "tags", Name: "Tags", Type: types.FieldTypeMultiSelect,
				Options: []config.FieldOption{{ID: "phishing"}, {ID: "okta"}, {ID: "abuse"}}},
			{ID: "owner", Name: "Owner", Type: types.FieldTypeUser},
			{ID: "watchers", Name: "Watchers", Type: types.FieldTypeMultiUser},
			{ID: "count", Name: "Count", Type: types.FieldTypeNumber},
			{ID: "summary", Name: "Internal Summary", Type: types.FieldTypeText},
			{ID: "evidence_url", Name: "Evidence URL", Type: types.FieldTypeURL},
			{ID: "detected_at", Name: "Detected At", Type: types.FieldTypeDate},
		},
	}
}

func newRichCtx() usecase.MaterializeContext {
	return usecase.MaterializeContext{
		Workspace: &model.WorkspaceEntry{
			Workspace:   model.Workspace{ID: "ws-rich", Name: "Rich"},
			FieldSchema: newRichSchema(),
		},
		EstimationReason: "test",
	}
}

func TestMaterialize_StructuredJSON(t *testing.T) {
	llm := &mockLLMClient{
		newSessionFn: func(ctx context.Context, options ...gollem.SessionOption) (gollem.Session, error) {
			return &mockLLMSession{
				generateContentFn: func(_ context.Context, _ ...gollem.Input) (*gollem.Response, error) {
					return &gollem.Response{Texts: []string{`{
                        "title": "Suspicious login spike",
                        "description": "Multiple failed sign-ins from a new ASN with MFA bypass.",
                        "custom_fields": {
                            "severity": "high",
                            "tags": ["okta", "abuse"],
                            "owner": "U_alice",
                            "watchers": ["U1", "U2"],
                            "count": 5,
                            "summary": "auto-generated summary",
                            "evidence_url": "https://wiki.example/evidence/1",
                            "detected_at": "2026-05-02"
                        }
                    }`}}, nil
				},
			}, nil
		},
	}

	m := usecase.NewDraftMaterializer(llm)
	mat, err := m.Materialize(context.Background(), newTestDraft(), newRichCtx())
	gt.NoError(t, err).Required()
	gt.Value(t, mat.Title).Equal("Suspicious login spike")
	gt.Value(t, mat.Description).Equal("Multiple failed sign-ins from a new ASN with MFA bypass.")

	gt.Value(t, mat.CustomFieldValues["severity"].Value).Equal("high")
	gt.Value(t, mat.CustomFieldValues["severity"].Type).Equal(types.FieldTypeSelect)

	tags, ok := mat.CustomFieldValues["tags"].Value.([]string)
	gt.Bool(t, ok).True()
	gt.Array(t, tags).Length(2)
	gt.Value(t, tags[0]).Equal("okta")

	gt.Value(t, mat.CustomFieldValues["owner"].Value).Equal("U_alice")

	watchers := mat.CustomFieldValues["watchers"].Value.([]string)
	gt.Array(t, watchers).Length(2)

	count, ok := mat.CustomFieldValues["count"].Value.(float64)
	gt.Bool(t, ok).True()
	gt.Number(t, count).Equal(5)

	gt.Value(t, mat.CustomFieldValues["evidence_url"].Value).Equal("https://wiki.example/evidence/1")
	gt.Value(t, mat.CustomFieldValues["detected_at"].Value).Equal("2026-05-02")

	gt.Bool(t, mat.GeneratedAt.IsZero()).False()
}

func TestMaterialize_OmitsFieldsAIDidNotFill(t *testing.T) {
	llm := &mockLLMClient{
		newSessionFn: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			return &mockLLMSession{
				generateContentFn: func(_ context.Context, _ ...gollem.Input) (*gollem.Response, error) {
					return &gollem.Response{Texts: []string{`{"title":"t","description":"d","custom_fields":{}}`}}, nil
				},
			}, nil
		},
	}
	m := usecase.NewDraftMaterializer(llm)
	mat, err := m.Materialize(context.Background(), newTestDraft(), newRichCtx())
	gt.NoError(t, err).Required()
	gt.Value(t, mat.Title).Equal("t")
	gt.Value(t, mat.Description).Equal("d")
	gt.Map(t, mat.CustomFieldValues).Length(0)
}

func TestMaterialize_DropsTypeMismatchedFields(t *testing.T) {
	llm := &mockLLMClient{
		newSessionFn: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			return &mockLLMSession{
				generateContentFn: func(_ context.Context, _ ...gollem.Input) (*gollem.Response, error) {
					// count is a number field but AI returned a string; severity is a string-good.
					return &gollem.Response{Texts: []string{`{"title":"t","description":"d","custom_fields":{"severity":"low","count":"five","tags":"oops-not-an-array"}}`}}, nil
				},
			}, nil
		},
	}
	m := usecase.NewDraftMaterializer(llm)
	mat, err := m.Materialize(context.Background(), newTestDraft(), newRichCtx())
	gt.NoError(t, err).Required()
	gt.Value(t, mat.CustomFieldValues["severity"].Value).Equal("low")
	_, hasCount := mat.CustomFieldValues["count"]
	gt.Bool(t, hasCount).False()
	_, hasTags := mat.CustomFieldValues["tags"]
	gt.Bool(t, hasTags).False()
}

func TestMaterialize_DropsUnknownFields(t *testing.T) {
	llm := &mockLLMClient{
		newSessionFn: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			return &mockLLMSession{
				generateContentFn: func(_ context.Context, _ ...gollem.Input) (*gollem.Response, error) {
					return &gollem.Response{Texts: []string{`{"title":"t","description":"d","custom_fields":{"hallucinated":"value","severity":"low"}}`}}, nil
				},
			}, nil
		},
	}
	m := usecase.NewDraftMaterializer(llm)
	mat, err := m.Materialize(context.Background(), newTestDraft(), newRichCtx())
	gt.NoError(t, err).Required()
	gt.Value(t, mat.CustomFieldValues["severity"].Value).Equal("low")
	_, has := mat.CustomFieldValues["hallucinated"]
	gt.Bool(t, has).False()
}

func TestMaterialize_PropagatesLLMError(t *testing.T) {
	llm := &mockLLMClient{
		newSessionFn: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			return &mockLLMSession{
				generateContentFn: func(_ context.Context, _ ...gollem.Input) (*gollem.Response, error) {
					return nil, errors.New("LLM is down")
				},
			}, nil
		},
	}
	m := usecase.NewDraftMaterializer(llm)
	mat, err := m.Materialize(context.Background(), newTestDraft(), newRichCtx())
	gt.Value(t, err).NotNil()
	gt.Value(t, mat).Nil()
}

func TestMaterialize_PropagatesSessionError(t *testing.T) {
	llm := &mockLLMClient{
		newSessionFn: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			return nil, errors.New("no session for you")
		},
	}
	m := usecase.NewDraftMaterializer(llm)
	mat, err := m.Materialize(context.Background(), newTestDraft(), newRichCtx())
	gt.Value(t, err).NotNil()
	gt.Value(t, mat).Nil()
}

func TestMaterialize_PropagatesEmptyResponse(t *testing.T) {
	llm := &mockLLMClient{
		newSessionFn: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			return &mockLLMSession{
				generateContentFn: func(_ context.Context, _ ...gollem.Input) (*gollem.Response, error) {
					return &gollem.Response{Texts: nil}, nil
				},
			}, nil
		},
	}
	m := usecase.NewDraftMaterializer(llm)
	mat, err := m.Materialize(context.Background(), newTestDraft(), newRichCtx())
	gt.Value(t, err).NotNil()
	gt.Value(t, mat).Nil()
}

func TestMaterialize_PropagatesInvalidJSON(t *testing.T) {
	llm := &mockLLMClient{
		newSessionFn: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			return &mockLLMSession{
				generateContentFn: func(_ context.Context, _ ...gollem.Input) (*gollem.Response, error) {
					return &gollem.Response{Texts: []string{`<<not-json>>`}}, nil
				},
			}, nil
		},
	}
	m := usecase.NewDraftMaterializer(llm)
	mat, err := m.Materialize(context.Background(), newTestDraft(), newRichCtx())
	gt.Value(t, err).NotNil()
	gt.Value(t, mat).Nil()
}

func TestMaterialize_NilArgsRejected(t *testing.T) {
	m := usecase.NewDraftMaterializer(&mockLLMClient{})

	_, err := m.Materialize(context.Background(), nil, newRichCtx())
	gt.Value(t, err).NotNil().Required()

	_, err = m.Materialize(context.Background(), newTestDraft(), usecase.MaterializeContext{})
	gt.Value(t, err).NotNil().Required()
}
