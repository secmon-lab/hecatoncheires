package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/m-mizutani/gt"
	controllerhttp "github.com/secmon-lab/hecatoncheires/pkg/controller/http"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

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
