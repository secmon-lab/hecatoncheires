package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/auth"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

const importTestWorkspaceID = "ws-acme"
const importTestUserID = "U12345678"

func newImportTestSetup(t *testing.T) (*usecase.UseCases, *memory.Memory, context.Context) {
	t.Helper()

	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: importTestWorkspaceID, Name: "ACME"},
		FieldSchema: &config.FieldSchema{
			Fields: []config.FieldDefinition{
				{ID: "severity", Type: types.FieldTypeSelect, Required: false, Options: []config.FieldOption{
					{ID: "low"}, {ID: "medium"}, {ID: "high"},
				}},
				{ID: "source", Type: types.FieldTypeText, Required: false},
				{ID: "owner", Type: types.FieldTypeUser, Required: false},
			},
		},
	})

	repo := memory.New()
	uc := usecase.New(repo, registry)

	tok := auth.NewToken(importTestUserID, "alice@example.com", "Alice")
	ctx := auth.ContextWithToken(context.Background(), tok)
	return uc, repo, ctx
}

func newImportTestUseCases(t *testing.T) (*usecase.UseCases, context.Context) {
	uc, _, ctx := newImportTestSetup(t)
	return uc, ctx
}

const validYAML = `
version: 1
cases:
  - title: "Suspicious login"
    description: "Multiple attempts."
    isPrivate: false
    fields:
      severity: high
      source: cloudtrail
    actions:
      - title: "Block IP"
        description: "firewall"
      - title: "Notify SOC"
`

const yamlWithMissingTitle = `
version: 1
cases:
  - title: ""
    description: "no title"
    fields:
      severity: high
    actions:
      - title: "Action"
`

const yamlUnsupportedVersion = `
version: 2
cases:
  - title: "x"
`

const yamlWithInvalidSelectValue = `
version: 1
cases:
  - title: "Bad fields"
    fields:
      severity: urgent
`

func TestImport_Create_ValidYAML(t *testing.T) {
	uc, ctx := newImportTestUseCases(t)
	session, err := uc.Import.Create(ctx, importTestWorkspaceID, validYAML, "incidents.yaml")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if session.Status != model.ImportSessionPending {
		t.Errorf("status: got %q want pending", session.Status)
	}
	if !session.Valid() {
		t.Errorf("expected valid=true; issues=%+v", session.Issues)
		for _, c := range session.Snapshot.Cases {
			t.Logf("case issues: %+v", c.Issues)
		}
	}
	if len(session.Snapshot.Cases) != 1 {
		t.Fatalf("cases: got %d want 1", len(session.Snapshot.Cases))
	}
	c := session.Snapshot.Cases[0]
	if c.Title != "Suspicious login" {
		t.Errorf("title: %q", c.Title)
	}
	// Actions are intentionally NOT imported (DRAFT restriction). The
	// YAML had 2 actions but the persisted snapshot must carry none —
	// a single WARNING issue captures the drop instead.
	if len(c.Actions) != 0 {
		t.Errorf("actions should not be imported into DRAFT; got %d", len(c.Actions))
	}
	warned := false
	for _, i := range c.Issues {
		if i.Severity == model.ImportIssueWarning && strings.Contains(i.Path, "actions") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("expected actions-ignored warning; got %+v", c.Issues)
	}
	if c.FieldValues["severity"].Value != "high" {
		t.Errorf("field severity: %+v", c.FieldValues["severity"])
	}
	if session.FieldSchemaHash == "" {
		t.Errorf("schema hash should be non-empty")
	}
	if session.CreatorUserID != importTestUserID {
		t.Errorf("creator: %q", session.CreatorUserID)
	}
	if session.Source.OriginalFileName != "incidents.yaml" {
		t.Errorf("filename: %q", session.Source.OriginalFileName)
	}
}

func TestImport_Create_MissingTitleProducesError(t *testing.T) {
	uc, ctx := newImportTestUseCases(t)
	session, err := uc.Import.Create(ctx, importTestWorkspaceID, yamlWithMissingTitle, "x.yaml")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if session.Valid() {
		t.Fatalf("expected valid=false")
	}
	found := false
	for _, c := range session.Snapshot.Cases {
		for _, i := range c.Issues {
			if i.Severity == model.ImportIssueError && strings.Contains(i.Message, "title is required") {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("expected title-required error not found; issues=%+v", session.Snapshot.Cases[0].Issues)
	}
}

func TestImport_Create_UnsupportedVersion(t *testing.T) {
	uc, ctx := newImportTestUseCases(t)
	session, err := uc.Import.Create(ctx, importTestWorkspaceID, yamlUnsupportedVersion, "x.yaml")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if session.Valid() {
		t.Fatalf("expected valid=false")
	}
	found := false
	for _, i := range session.Issues {
		if i.Severity == model.ImportIssueError && strings.Contains(i.Message, "unsupported version") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected unsupported-version error; got %+v", session.Issues)
	}
}

func TestImport_Create_InvalidSelectValue(t *testing.T) {
	uc, ctx := newImportTestUseCases(t)
	session, err := uc.Import.Create(ctx, importTestWorkspaceID, yamlWithInvalidSelectValue, "x.yaml")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if session.Valid() {
		t.Fatalf("expected valid=false")
	}
	found := false
	for _, c := range session.Snapshot.Cases {
		for _, i := range c.Issues {
			if i.Severity == model.ImportIssueError && strings.Contains(i.Path, "fields") {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("expected fields validation error")
	}
}

func TestImport_Get_OnlyCreator(t *testing.T) {
	uc, ctx := newImportTestUseCases(t)
	session, err := uc.Import.Create(ctx, importTestWorkspaceID, validYAML, "x.yaml")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Same caller — OK.
	got, err := uc.Import.Get(ctx, importTestWorkspaceID, session.ID)
	if err != nil {
		t.Fatalf("Get same user: %v", err)
	}
	if got.ID != session.ID {
		t.Errorf("Get returned different session")
	}

	// Different caller — must look like "not found".
	otherTok := auth.NewToken("U_OTHER", "o@example.com", "Other")
	otherCtx := auth.ContextWithToken(context.Background(), otherTok)
	if _, err := uc.Import.Get(otherCtx, importTestWorkspaceID, session.ID); !errors.Is(err, usecase.ErrImportSessionNotFound) {
		t.Fatalf("expected ErrImportSessionNotFound for other user; got %v", err)
	}
}

func TestImport_Execute_AllCreated(t *testing.T) {
	uc, ctx := newImportTestUseCases(t)
	session, err := uc.Import.Create(ctx, importTestWorkspaceID, validYAML, "x.yaml")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	executed, err := uc.Import.Execute(ctx, importTestWorkspaceID, session.ID)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if executed.Status != model.ImportSessionApplied {
		t.Fatalf("status: %q", executed.Status)
	}
	if executed.CreatedCount != 1 {
		t.Errorf("created count: %d", executed.CreatedCount)
	}
	if executed.FailedCount != 0 || executed.SkippedCount != 0 {
		t.Errorf("counts: failed=%d skipped=%d", executed.FailedCount, executed.SkippedCount)
	}
	c := executed.Snapshot.Cases[0]
	if c.Result.Status != model.ImportItemCreated || c.Result.CreatedCaseID == nil {
		t.Errorf("case result: %+v", c.Result)
	}
	// Actions are not part of the snapshot — Import does not create them.
	if len(c.Actions) != 0 {
		t.Errorf("snapshot must not carry Actions; got %d", len(c.Actions))
	}
}

func TestImport_Execute_RejectsInvalidSession(t *testing.T) {
	uc, ctx := newImportTestUseCases(t)
	session, err := uc.Import.Create(ctx, importTestWorkspaceID, yamlWithMissingTitle, "x.yaml")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, err = uc.Import.Execute(ctx, importTestWorkspaceID, session.ID)
	if !errors.Is(err, usecase.ErrImportValidation) {
		t.Fatalf("expected ErrImportValidation; got %v", err)
	}
}

func TestImport_Execute_DoubleExecuteRejected(t *testing.T) {
	uc, ctx := newImportTestUseCases(t)
	session, err := uc.Import.Create(ctx, importTestWorkspaceID, validYAML, "x.yaml")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := uc.Import.Execute(ctx, importTestWorkspaceID, session.ID); err != nil {
		t.Fatalf("first Execute: %v", err)
	}
	_, err = uc.Import.Execute(ctx, importTestWorkspaceID, session.ID)
	if !errors.Is(err, usecase.ErrImportSessionInvalidState) {
		t.Fatalf("expected ErrImportSessionInvalidState; got %v", err)
	}
}

// Required fields must be present even though imported cases land as
// DRAFT. Import is the bulk-load entry point; missing required fields
// indicate the YAML is incomplete and should be surfaced at preview
// time so the user can fix it before executing. (DRAFT's partial check
// is reserved for the Slack "Save as draft" half-finished flow, not
// for bulk import.)
func TestImport_Create_RejectsMissingRequiredFields(t *testing.T) {
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: importTestWorkspaceID, Name: "ACME"},
		FieldSchema: &config.FieldSchema{
			Fields: []config.FieldDefinition{
				// severity is REQUIRED in the workspace schema
				{ID: "severity", Type: types.FieldTypeSelect, Required: true, Options: []config.FieldOption{
					{ID: "low"}, {ID: "medium"}, {ID: "high"},
				}},
				{ID: "source", Type: types.FieldTypeText, Required: false},
			},
		},
	})
	repo := memory.New()
	uc := usecase.New(repo, registry)
	tok := auth.NewToken(importTestUserID, "alice@example.com", "Alice")
	ctx := auth.ContextWithToken(context.Background(), tok)

	yamlWithoutFields := `
version: 1
cases:
  - title: "no fields at all"
    actions:
      - title: "do something"
  - title: "only optional field"
    fields:
      source: cloudtrail
    actions:
      - title: "investigate"
  - title: "complete"
    fields:
      severity: high
      source: cloudtrail
    actions:
      - title: "investigate"
`
	session, err := uc.Import.Create(ctx, importTestWorkspaceID, yamlWithoutFields, "x.yaml")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Cases 0 and 1 are missing the required severity → preview must
	// surface a per-field ERROR (path includes the missing field ID,
	// message names it) and the session as a whole must be invalid.
	if session.Valid() {
		t.Fatalf("expected valid=false because required fields are missing")
	}
	for _, idx := range []int{0, 1} {
		c := session.Snapshot.Cases[idx]
		var matched *model.ImportIssue
		for i := range c.Issues {
			if c.Issues[i].Path == fmt.Sprintf("cases[%d].fields.severity", idx) {
				m := c.Issues[i]
				matched = &m
				break
			}
		}
		if matched == nil {
			t.Errorf("case %d: expected per-field path 'cases[%d].fields.severity'; got %+v", idx, idx, c.Issues)
			continue
		}
		if matched.Severity != model.ImportIssueError {
			t.Errorf("case %d: expected ERROR severity; got %q", idx, matched.Severity)
		}
		if !strings.Contains(matched.Message, "severity") {
			t.Errorf("case %d: message should name the missing field; got %q", idx, matched.Message)
		}
	}
	// Case 2 fully satisfies the schema → it should have no error-
	// severity issues of its own.
	for _, i := range session.Snapshot.Cases[2].Issues {
		if i.Severity == model.ImportIssueError {
			t.Errorf("case 2 should have no errors; got %+v", i)
		}
	}
	// Execute must be refused at the validation gate so that no DRAFT
	// is silently created from an incomplete YAML.
	if _, err := uc.Import.Execute(ctx, importTestWorkspaceID, session.ID); !errors.Is(err, usecase.ErrImportValidation) {
		t.Fatalf("expected ErrImportValidation; got %v", err)
	}
}

func TestImport_Execute_FieldSchemaStale(t *testing.T) {
	uc, ctx := newImportTestUseCases(t)
	session, err := uc.Import.Create(ctx, importTestWorkspaceID, validYAML, "x.yaml")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Mutate the field schema so the schema hash drifts. Drop "severity"
	// so the case's severity=high becomes an unknown-value error under
	// the new schema.
	registry := uc.WorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: importTestWorkspaceID, Name: "ACME"},
		FieldSchema: &config.FieldSchema{
			Fields: []config.FieldDefinition{
				{ID: "source", Type: types.FieldTypeText, Required: false},
			},
		},
	})

	_, err = uc.Import.Execute(ctx, importTestWorkspaceID, session.ID)
	if !errors.Is(err, usecase.ErrImportFieldSchemaStale) {
		t.Fatalf("expected ErrImportFieldSchemaStale; got %v", err)
	}
	// The persisted session must now carry the stale issues for the UI.
	reloaded, err := uc.Import.Get(ctx, importTestWorkspaceID, session.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	found := false
	for _, i := range reloaded.Issues {
		if i.Severity == model.ImportIssueError && strings.Contains(i.Message, "field schema has changed") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected stale-schema header issue, got %+v", reloaded.Issues)
	}
}

// Slack user references in assigneeIDs and USER-typed fields must
// resolve against the workspace's SlackUser registry at preview time.
// Unknown users surface as per-reference ERROR issues so the YAML
// author knows exactly which lines to fix before executing.
func TestImport_Create_DetectsUnknownSlackUsers(t *testing.T) {
	uc, repo, ctx := newImportTestSetup(t)

	// Seed exactly one known Slack user. Everything else is "unknown".
	if err := repo.SlackUser().SaveMany(ctx, []*model.SlackUser{
		{ID: model.SlackUserID("U_KNOWN"), Name: "known"},
	}); err != nil {
		t.Fatalf("seed slack users: %v", err)
	}

	const y = `version: 1
cases:
  - title: "Mixed assignees"
    assigneeIDs:
      - U_KNOWN
      - U_UNKNOWN_1
    fields:
      owner: U_UNKNOWN_2
  - title: "Clean case"
    assigneeIDs:
      - U_KNOWN
    fields:
      owner: U_KNOWN
`
	session, err := uc.Import.Create(ctx, importTestWorkspaceID, y, "x.yaml")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if session.Valid() {
		t.Fatalf("expected valid=false because some users are unknown")
	}

	// Case 0: two unknown-user errors with precise paths.
	c0 := session.Snapshot.Cases[0]
	pathFound := map[string]bool{}
	for _, i := range c0.Issues {
		if i.Severity == model.ImportIssueError {
			pathFound[i.Path] = true
		}
	}
	if !pathFound["cases[0].assigneeIDs[1]"] {
		t.Errorf("expected error at cases[0].assigneeIDs[1] for U_UNKNOWN_1; got %+v", c0.Issues)
	}
	if !pathFound["cases[0].fields.owner"] {
		t.Errorf("expected error at cases[0].fields.owner for U_UNKNOWN_2; got %+v", c0.Issues)
	}

	// Case 0: the U_KNOWN assignee must NOT produce an issue.
	for _, i := range c0.Issues {
		if strings.Contains(i.Message, "U_KNOWN") && !strings.Contains(i.Message, "U_KNOWN_") {
			t.Errorf("U_KNOWN should not produce an unknown-user issue; got %+v", i)
		}
	}

	// Case 1: all references resolve, so no per-user issues.
	c1 := session.Snapshot.Cases[1]
	for _, i := range c1.Issues {
		if strings.Contains(i.Message, "unknown Slack user") {
			t.Errorf("Case 1 should have no unknown-user issues; got %+v", i)
		}
	}

	// Execute must be refused because the session is invalid.
	if _, err := uc.Import.Execute(ctx, importTestWorkspaceID, session.ID); !errors.Is(err, usecase.ErrImportValidation) {
		t.Fatalf("expected ErrImportValidation; got %v", err)
	}
}
