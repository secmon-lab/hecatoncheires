package firestore

import (
	"context"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/auth"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const tokensCollection = "tokens"

func (r *Firestore) PutToken(ctx context.Context, token *auth.Token) error {
	if err := token.Validate(); err != nil {
		return goerr.Wrap(err, "invalid token")
	}

	docRef := r.client.Collection(tokensCollection).Doc(token.ID.String())
	if _, err := docRef.Set(ctx, token); err != nil {
		return goerr.Wrap(err, "failed to put token to firestore")
	}

	return nil
}

func (r *Firestore) GetToken(ctx context.Context, tokenID auth.TokenID) (*auth.Token, error) {
	if err := tokenID.Validate(); err != nil {
		return nil, goerr.Wrap(err, "invalid token ID")
	}

	docRef := r.client.Collection(tokensCollection).Doc(tokenID.String())
	doc, err := docRef.Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, ErrNotFound
		}
		return nil, goerr.Wrap(err, "failed to get token from firestore")
	}

	var token auth.Token
	if err := doc.DataTo(&token); err != nil {
		return nil, goerr.Wrap(err, "failed to unmarshal token")
	}

	return &token, nil
}

func (r *Firestore) DeleteToken(ctx context.Context, tokenID auth.TokenID) error {
	if err := tokenID.Validate(); err != nil {
		return goerr.Wrap(err, "invalid token ID")
	}

	docRef := r.client.Collection(tokensCollection).Doc(tokenID.String())

	// Check if document exists first
	_, err := docRef.Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return ErrNotFound
		}
		return goerr.Wrap(err, "failed to get token from firestore")
	}

	// Delete the document
	if _, err := docRef.Delete(ctx); err != nil {
		return goerr.Wrap(err, "failed to delete token from firestore")
	}

	return nil
}
