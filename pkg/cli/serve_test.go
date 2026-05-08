package cli

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

func TestClassifyError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"missing required → BAD_USER_INPUT", goerr.Wrap(model.ErrMissingRequired, "x"), "BAD_USER_INPUT"},
		{"invalid option ID → BAD_USER_INPUT", goerr.Wrap(model.ErrInvalidOptionID, "x"), "BAD_USER_INPUT"},
		{"invalid field type → BAD_USER_INPUT", goerr.Wrap(model.ErrInvalidFieldType, "x"), "BAD_USER_INPUT"},
		{"case not found → NOT_FOUND", goerr.Wrap(usecase.ErrCaseNotFound, "x"), "NOT_FOUND"},
		{"action not found → NOT_FOUND", goerr.Wrap(usecase.ErrActionNotFound, "x"), "NOT_FOUND"},
		{"workspace not found → NOT_FOUND", goerr.Wrap(model.ErrWorkspaceNotFound, "x"), "NOT_FOUND"},
		{"access denied → FORBIDDEN", goerr.Wrap(usecase.ErrAccessDenied, "x"), "FORBIDDEN"},
		{"already closed → CONFLICT", goerr.Wrap(usecase.ErrCaseAlreadyClosed, "x"), "CONFLICT"},
		{"random error → untagged", goerr.New("boom"), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gt.String(t, classifyError(c.err)).Equal(c.want)
		})
	}
}

func TestStatusForExtensionCode(t *testing.T) {
	gt.Number(t, statusForExtensionCode("BAD_USER_INPUT")).Equal(http.StatusBadRequest)
	gt.Number(t, statusForExtensionCode("NOT_FOUND")).Equal(http.StatusNotFound)
	gt.Number(t, statusForExtensionCode("FORBIDDEN")).Equal(http.StatusForbidden)
	gt.Number(t, statusForExtensionCode("CONFLICT")).Equal(http.StatusConflict)
	gt.Number(t, statusForExtensionCode("UNAUTHENTICATED")).Equal(http.StatusUnauthorized)
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
