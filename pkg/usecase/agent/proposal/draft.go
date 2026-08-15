package proposal

import (
	"strings"

	"github.com/m-mizutani/goerr/v2"
)

// Draft is the terminal output of a case-draft turn: the case the agent proposes.
// The schema handed to the model is derived from these struct tags via
// gollem.ToSchema; Validate enforces what a plain JSON schema cannot.
//
// It is a proposal, not a case: the host renders it into a preview a human
// reviews, edits and submits.
type Draft struct {
	WorkspaceID string `json:"workspace_id" description:"The id of the workspace this case belongs to, taken from the registered list." required:"true"`
	Title       string `json:"title" description:"A concise case title, about 80 characters or fewer. A noun phrase that fits one line." required:"true"`
	Description string `json:"description" description:"A clear case description, never more than 2000 characters. Summarise; do not paste raw logs or whole transcripts." required:"true"`
	// CustomFieldValues is keyed by the workspace's field ids. A required field the
	// agent could not determine is left out on purpose — the review UI blocks
	// submit until the human fills it, which is better than a fabricated value.
	CustomFieldValues map[string]any `json:"custom_field_values,omitempty" description:"Field values keyed by the field ids from get_workspace. Omit a field you cannot determine rather than guessing."`
	IsTest            bool           `json:"is_test,omitempty" description:"True only when the case exists to verify the system itself or as a drill, never for a real case."`
}

// Validate enforces the draft's shape invariants so a workspace-less or
// title-less proposal is rejected inside planexec's regeneration loop rather than
// reaching the human as a broken preview. It satisfies planexec.Validatable.
//
// The field VALUES are not checked here: that needs the workspace's schema, which
// this method cannot see. The host's finalizer does it.
func (d Draft) Validate() error {
	if strings.TrimSpace(d.WorkspaceID) == "" {
		return goerr.New("the draft must name the workspace it belongs to")
	}
	if strings.TrimSpace(d.Title) == "" {
		return goerr.New("the draft requires a non-empty title")
	}
	if strings.TrimSpace(d.Description) == "" {
		return goerr.New("the draft requires a non-empty description")
	}
	return nil
}
