package firestore

import (
	"context"

	"cloud.google.com/go/firestore"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type userPreferenceRepository struct {
	client *firestore.Client
}

func newUserPreferenceRepository(client *firestore.Client) *userPreferenceRepository {
	return &userPreferenceRepository{client: client}
}

// docRef returns the single per-user preference document.
// Path: userPreferences/{userID}
func (r *userPreferenceRepository) docRef(userID string) *firestore.DocumentRef {
	return r.client.Collection("userPreferences").Doc(userID)
}

func (r *userPreferenceRepository) Get(ctx context.Context, userID string) (*model.UserPreference, error) {
	docSnap, err := r.docRef(userID).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, goerr.Wrap(ErrNotFound, "user preference not found", goerr.V("user_id", userID))
		}
		return nil, goerr.Wrap(err, "failed to get user preference", goerr.V("user_id", userID))
	}

	var p model.UserPreference
	if err := docSnap.DataTo(&p); err != nil {
		return nil, goerr.Wrap(err, "failed to decode user preference", goerr.V("user_id", userID))
	}
	return &p, nil
}

func (r *userPreferenceRepository) Set(ctx context.Context, pref *model.UserPreference) error {
	if err := pref.Validate(); err != nil {
		return goerr.Wrap(err, "user preference validation failed before set")
	}

	if _, err := r.docRef(pref.UserID).Set(ctx, pref); err != nil {
		return goerr.Wrap(err, "failed to set user preference", goerr.V("user_id", pref.UserID))
	}
	return nil
}
