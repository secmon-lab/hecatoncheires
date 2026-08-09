package cli_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/cli"
	httpctrl "github.com/secmon-lab/hecatoncheires/pkg/controller/http"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	domainconfig "github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

// dbCheckWorkspaceTOML is the workspace configuration the checker is handed. The
// severity option set is the parameter under test: the stored Case value must be
// judged against this document, not against whatever the process was started
// with.
func dbCheckWorkspaceTOML(severityOptions string) []byte {
	return []byte(`
[workspace]
id = "risk"
name = "Risk Management"

[[fields]]
id = "severity"
name = "Severity"
type = "select"
` + severityOptions)
}

const dbCheckSeverityHighOnly = `
[[fields.options]]
id = "high"
name = "High"
`

const dbCheckSeverityHighAndLow = dbCheckSeverityHighOnly + `
[[fields.options]]
id = "low"
name = "Low"
`

// setupDBCheck builds a checker over a repository holding one Case whose
// severity is "low", and a process configuration that considers "low" valid. It
// returns the stored Case id so assertions can name the entity the report
// samples.
func setupDBCheck(t *testing.T) (context.Context, httpctrl.DBConsistencyChecker, int64) {
	t.Helper()
	ctx := context.Background()

	repo := memory.New()
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
		FieldSchema: &domainconfig.FieldSchema{
			Fields: []domainconfig.FieldDefinition{
				{
					ID:   "severity",
					Name: "Severity",
					Type: types.FieldTypeSelect,
					Options: []domainconfig.FieldOption{
						{ID: "high", Name: "High"},
						{ID: "low", Name: "Low"},
					},
				},
			},
		},
	})

	stored, err := repo.Case().Create(ctx, "risk", &model.Case{
		ReporterID: "U-TEST",
		Title:      "Stored severity is low",
		FieldValues: map[string]model.FieldValue{
			"severity": {FieldID: "severity", Type: types.FieldTypeSelect, Value: "low"},
		},
	})
	gt.NoError(t, err).Required()

	uc := usecase.New(repo, registry)
	return ctx, cli.NewDBConsistencyCheckerForTest(uc), stored.ID
}

func TestDBConsistencyChecker_ChecksAgainstSubmittedConfig(t *testing.T) {
	ctx, checker, caseID := setupDBCheck(t)

	// The submitted document drops the "low" option the process still has, so the
	// stored value must be reported even though the running configuration accepts
	// it.
	result, err := checker.CheckDBConsistency(ctx, []httpctrl.ConfigDocument{
		{Name: "risk.toml", Data: dbCheckWorkspaceTOML(dbCheckSeverityHighOnly)},
	})
	gt.NoError(t, err).Required()
	gt.Array(t, result.Issues).Length(1).Required()
	gt.Value(t, result.Issues[0].WorkspaceID).Equal("risk")
	gt.Value(t, result.Issues[0].Kind).Equal(usecase.IssueKindFieldValue)
	gt.Value(t, result.Issues[0].FieldID).Equal("severity")
	gt.Value(t, result.Issues[0].Actual).Equal("low")
	gt.Number(t, result.Issues[0].Count).Equal(int64(1))
	gt.Value(t, result.Issues[0].Sample.Kind).Equal(usecase.TargetKindCase)
	gt.Number(t, result.Issues[0].Sample.CaseID).Equal(caseID)
}

func TestDBConsistencyChecker_ReportsNothingWhenConfigCoversData(t *testing.T) {
	ctx, checker, _ := setupDBCheck(t)

	result, err := checker.CheckDBConsistency(ctx, []httpctrl.ConfigDocument{
		{Name: "risk.toml", Data: dbCheckWorkspaceTOML(dbCheckSeverityHighAndLow)},
	})
	gt.NoError(t, err).Required()
	gt.Bool(t, result.HasIssues()).False()
	gt.Array(t, result.Issues).Length(0)
}

// TestDBConsistencyChecker_UnknownWorkspaceIsNotChecked pins that the submitted
// configuration decides which workspaces are walked: a document naming a
// different workspace leaves the stored "risk" data untouched.
func TestDBConsistencyChecker_UnknownWorkspaceIsNotChecked(t *testing.T) {
	ctx, checker, _ := setupDBCheck(t)

	result, err := checker.CheckDBConsistency(ctx, []httpctrl.ConfigDocument{
		{Name: "other.toml", Data: []byte(`
[workspace]
id = "other"
name = "Other"

[[fields]]
id = "note"
name = "Note"
type = "text"
`)},
	})
	gt.NoError(t, err).Required()
	gt.Bool(t, result.HasIssues()).False()
}

func TestDBConsistencyChecker_InvalidConfigIsReportedAsClientFault(t *testing.T) {
	ctx, checker, _ := setupDBCheck(t)

	_, err := checker.CheckDBConsistency(ctx, []httpctrl.ConfigDocument{
		{Name: "broken.toml", Data: []byte("this is not TOML = = =")},
	})
	gt.Value(t, err).NotNil().Required()
	gt.Error(t, err).Is(httpctrl.ErrInvalidConfigDocument)
}

func TestDBConsistencyChecker_NoDocumentIsReportedAsClientFault(t *testing.T) {
	ctx, checker, _ := setupDBCheck(t)

	_, err := checker.CheckDBConsistency(ctx, nil)
	gt.Value(t, err).NotNil().Required()
	gt.Error(t, err).Is(httpctrl.ErrInvalidConfigDocument)
}
