package model_test

import (
	"errors"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

func TestWorkspaceGroup_Validate(t *testing.T) {
	t.Run("valid group passes", func(t *testing.T) {
		g := &model.WorkspaceGroup{ID: "security", Name: "Security", MemberIDs: []string{"risk"}}
		gt.NoError(t, g.Validate())
	})

	t.Run("empty ID is rejected", func(t *testing.T) {
		g := &model.WorkspaceGroup{ID: "", Name: "Security"}
		err := g.Validate()
		gt.Error(t, err).Is(model.ErrInvalidWorkspaceGroup)
	})

	t.Run("empty members are allowed", func(t *testing.T) {
		g := &model.WorkspaceGroup{ID: "wip"}
		gt.NoError(t, g.Validate())
	})
}

func TestNewWorkspaceGroupRegistry(t *testing.T) {
	reg := model.NewWorkspaceGroupRegistry()
	gt.Value(t, reg).NotNil()
	gt.Array(t, reg.List()).Length(0)
}

func TestWorkspaceGroupRegistry_Register(t *testing.T) {
	reg := model.NewWorkspaceGroupRegistry()
	reg.Register(&model.WorkspaceGroup{
		ID:          "security",
		Name:        "Security",
		Description: "Security workspaces",
		MemberIDs:   []string{"risk", "incident"},
	})

	got := reg.List()
	gt.Array(t, got).Length(1).Required()
	gt.Value(t, got[0].ID).Equal("security")
	gt.Value(t, got[0].Name).Equal("Security")
	gt.Value(t, got[0].Description).Equal("Security workspaces")
	gt.Array(t, got[0].MemberIDs).Length(2)
	gt.Value(t, got[0].MemberIDs[0]).Equal("risk")
	gt.Value(t, got[0].MemberIDs[1]).Equal("incident")
}

func TestWorkspaceGroupRegistry_RegisterMultiplePreservesOrder(t *testing.T) {
	reg := model.NewWorkspaceGroupRegistry()
	reg.Register(&model.WorkspaceGroup{ID: "security"})
	reg.Register(&model.WorkspaceGroup{ID: "corp"})
	reg.Register(&model.WorkspaceGroup{ID: "audit"})

	got := reg.List()
	gt.Array(t, got).Length(3).Required()
	gt.Value(t, got[0].ID).Equal("security")
	gt.Value(t, got[1].ID).Equal("corp")
	gt.Value(t, got[2].ID).Equal("audit")
}

func TestWorkspaceGroupRegistry_RegisterOverwriteKeepsPosition(t *testing.T) {
	reg := model.NewWorkspaceGroupRegistry()
	reg.Register(&model.WorkspaceGroup{ID: "security", Name: "Old"})
	reg.Register(&model.WorkspaceGroup{ID: "corp", Name: "Corp"})
	reg.Register(&model.WorkspaceGroup{ID: "security", Name: "New"})

	got := reg.List()
	gt.Array(t, got).Length(2).Required()
	// security keeps its original first position, with the updated name.
	gt.Value(t, got[0].ID).Equal("security")
	gt.Value(t, got[0].Name).Equal("New")
	gt.Value(t, got[1].ID).Equal("corp")
}

func TestWorkspaceGroupRegistry_Get(t *testing.T) {
	reg := model.NewWorkspaceGroupRegistry()
	reg.Register(&model.WorkspaceGroup{ID: "security", Name: "Security", MemberIDs: []string{"risk"}})

	g, err := reg.Get("security")
	gt.NoError(t, err).Required()
	gt.Value(t, g.ID).Equal("security")
	gt.Value(t, g.Name).Equal("Security")
	gt.Array(t, g.MemberIDs).Length(1)
	gt.Value(t, g.MemberIDs[0]).Equal("risk")
}

func TestWorkspaceGroupRegistry_GetNotFound(t *testing.T) {
	reg := model.NewWorkspaceGroupRegistry()
	g, err := reg.Get("nope")
	gt.Value(t, g).Nil()
	gt.Bool(t, errors.Is(err, model.ErrWorkspaceGroupNotFound)).True()
}
