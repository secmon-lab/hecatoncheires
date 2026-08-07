package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/m-mizutani/goerr/v2"

	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/safe"
)

// dbCheckMaxBodySize bounds the workspace configuration one request may carry.
// A workspace TOML document is a few KiB, so 1 MiB covers a whole config
// directory while keeping an unauthenticated endpoint from buffering arbitrary
// input.
const dbCheckMaxBodySize = 1 << 20

// dbCheckDefaultDocumentName labels a document that arrived as the raw request
// body, which carries no file name.
const dbCheckDefaultDocumentName = "request body"

// ErrInvalidConfigDocument marks a DB check failure caused by the configuration
// in the request rather than by the server. The DBConsistencyChecker
// implementation wraps parse and validation failures with it so this handler can
// answer 400 instead of 500.
//
// The benign tag lives on the sentinel rather than being applied where the
// response is written: goerr.HasTag resolves an error that contains a
// goerr.Join result through Errors.HasTag, which inspects only the joined
// errors, so a tag attached to an outer wrapper of a joined error is never
// found. As one of the joined errors, the sentinel is.
var ErrInvalidConfigDocument = goerr.New("invalid workspace configuration document",
	goerr.T(errutil.TagBenign))

// ConfigDocument is one workspace configuration (TOML) document taken from a
// request.
type ConfigDocument struct {
	// Name identifies the document in error messages — a multipart part's file
	// name, or dbCheckDefaultDocumentName for a raw body.
	Name string
	// Data is the raw TOML document.
	Data []byte
}

// DBConsistencyChecker checks the persisted data of every workspace defined by
// the supplied configuration documents. The implementation lives in pkg/cli,
// which owns both TOML parsing and the usecase; a configuration-level failure
// MUST be wrapped with ErrInvalidConfigDocument.
type DBConsistencyChecker interface {
	CheckDBConsistency(ctx context.Context, docs []ConfigDocument) (*usecase.ValidationResult, error)
}

// DBCheckHandler exposes POST /api/validate/db: the `validate --check-db`
// consistency report over HTTP, against workspace configuration supplied in the
// request instead of the configuration this process was started with.
//
// The endpoint is intentionally unauthenticated, on the same assumption as
// POST /hooks/tick: the deployment fronts it with IAP / internal-network policy.
// The response body names Case, Action and Memo ids and echoes offending field
// values, so exposing it publicly would leak workspace data.
//
// Unlike the tick hook this responds synchronously — the report IS the result,
// so it cannot be dispatched to the background. A workspace with many Cases
// takes as long as the scan takes; call it from tooling that tolerates that,
// not from a browser request path.
type DBCheckHandler struct {
	checker DBConsistencyChecker
}

// NewDBCheckHandler builds the handler.
func NewDBCheckHandler(checker DBConsistencyChecker) *DBCheckHandler {
	return &DBCheckHandler{checker: checker}
}

// dbCheckResponse is the JSON body of a successful check. Issues is never null:
// a clean deployment reports an empty list.
type dbCheckResponse struct {
	HasIssues  bool           `json:"has_issues"`
	TotalCount int64          `json:"total_count"`
	Issues     []dbCheckIssue `json:"issues"`
}

// dbCheckIssue is one group of identical inconsistencies. Count is how many
// entities hit it; Sample describes the lowest-ordered one.
type dbCheckIssue struct {
	WorkspaceID string        `json:"workspace_id"`
	Kind        string        `json:"kind"`
	FieldID     string        `json:"field_id"`
	Count       int64         `json:"count"`
	Sample      dbCheckTarget `json:"sample"`
	Expected    string        `json:"expected"`
	Actual      string        `json:"actual"`
	Message     string        `json:"message"`
}

// dbCheckTarget identifies the sampled entity. ActionID / MemoID are omitted for
// the kinds they do not apply to.
type dbCheckTarget struct {
	Kind     string `json:"kind"`
	CaseID   int64  `json:"case_id"`
	ActionID int64  `json:"action_id,omitempty"`
	MemoID   string `json:"memo_id,omitempty"`
}

// ServeHTTP implements http.Handler.
func (h *DBCheckHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if h == nil || h.checker == nil {
		http.Error(w, "db consistency checker not configured", http.StatusServiceUnavailable)
		return
	}

	docs, err := readConfigDocuments(ctx, w, r)
	if err != nil {
		status := http.StatusBadRequest
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		// A malformed or oversized request is normal traffic on an operator
		// endpoint, so the reason goes back to the caller but not to Sentry.
		errutil.HandleHTTP(ctx, w, goerr.With(err, goerr.T(errutil.TagBenign)), status)
		return
	}

	result, err := h.checker.CheckDBConsistency(ctx, docs)
	if err != nil {
		// A rejected configuration is the client's mistake and must not be
		// reported as a server fault; anything else (a repository failure mid
		// scan) is.
		if errors.Is(err, ErrInvalidConfigDocument) {
			// The sentinel already carries the benign tag, so this is logged at
			// Info level and skipped by Sentry.
			errutil.HandleHTTP(ctx, w, err, http.StatusBadRequest)
			return
		}
		// The failure detail (which can name the storage project / database) is
		// recorded, not returned: this endpoint is unauthenticated, so its
		// response body stays free of infrastructure identifiers.
		errutil.Handle(ctx, goerr.Wrap(err, "db consistency check failed"), "db consistency check failed")
		http.Error(w, "db consistency check failed", http.StatusInternalServerError)
		return
	}

	resp := dbCheckResponse{
		HasIssues:  result.HasIssues(),
		TotalCount: result.TotalCount(),
		Issues:     make([]dbCheckIssue, 0, len(result.Issues)),
	}
	for _, issue := range result.Issues {
		resp.Issues = append(resp.Issues, dbCheckIssue{
			WorkspaceID: issue.WorkspaceID,
			Kind:        string(issue.Kind),
			FieldID:     issue.FieldID,
			Count:       issue.Count,
			Sample: dbCheckTarget{
				Kind:     string(issue.Sample.Kind),
				CaseID:   issue.Sample.CaseID,
				ActionID: issue.Sample.ActionID,
				MemoID:   string(issue.Sample.MemoID),
			},
			Expected: issue.Expected,
			Actual:   issue.Actual,
			Message:  issue.Message,
		})
	}

	data, err := json.Marshal(resp)
	if err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "failed to marshal db check response"), "failed to marshal db check response")
		http.Error(w, "failed to marshal db check response", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	// #nosec G104 -- header already committed, write errors are unactionable
	w.Write(data) //nolint:errcheck
}

// readConfigDocuments extracts the workspace configuration documents from the
// request: every part of a multipart/form-data body in the order the client sent
// them, or the whole body as a single document for any other content type.
func readConfigDocuments(ctx context.Context, w http.ResponseWriter, r *http.Request) ([]ConfigDocument, error) {
	r.Body = http.MaxBytesReader(w, r.Body, dbCheckMaxBodySize)

	mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err == nil && strings.HasPrefix(mediaType, "multipart/") {
		boundary, ok := params["boundary"]
		if !ok {
			return nil, goerr.New("multipart request has no boundary parameter")
		}
		return readMultipartConfigDocuments(ctx, multipart.NewReader(r.Body, boundary))
	}

	data, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to read request body")
	}
	if len(data) == 0 {
		return nil, goerr.New("request carries no workspace configuration document")
	}
	return []ConfigDocument{{Name: dbCheckDefaultDocumentName, Data: data}}, nil
}

// readMultipartConfigDocuments reads every part as one document. The part's file
// name identifies it when present so the report can point at the operator's own
// file names; otherwise the form field name does.
func readMultipartConfigDocuments(ctx context.Context, reader *multipart.Reader) ([]ConfigDocument, error) {
	var docs []ConfigDocument
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, goerr.Wrap(err, "failed to read multipart request")
		}

		// NextPart closes the previous part on its own, but a read failure below
		// leaves this loop before the next call, so the part is closed here
		// explicitly. Part.Close only drains the remaining body, so closing it
		// twice is harmless.
		data, readErr := io.ReadAll(part)
		name := part.FileName()
		if name == "" {
			name = part.FormName()
		}
		safe.Close(ctx, part)
		if readErr != nil {
			return nil, goerr.Wrap(readErr, "failed to read multipart part",
				goerr.V("part", name))
		}
		if len(data) == 0 {
			continue
		}

		docs = append(docs, ConfigDocument{Name: name, Data: data})
	}

	if len(docs) == 0 {
		return nil, goerr.New("request carries no workspace configuration document")
	}
	return docs, nil
}
