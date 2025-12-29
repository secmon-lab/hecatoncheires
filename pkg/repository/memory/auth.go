package memory

import (
	"context"
	"sync"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/auth"
)

type tokenStore struct {
	mu     sync.RWMutex
	tokens map[auth.TokenID]*auth.Token
}

func newTokenStore() *tokenStore {
	return &tokenStore{
		tokens: make(map[auth.TokenID]*auth.Token),
	}
}

func (r *Repository) PutToken(ctx context.Context, token *auth.Token) error {
	if err := token.Validate(); err != nil {
		return goerr.Wrap(err, "invalid token")
	}

	r.tokens.mu.Lock()
	defer r.tokens.mu.Unlock()

	r.tokens.tokens[token.ID] = token
	return nil
}

func (r *Repository) GetToken(ctx context.Context, tokenID auth.TokenID) (*auth.Token, error) {
	if err := tokenID.Validate(); err != nil {
		return nil, goerr.Wrap(err, "invalid token ID")
	}

	r.tokens.mu.RLock()
	defer r.tokens.mu.RUnlock()

	token, ok := r.tokens.tokens[tokenID]
	if !ok {
		return nil, ErrNotFound
	}

	return token, nil
}

func (r *Repository) DeleteToken(ctx context.Context, tokenID auth.TokenID) error {
	if err := tokenID.Validate(); err != nil {
		return goerr.Wrap(err, "invalid token ID")
	}

	r.tokens.mu.Lock()
	defer r.tokens.mu.Unlock()

	if _, ok := r.tokens.tokens[tokenID]; !ok {
		return ErrNotFound
	}

	delete(r.tokens.tokens, tokenID)
	return nil
}
