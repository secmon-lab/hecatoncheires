package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/m-mizutani/gt"
	controllerhttp "github.com/secmon-lab/hecatoncheires/pkg/controller/http"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

// TestServer_DBCheckRouteRegistered drives the router itself, not the handler:
// the endpoint only exists if New registers it ahead of the catch-all SPA route,
// which serves index.html for anything it does not recognise and would hide a
// missing registration behind a 200.
func TestServer_DBCheckRouteRegistered(t *testing.T) {
	checker := &stubDBChecker{result: &usecase.ValidationResult{}}
	srv, err := controllerhttp.New(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }),
		controllerhttp.WithDBCheck(controllerhttp.NewDBCheckHandler(checker)),
	)
	gt.NoError(t, err).Required()

	req := httptest.NewRequest("POST", "/api/validate/db", strings.NewReader("[workspace]\nid = \"risk\"\n"))
	req.Header.Set("Content-Type", "application/toml")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	gt.Number(t, rec.Code).Equal(200)
	gt.String(t, rec.Header().Get("Content-Type")).Equal("application/json")
	gt.Array(t, checker.docs).Length(1).Required()
	gt.String(t, string(checker.docs[0].Data)).Equal("[workspace]\nid = \"risk\"\n")
}

// TestServer_DBCheckRouteAbsentWithoutHandler pins that a deployment that does
// not wire the checker exposes no such endpoint at all.
func TestServer_DBCheckRouteAbsentWithoutHandler(t *testing.T) {
	srv, err := controllerhttp.New(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }),
	)
	gt.NoError(t, err).Required()

	req := httptest.NewRequest("POST", "/api/validate/db", strings.NewReader("[workspace]\nid = \"risk\"\n"))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	gt.Number(t, rec.Code).Equal(http.StatusMethodNotAllowed)
}

type workspaceItem struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Emoji string `json:"emoji"`
	Color string `json:"color"`
}

type workspacesPayload struct {
	Workspaces []workspaceItem `json:"workspaces"`
}

func TestWorkspacesHandler_EmojiAndColor(t *testing.T) {
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "risk", Name: "Risk Management", Emoji: "🛡️"},
	})
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "incident", Name: "Incident Response", Color: "#c8501c"},
	})
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "plain", Name: "Plain Workspace"},
	})

	handler := controllerhttp.WorkspacesHandlerForTest(registry)
	req := httptest.NewRequest(http.MethodGet, "/api/workspaces", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	gt.Value(t, rec.Code).Equal(http.StatusOK)
	gt.String(t, rec.Header().Get("Content-Type")).Equal("application/json")

	var payload workspacesPayload
	gt.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload)).Required()
	gt.Array(t, payload.Workspaces).Length(3).Required()

	// Registration order is preserved by WorkspaceRegistry.
	gt.Value(t, payload.Workspaces[0].ID).Equal("risk")
	gt.Value(t, payload.Workspaces[0].Emoji).Equal("🛡️")
	gt.Value(t, payload.Workspaces[0].Color).Equal("")

	gt.Value(t, payload.Workspaces[1].ID).Equal("incident")
	gt.Value(t, payload.Workspaces[1].Color).Equal("#c8501c")
	gt.Value(t, payload.Workspaces[1].Emoji).Equal("")

	gt.Value(t, payload.Workspaces[2].ID).Equal("plain")
	gt.Value(t, payload.Workspaces[2].Emoji).Equal("")
	gt.Value(t, payload.Workspaces[2].Color).Equal("")
}

func TestWorkspacesHandler_OmitsEmptyEmojiColor(t *testing.T) {
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "plain", Name: "Plain Workspace"},
	})

	handler := controllerhttp.WorkspacesHandlerForTest(registry)
	req := httptest.NewRequest(http.MethodGet, "/api/workspaces", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	gt.Value(t, rec.Code).Equal(http.StatusOK)
	// omitempty: keys must not appear when unset.
	var raw map[string]any
	gt.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw)).Required()
	list, ok := raw["workspaces"].([]any)
	gt.Bool(t, ok).True()
	gt.Array(t, list).Length(1).Required()
	first, ok := list[0].(map[string]any)
	gt.Bool(t, ok).True()
	_, hasEmoji := first["emoji"]
	_, hasColor := first["color"]
	gt.Bool(t, hasEmoji).False()
	gt.Bool(t, hasColor).False()
}
