package firestore

import (
	"context"

	"cloud.google.com/go/firestore"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// importsCollection returns the subcollection under workspaces/{workspaceID}
// that holds ImportSession documents. The collection name uses the same
// convention as the rest of the repository ("workspaces/{id}/<resource>").
const importsSubcollection = "imports"

type importRepository struct {
	client *firestore.Client
}

func newImportRepository(client *firestore.Client) *importRepository {
	return &importRepository{client: client}
}

func (r *importRepository) collection(workspaceID string) *firestore.CollectionRef {
	return r.client.Collection("workspaces").Doc(workspaceID).Collection(importsSubcollection)
}

func (r *importRepository) docRef(workspaceID string, id model.ImportSessionID) *firestore.DocumentRef {
	return r.collection(workspaceID).Doc(id.String())
}

func (r *importRepository) Create(ctx context.Context, workspaceID string, s *model.ImportSession) (*model.ImportSession, error) {
	if err := s.Validate(); err != nil {
		return nil, goerr.Wrap(err, "import session validation failed",
			goerr.V("workspace_id", workspaceID))
	}
	if s.WorkspaceID != workspaceID {
		return nil, goerr.New("workspaceID mismatch",
			goerr.V("argument", workspaceID),
			goerr.V("session", s.WorkspaceID))
	}

	// Use Create (not Set) so duplicate IDs are rejected by Firestore
	// instead of silently overwriting an existing session.
	if _, err := r.docRef(workspaceID, s.ID).Create(ctx, s); err != nil {
		if status.Code(err) == codes.AlreadyExists {
			return nil, goerr.Wrap(err, "import session already exists",
				goerr.V("workspace_id", workspaceID),
				goerr.V("import_id", s.ID))
		}
		return nil, goerr.Wrap(err, "failed to create import session",
			goerr.V("workspace_id", workspaceID),
			goerr.V("import_id", s.ID))
	}
	return s, nil
}

func (r *importRepository) Update(ctx context.Context, workspaceID string, s *model.ImportSession) (*model.ImportSession, error) {
	if err := s.Validate(); err != nil {
		return nil, goerr.Wrap(err, "import session validation failed",
			goerr.V("workspace_id", workspaceID))
	}
	if s.WorkspaceID != workspaceID {
		return nil, goerr.New("workspaceID mismatch",
			goerr.V("argument", workspaceID),
			goerr.V("session", s.WorkspaceID))
	}

	// Set is upsert-by-design here. The previous Get → Set guard added a
	// round-trip per Update with no real safety: any caller wanting an
	// existence check has already retrieved the session via Get before
	// mutating + Updating. Drop the extra read; the only legitimate
	// caller path is "Get → mutate → Update" through the usecase.
	if _, err := r.docRef(workspaceID, s.ID).Set(ctx, s); err != nil {
		return nil, goerr.Wrap(err, "failed to update import session",
			goerr.V("workspace_id", workspaceID),
			goerr.V("import_id", s.ID))
	}
	return s, nil
}

func (r *importRepository) Get(ctx context.Context, workspaceID string, id model.ImportSessionID) (*model.ImportSession, error) {
	doc, err := r.docRef(workspaceID, id).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, goerr.Wrap(ErrNotFound, "import session not found",
				goerr.V("workspace_id", workspaceID),
				goerr.V("import_id", id))
		}
		return nil, goerr.Wrap(err, "failed to get import session",
			goerr.V("workspace_id", workspaceID),
			goerr.V("import_id", id))
	}

	var s model.ImportSession
	if err := doc.DataTo(&s); err != nil {
		return nil, goerr.Wrap(err, "failed to decode import session",
			goerr.V("workspace_id", workspaceID),
			goerr.V("import_id", id))
	}
	return &s, nil
}
