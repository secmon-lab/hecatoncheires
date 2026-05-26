package cli

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"
	gqlctrl "github.com/secmon-lab/hecatoncheires/pkg/controller/graphql"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

func TestClassifyError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"missing required → BAD_USER_INPUT", goerr.Wrap(model.ErrMissingRequired, "x"), gqlctrl.ErrCodeBadUserInput},
		{"invalid option ID → BAD_USER_INPUT", goerr.Wrap(model.ErrInvalidOptionID, "x"), gqlctrl.ErrCodeBadUserInput},
		{"invalid field type → BAD_USER_INPUT", goerr.Wrap(model.ErrInvalidFieldType, "x"), gqlctrl.ErrCodeBadUserInput},
		{"case not found → NOT_FOUND", goerr.Wrap(usecase.ErrCaseNotFound, "x"), gqlctrl.ErrCodeNotFound},
		{"action not found → NOT_FOUND", goerr.Wrap(usecase.ErrActionNotFound, "x"), gqlctrl.ErrCodeNotFound},
		{"workspace not found → NOT_FOUND", goerr.Wrap(model.ErrWorkspaceNotFound, "x"), gqlctrl.ErrCodeNotFound},
		{"access denied → FORBIDDEN", goerr.Wrap(usecase.ErrAccessDenied, "x"), gqlctrl.ErrCodeForbidden},
		{"already closed → CONFLICT", goerr.Wrap(usecase.ErrCaseAlreadyClosed, "x"), gqlctrl.ErrCodeConflict},
		{"missing required on submit → MISSING_REQUIRED_FIELDS", goerr.Wrap(usecase.ErrMissingRequiredOnSubmit, "x"), gqlctrl.ErrCodeMissingRequiredFields},
		{"draft title required → TITLE_REQUIRED", goerr.Wrap(usecase.ErrDraftTitleRequired, "x"), gqlctrl.ErrCodeTitleRequired},
		{"case not draft (domain) → INVALID_STATUS_TRANSITION", goerr.Wrap(model.ErrCaseNotDraft, "x"), gqlctrl.ErrCodeInvalidStatusTransition},
		{"case not draft (usecase) → INVALID_STATUS_TRANSITION", goerr.Wrap(usecase.ErrCaseNotDraft, "x"), gqlctrl.ErrCodeInvalidStatusTransition},
		{"field validation failed → FIELD_VALIDATION_FAILED", goerr.Wrap(usecase.ErrFieldValidationFailed, "x"), gqlctrl.ErrCodeFieldValidationFailed},
		{"activation failed → ACTIVATION_FAILED", goerr.Wrap(usecase.ErrActivationFailed, "x"), gqlctrl.ErrCodeActivationFailed},
		{"random error → untagged", goerr.New("boom"), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gt.String(t, classifyError(c.err)).Equal(c.want)
		})
	}
}

func TestStatusForExtensionCode(t *testing.T) {
	gt.Number(t, statusForExtensionCode(gqlctrl.ErrCodeBadUserInput)).Equal(http.StatusBadRequest)
	gt.Number(t, statusForExtensionCode(gqlctrl.ErrCodeNotFound)).Equal(http.StatusNotFound)
	gt.Number(t, statusForExtensionCode(gqlctrl.ErrCodeForbidden)).Equal(http.StatusForbidden)
	gt.Number(t, statusForExtensionCode(gqlctrl.ErrCodeConflict)).Equal(http.StatusConflict)
	gt.Number(t, statusForExtensionCode(gqlctrl.ErrCodeUnauthenticated)).Equal(http.StatusUnauthorized)
	gt.Number(t, statusForExtensionCode(gqlctrl.ErrCodeMissingRequiredFields)).Equal(http.StatusBadRequest)
	gt.Number(t, statusForExtensionCode(gqlctrl.ErrCodeTitleRequired)).Equal(http.StatusBadRequest)
	gt.Number(t, statusForExtensionCode(gqlctrl.ErrCodeFieldValidationFailed)).Equal(http.StatusBadRequest)
	gt.Number(t, statusForExtensionCode(gqlctrl.ErrCodeInvalidStatusTransition)).Equal(http.StatusConflict)
	gt.Number(t, statusForExtensionCode(gqlctrl.ErrCodeActivationFailed)).Equal(0) // server fault → 500 fallback
	gt.Number(t, statusForExtensionCode("")).Equal(0)
	gt.Number(t, statusForExtensionCode("WHATEVER")).Equal(0)
}

func TestHTTPStatusForGraphQLErrors(t *testing.T) {
	mk := func(codes ...string) []gqlErrorEnvelope {
		out := make([]gqlErrorEnvelope, len(codes))
		for i, c := range codes {
			out[i].Extensions.Code = c
		}
		return out
	}

	gt.Number(t, httpStatusForGraphQLErrors(mk("BAD_USER_INPUT"))).Equal(http.StatusBadRequest)
	gt.Number(t, httpStatusForGraphQLErrors(mk("NOT_FOUND"))).Equal(http.StatusNotFound)

	// One untagged error in the batch → escalate to 500
	gt.Number(t, httpStatusForGraphQLErrors(mk("BAD_USER_INPUT", ""))).Equal(http.StatusInternalServerError)

	// Highest 4xx wins when all are tagged
	gt.Number(t, httpStatusForGraphQLErrors(mk("BAD_USER_INPUT", "FORBIDDEN"))).Equal(http.StatusForbidden)

	// Empty list → 500 fallback
	gt.Number(t, httpStatusForGraphQLErrors(mk())).Equal(http.StatusInternalServerError)
}

func TestGraphqlErrorStatusMiddleware_MapsClientErrorsTo4xx(t *testing.T) {
	cases := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{
			name:       "BAD_USER_INPUT → 400",
			body:       `{"data":null,"errors":[{"message":"required field is missing","extensions":{"code":"BAD_USER_INPUT"}}]}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "NOT_FOUND → 404",
			body:       `{"data":null,"errors":[{"message":"case not found","extensions":{"code":"NOT_FOUND"}}]}`,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "FORBIDDEN → 403",
			body:       `{"data":null,"errors":[{"message":"access denied","extensions":{"code":"FORBIDDEN"}}]}`,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "untagged error → 500",
			body:       `{"data":null,"errors":[{"message":"boom"}]}`,
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "no errors → 200 passthrough",
			body:       `{"data":{"hello":"world"}}`,
			wantStatus: http.StatusOK,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := graphqlErrorStatusMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(c.body))
			}))
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/graphql", nil)
			h.ServeHTTP(rec, req)
			gt.Number(t, rec.Code).Equal(c.wantStatus)
		})
	}
}
