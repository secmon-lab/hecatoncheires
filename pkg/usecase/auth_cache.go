package usecase

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/auth"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/firestore"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
)

// isTokenNotFound reports whether err comes from the repository signalling
// the token did not exist. Both backends define their own sentinel; the
// codebase consistently checks both in this combined form.
func isTokenNotFound(err error) bool {
	return errors.Is(err, memory.ErrNotFound) || errors.Is(err, firestore.ErrNotFound)
}

const (
	authCacheTTL = 5 * time.Minute
)

type cachedToken struct {
	token     *auth.Token
	expiresAt time.Time
}

type authCache struct {
	cache sync.Map
}

func newAuthCache() *authCache {
	return &authCache{}
}

func (c *authCache) get(tokenID auth.TokenID) (*auth.Token, bool) {
	val, ok := c.cache.Load(tokenID)
	if !ok {
		return nil, false
	}

	cached := val.(*cachedToken)
	if time.Now().After(cached.expiresAt) {
		c.cache.Delete(tokenID)
		return nil, false
	}

	return cached.token, true
}

func (c *authCache) set(token *auth.Token) {
	cached := &cachedToken{
		token:     token,
		expiresAt: time.Now().Add(authCacheTTL),
	}
	c.cache.Store(token.ID, cached)
}

func (c *authCache) remove(tokenID auth.TokenID) {
	c.cache.Delete(tokenID)
}

// validateTokenWithCache validates token with cache
func (uc *AuthUseCase) validateTokenWithCache(ctx context.Context, tokenID auth.TokenID, tokenSecret auth.TokenSecret) (*auth.Token, error) {
	// Check cache first
	if token, ok := uc.cache.get(tokenID); ok {
		// Verify secret matches
		if token.Secret != tokenSecret {
			return nil, goerr.New("invalid token secret", goerr.T(errutil.TagBenign))
		}
		// Check if token is expired
		if token.IsExpired() {
			uc.cache.remove(tokenID)
			return nil, goerr.New("token expired", goerr.T(errutil.TagBenign))
		}
		return token, nil
	}

	// Cache miss, get from repository. Only the "not found" path is part
	// of the normal flow (unauthenticated visitor / revoked or expired
	// session); other failures (Firestore outage, deadline, permission
	// errors, decode failures) MUST keep paging Sentry, so they stay
	// untagged.
	token, err := uc.repo.GetToken(ctx, tokenID)
	if err != nil {
		opts := []goerr.Option{}
		if isTokenNotFound(err) {
			opts = append(opts, goerr.T(errutil.TagBenign))
		}
		return nil, goerr.Wrap(err, "failed to get token from repository", opts...)
	}

	// Verify secret matches
	if token.Secret != tokenSecret {
		return nil, goerr.New("invalid token secret", goerr.T(errutil.TagBenign))
	}

	// Check if token is expired
	if token.IsExpired() {
		if err := uc.repo.DeleteToken(ctx, tokenID); err != nil {
			return nil, goerr.Wrap(err, "failed to delete expired token", goerr.V("tokenID", tokenID))
		}
		return nil, goerr.New("token expired", goerr.T(errutil.TagBenign))
	}

	// Cache the token
	uc.cache.set(token)

	return token, nil
}
