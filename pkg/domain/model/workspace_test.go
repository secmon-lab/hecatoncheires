package model_test

import (
	"errors"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
)

func TestNewWorkspaceRegistry(t *testing.T) {
	reg := model.NewWorkspaceRegistry()
	gt.Value(t, reg).NotNil()
	gt.Array(t, reg.List()).Length(0)
	gt.Array(t, reg.Workspaces()).Length(0)
}

func TestWorkspaceRegistry_Register(t *testing.T) {
	reg := model.NewWorkspaceRegistry()

	entry := &model.WorkspaceEntry{
		Workspace: model.Workspace{
			ID:   "risk",
			Name: "Risk Management",
		},
		FieldSchema: &config.FieldSchema{
			Labels: config.EntityLabels{Case: "Risk"},
		},
	}
	reg.Register(entry)

	gt.Array(t, reg.List()).Length(1)
	gt.Array(t, reg.Workspaces()).Length(1)
	gt.Value(t, reg.Workspaces()[0].ID).Equal("risk")
	gt.Value(t, reg.Workspaces()[0].Name).Equal("Risk Management")
}

func TestWorkspaceRegistry_RegisterMultiple(t *testing.T) {
	reg := model.NewWorkspaceRegistry()

	reg.Register(&model.WorkspaceEntry{
		Workspace:   model.Workspace{ID: "risk", Name: "Risk Management"},
		FieldSchema: &config.FieldSchema{},
	})
	reg.Register(&model.WorkspaceEntry{
		Workspace:   model.Workspace{ID: "recruit", Name: "Recruitment"},
		FieldSchema: &config.FieldSchema{},
	})

	gt.Array(t, reg.List()).Length(2)
	gt.Array(t, reg.Workspaces()).Length(2)

	// Verify registration order is preserved
	workspaces := reg.Workspaces()
	gt.Value(t, workspaces[0].ID).Equal("risk")
	gt.Value(t, workspaces[1].ID).Equal("recruit")
}

func TestWorkspaceRegistry_RegisterOverwrite(t *testing.T) {
	reg := model.NewWorkspaceRegistry()

	reg.Register(&model.WorkspaceEntry{
		Workspace:   model.Workspace{ID: "risk", Name: "Old Name"},
		FieldSchema: &config.FieldSchema{},
	})
	reg.Register(&model.WorkspaceEntry{
		Workspace:   model.Workspace{ID: "risk", Name: "New Name"},
		FieldSchema: &config.FieldSchema{},
	})

	// Should not duplicate the entry
	gt.Array(t, reg.List()).Length(1)
	gt.Array(t, reg.Workspaces()).Length(1)

	// Should have the updated name
	gt.Value(t, reg.Workspaces()[0].Name).Equal("New Name")
}

func TestWorkspaceRegistry_Get(t *testing.T) {
	reg := model.NewWorkspaceRegistry()

	schema := &config.FieldSchema{
		Labels: config.EntityLabels{Case: "Risk"},
		Fields: []config.FieldDefinition{
			{ID: "category", Name: "Category", Type: "text"},
		},
	}
	reg.Register(&model.WorkspaceEntry{
		Workspace:   model.Workspace{ID: "risk", Name: "Risk Management"},
		FieldSchema: schema,
	})

	entry, err := reg.Get("risk")
	gt.NoError(t, err)
	gt.Value(t, entry).NotNil()
	gt.Value(t, entry.Workspace.ID).Equal("risk")
	gt.Value(t, entry.Workspace.Name).Equal("Risk Management")
	gt.Value(t, entry.FieldSchema.Labels.Case).Equal("Risk")
	gt.Array(t, entry.FieldSchema.Fields).Length(1)
	gt.Value(t, entry.FieldSchema.Fields[0].ID).Equal("category")
}

func TestWorkspaceRegistry_GetNotFound(t *testing.T) {
	reg := model.NewWorkspaceRegistry()

	entry, err := reg.Get("nonexistent")
	gt.Value(t, entry).Nil()
	gt.Value(t, err).NotNil()
	gt.Bool(t, errors.Is(err, model.ErrWorkspaceNotFound)).True()
}

func TestWorkspaceRegistry_List(t *testing.T) {
	reg := model.NewWorkspaceRegistry()

	reg.Register(&model.WorkspaceEntry{
		Workspace:   model.Workspace{ID: "alpha", Name: "Alpha"},
		FieldSchema: &config.FieldSchema{},
	})
	reg.Register(&model.WorkspaceEntry{
		Workspace:   model.Workspace{ID: "beta", Name: "Beta"},
		FieldSchema: &config.FieldSchema{},
	})
	reg.Register(&model.WorkspaceEntry{
		Workspace:   model.Workspace{ID: "gamma", Name: "Gamma"},
		FieldSchema: &config.FieldSchema{},
	})

	entries := reg.List()
	gt.Array(t, entries).Length(3)

	// Verify registration order is preserved
	gt.Value(t, entries[0].Workspace.ID).Equal("alpha")
	gt.Value(t, entries[1].Workspace.ID).Equal("beta")
	gt.Value(t, entries[2].Workspace.ID).Equal("gamma")

	// Verify each entry has its FieldSchema
	for _, e := range entries {
		gt.Value(t, e.FieldSchema).NotNil()
	}
}

func TestWorkspaceRegistry_Workspaces(t *testing.T) {
	reg := model.NewWorkspaceRegistry()

	reg.Register(&model.WorkspaceEntry{
		Workspace:   model.Workspace{ID: "risk", Name: "Risk Management"},
		FieldSchema: &config.FieldSchema{},
	})
	reg.Register(&model.WorkspaceEntry{
		Workspace:   model.Workspace{ID: "recruit", Name: "Recruitment"},
		FieldSchema: &config.FieldSchema{},
	})

	workspaces := reg.Workspaces()
	gt.Array(t, workspaces).Length(2)
	gt.Value(t, workspaces[0].ID).Equal("risk")
	gt.Value(t, workspaces[0].Name).Equal("Risk Management")
	gt.Value(t, workspaces[1].ID).Equal("recruit")
	gt.Value(t, workspaces[1].Name).Equal("Recruitment")
}
