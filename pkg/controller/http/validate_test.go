package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"

	httpctrl "github.com/secmon-lab/hecatoncheires/pkg/controller/http"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
)

// stubDBChecker records the documents it was handed and returns a canned answer.
type stubDBChecker struct {
	docs   []httpctrl.ConfigDocument
	result *usecase.ValidationResult
	err    error
}

func (s *stubDBChecker) CheckDBConsistency(_ context.Context, docs []httpctrl.ConfigDocument) (*usecase.ValidationResult, error) {
	s.docs = docs
	return s.result, s.err
}

// dbCheckBody is the response shape the endpoint promises. It is spelled out
// here rather than reused from the handler so a rename of a JSON key fails the
// test instead of silently changing the contract.
type dbCheckBody struct {
	HasIssues  bool  `json:"has_issues"`
	TotalCount int64 `json:"total_count"`
	Issues     []struct {
		WorkspaceID string `json:"workspace_id"`
		Kind        string `json:"kind"`
		FieldID     string `json:"field_id"`
		Count       int64  `json:"count"`
		Sample      struct {
			Kind     string `json:"kind"`
			CaseID   int64  `json:"case_id"`
			ActionID int64  `json:"action_id"`
			MemoID   string `json:"memo_id"`
		} `json:"sample"`
		Expected string `json:"expected"`
		Actual   string `json:"actual"`
		Message  string `json:"message"`
	} `json:"issues"`
}

func postDBCheck(t *testing.T, checker httpctrl.DBConsistencyChecker, contentType, body string) *httptest.ResponseRecorder {
	t.Helper()
	h := httpctrl.NewDBCheckHandler(checker)
	req := httptest.NewRequest("POST", "/api/validate/db", strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestDBCheckHandler_RawBodyIsOneDocument(t *testing.T) {
	checker := &stubDBChecker{result: &usecase.ValidationResult{}}
	rec := postDBCheck(t, checker, "application/toml", "[workspace]\nid = \"risk\"\n")

	gt.Number(t, rec.Code).Equal(200)
	gt.String(t, rec.Header().Get("Content-Type")).Equal("application/json")

	gt.Array(t, checker.docs).Length(1).Required()
	gt.String(t, checker.docs[0].Name).Equal("request body")
	gt.String(t, string(checker.docs[0].Data)).Equal("[workspace]\nid = \"risk\"\n")

	var got dbCheckBody
	gt.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got)).Required()
	gt.Bool(t, got.HasIssues).False()
	gt.Number(t, got.TotalCount).Equal(int64(0))
	gt.Array(t, got.Issues).Length(0)
	// A clean deployment must report an empty list, never JSON null.
	gt.String(t, rec.Body.String()).Contains(`"issues":[]`)
}

func TestDBCheckHandler_MultipartCarriesEveryDocumentInOrder(t *testing.T) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	first, err := mw.CreateFormFile("config", "risk.toml")
	gt.NoError(t, err).Required()
	_, err = first.Write([]byte("[workspace]\nid = \"risk\"\n"))
	gt.NoError(t, err).Required()
	second, err := mw.CreateFormFile("config", "task.toml")
	gt.NoError(t, err).Required()
	_, err = second.Write([]byte("[workspace]\nid = \"task\"\n"))
	gt.NoError(t, err).Required()
	gt.NoError(t, mw.Close()).Required()

	checker := &stubDBChecker{result: &usecase.ValidationResult{}}
	rec := postDBCheck(t, checker, mw.FormDataContentType(), buf.String())

	gt.Number(t, rec.Code).Equal(200)
	gt.Array(t, checker.docs).Length(2).Required()
	gt.String(t, checker.docs[0].Name).Equal("risk.toml")
	gt.String(t, string(checker.docs[0].Data)).Equal("[workspace]\nid = \"risk\"\n")
	gt.String(t, checker.docs[1].Name).Equal("task.toml")
	gt.String(t, string(checker.docs[1].Data)).Equal("[workspace]\nid = \"task\"\n")
}

func TestDBCheckHandler_MultipartFieldNameIdentifiesUnnamedPart(t *testing.T) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormField("workspace-config")
	gt.NoError(t, err).Required()
	_, err = part.Write([]byte("[workspace]\nid = \"risk\"\n"))
	gt.NoError(t, err).Required()
	gt.NoError(t, mw.Close()).Required()

	checker := &stubDBChecker{result: &usecase.ValidationResult{}}
	rec := postDBCheck(t, checker, mw.FormDataContentType(), buf.String())

	gt.Number(t, rec.Code).Equal(200)
	gt.Array(t, checker.docs).Length(1).Required()
	gt.String(t, checker.docs[0].Name).Equal("workspace-config")
}

func TestDBCheckHandler_ReportsIssues(t *testing.T) {
	checker := &stubDBChecker{result: &usecase.ValidationResult{
		Issues: []usecase.ValidationIssue{
			{
				WorkspaceID: "risk",
				Kind:        usecase.IssueKindBoardStatus,
				Count:       7,
				Sample:      usecase.ValidationTarget{Kind: usecase.TargetKindCase, CaseID: 42},
				Expected:    "one of the configured case status ids",
				Actual:      "triage",
				Message:     "board status is not defined in the workspace configuration",
			},
			{
				WorkspaceID: "risk",
				Kind:        usecase.IssueKindFieldValue,
				FieldID:     "severity",
				Count:       3,
				Sample: usecase.ValidationTarget{
					Kind:   usecase.TargetKindMemo,
					CaseID: 9,
					MemoID: model.MemoID("memo-1"),
				},
				Expected: "select",
				Actual:   "sev0",
				Message:  "value is not a configured option",
			},
			{
				WorkspaceID: "task",
				Kind:        usecase.IssueKindActionStatus,
				Count:       1,
				Sample: usecase.ValidationTarget{
					Kind:     usecase.TargetKindAction,
					CaseID:   3,
					ActionID: 11,
				},
				Expected: "one of the configured action status ids",
				Actual:   "OBSOLETE",
				Message:  "action status is not defined in the workspace configuration",
			},
		},
	}}

	rec := postDBCheck(t, checker, "application/toml", "[workspace]\nid = \"risk\"\n")
	gt.Number(t, rec.Code).Equal(200)

	var got dbCheckBody
	gt.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got)).Required()
	gt.Bool(t, got.HasIssues).True()
	gt.Number(t, got.TotalCount).Equal(int64(11))
	gt.Array(t, got.Issues).Length(3).Required()

	gt.String(t, got.Issues[0].WorkspaceID).Equal("risk")
	gt.String(t, got.Issues[0].Kind).Equal("board_status_invalid")
	gt.String(t, got.Issues[0].FieldID).Equal("")
	gt.Number(t, got.Issues[0].Count).Equal(int64(7))
	gt.String(t, got.Issues[0].Sample.Kind).Equal("case")
	gt.Number(t, got.Issues[0].Sample.CaseID).Equal(int64(42))
	gt.Number(t, got.Issues[0].Sample.ActionID).Equal(int64(0))
	gt.String(t, got.Issues[0].Sample.MemoID).Equal("")
	gt.String(t, got.Issues[0].Expected).Equal("one of the configured case status ids")
	gt.String(t, got.Issues[0].Actual).Equal("triage")
	gt.String(t, got.Issues[0].Message).Equal("board status is not defined in the workspace configuration")

	gt.String(t, got.Issues[1].Kind).Equal("field_value")
	gt.String(t, got.Issues[1].FieldID).Equal("severity")
	gt.Number(t, got.Issues[1].Count).Equal(int64(3))
	gt.String(t, got.Issues[1].Sample.Kind).Equal("memo")
	gt.Number(t, got.Issues[1].Sample.CaseID).Equal(int64(9))
	gt.String(t, got.Issues[1].Sample.MemoID).Equal("memo-1")
	gt.String(t, got.Issues[1].Expected).Equal("select")
	gt.String(t, got.Issues[1].Actual).Equal("sev0")

	gt.String(t, got.Issues[2].WorkspaceID).Equal("task")
	gt.String(t, got.Issues[2].Kind).Equal("action_status_invalid")
	gt.String(t, got.Issues[2].Sample.Kind).Equal("action")
	gt.Number(t, got.Issues[2].Sample.CaseID).Equal(int64(3))
	gt.Number(t, got.Issues[2].Sample.ActionID).Equal(int64(11))
	gt.String(t, got.Issues[2].Actual).Equal("OBSOLETE")
}

func TestDBCheckHandler_EmptyBodyIsRejected(t *testing.T) {
	checker := &stubDBChecker{result: &usecase.ValidationResult{}}
	rec := postDBCheck(t, checker, "application/toml", "")

	gt.Number(t, rec.Code).Equal(400)
	gt.Array(t, checker.docs).Length(0)
}

func TestDBCheckHandler_EmptyMultipartIsRejected(t *testing.T) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	gt.NoError(t, mw.Close()).Required()

	checker := &stubDBChecker{result: &usecase.ValidationResult{}}
	rec := postDBCheck(t, checker, mw.FormDataContentType(), buf.String())

	gt.Number(t, rec.Code).Equal(400)
	gt.Array(t, checker.docs).Length(0)
}

func TestDBCheckHandler_OversizedBodyIsRejected(t *testing.T) {
	checker := &stubDBChecker{result: &usecase.ValidationResult{}}
	rec := postDBCheck(t, checker, "application/toml", strings.Repeat("a", (1<<20)+1))

	gt.Number(t, rec.Code).Equal(413)
	gt.Array(t, checker.docs).Length(0)
}

// TestDBCheckHandler_OversizedMultipartIsRejected covers the size limit on the
// multipart path as well: the limit is applied to the request body before the
// multipart reader sees it, so a large part must be refused the same way a large
// raw body is, without the checker ever running.
func TestDBCheckHandler_OversizedMultipartIsRejected(t *testing.T) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("config", "huge.toml")
	gt.NoError(t, err).Required()
	_, err = part.Write([]byte(strings.Repeat("a", (1<<20)+1)))
	gt.NoError(t, err).Required()
	gt.NoError(t, mw.Close()).Required()

	checker := &stubDBChecker{result: &usecase.ValidationResult{}}
	rec := postDBCheck(t, checker, mw.FormDataContentType(), buf.String())

	gt.Number(t, rec.Code).Equal(413)
	gt.Array(t, checker.docs).Length(0)
}

func TestDBCheckHandler_InvalidConfigIsClientError(t *testing.T) {
	checker := &stubDBChecker{
		err: goerr.Join(httpctrl.ErrInvalidConfigDocument, goerr.New("duplicate workspace ID")),
	}
	rec := postDBCheck(t, checker, "application/toml", "[workspace]\nid = \"risk\"\n")

	gt.Number(t, rec.Code).Equal(400)
	gt.String(t, rec.Body.String()).Contains("duplicate workspace ID")
}

// TestErrInvalidConfigDocument_IsBenign pins that a rejected configuration is
// treated as ordinary client traffic rather than a server fault: errutil.Handle
// demotes a benign error to Info and keeps it out of Sentry. The tag has to sit
// on the sentinel itself — goerr.HasTag resolves an error containing a
// goerr.Join result through Errors.HasTag, which inspects only the joined
// errors, so tagging an outer wrapper of the joined error has no effect. The
// second assertion is the one that catches that regression.
func TestErrInvalidConfigDocument_IsBenign(t *testing.T) {
	gt.Bool(t, goerr.HasTag(httpctrl.ErrInvalidConfigDocument, errutil.TagBenign)).True()

	joined := goerr.Join(httpctrl.ErrInvalidConfigDocument, goerr.New("duplicate workspace ID"))
	gt.Bool(t, goerr.HasTag(joined, errutil.TagBenign)).True()
	gt.Bool(t, errors.Is(joined, httpctrl.ErrInvalidConfigDocument)).True()
}

func TestDBCheckHandler_CheckFailureIsServerError(t *testing.T) {
	checker := &stubDBChecker{err: goerr.New("firestore unavailable")}
	rec := postDBCheck(t, checker, "application/toml", "[workspace]\nid = \"risk\"\n")

	gt.Number(t, rec.Code).Equal(500)
}

func TestDBCheckHandler_FailsWhenCheckerNil(t *testing.T) {
	rec := postDBCheck(t, nil, "application/toml", "[workspace]\nid = \"risk\"\n")
	gt.Number(t, rec.Code).Equal(503)
}
