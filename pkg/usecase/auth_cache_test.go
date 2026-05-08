package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/auth"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
)

// flakyAuthRepo wraps the in-memory repository and lets a test substitute
// the GetToken response. The other ~15 Repository methods stay backed by
// memory.New() so we do not have to hand-roll the whole interface for one
// branch of one usecase method.
type flakyAuthRepo struct {
	interfaces.Repository
	getTokenErr error
}

func (r *flakyAuthRepo) GetToken(ctx context.Context, tokenID auth.TokenID) (*auth.Token, error) {
	if r.getTokenErr != nil {
		return nil, r.getTokenErr
	}
	return r.Repository.GetToken(ctx, tokenID)
}

func newAuthUC(t *testing.T, repo interfaces.Repository) *usecase.AuthUseCase {
	t.Helper()
	return usecase.NewAuthUseCase(repo, "client-id", "client-secret", "https://example.test/cb")
}

func newRandomToken(t *testing.T) *auth.Token {
	t.Helper()
	return &auth.Token{
		ID:        auth.NewTokenID(),
		Secret:    auth.TokenSecret("tok-secret-" + time.Now().Format("150405.000000000")),
		Sub:       "U" + time.Now().Format("150405.000000000"),
		Email:     "user@example.test",
		Name:      "Test User",
		ExpiresAt: time.Now().Add(time.Hour),
		CreatedAt: time.Now(),
	}
}

func TestValidateToken_NotFoundIsTaggedBenign(t *testing.T) {
	// Missing-token (revoked / expired session / unauthenticated browser)
	// is the normal flow we explicitly want demoted from Sentry.
	repo := memory.New()
	uc := newAuthUC(t, repo)

	_, err := uc.ValidateToken(context.Background(), auth.NewTokenID(), auth.TokenSecret("whatever"))
	gt.Value(t, err).NotNil().Required()
	gt.Bool(t, goerr.HasTag(err, errutil.TagBenign)).True()
}

func TestValidateToken_RepoFailureIsNotTaggedBenign(t *testing.T) {
	// The usecase MUST keep paging Sentry on real backend failures even
	// though the same return path also handles the benign not-found case.
	// Without the err-type guard, every Firestore outage would silently
	// disappear into Info-level logs.
	backendBoom := goerr.New("simulated firestore outage")
	repo := &flakyAuthRepo{Repository: memory.New(), getTokenErr: backendBoom}
	uc := newAuthUC(t, repo)

	_, err := uc.ValidateToken(context.Background(), auth.NewTokenID(), auth.TokenSecret("any"))
	gt.Value(t, err).NotNil().Required()
	gt.Bool(t, goerr.HasTag(err, errutil.TagBenign)).False()
	gt.Bool(t, errors.Is(err, backendBoom)).True()
}

func TestValidateToken_CachedInvalidSecretIsTaggedBenign(t *testing.T) {
	// Wrong secret on a cached token is normal-flow (a stale cookie or a
	// browser tab from a different user) — should not page Sentry.
	repo := memory.New()
	uc := newAuthUC(t, repo)
	tok := newRandomToken(t)
	gt.NoError(t, repo.PutToken(context.Background(), tok)).Required()

	// Prime the cache with a valid call, then call again with wrong secret.
	_, err := uc.ValidateToken(context.Background(), tok.ID, tok.Secret)
	gt.NoError(t, err).Required()

	_, err = uc.ValidateToken(context.Background(), tok.ID, auth.TokenSecret("wrong"))
	gt.Value(t, err).NotNil().Required()
	gt.Bool(t, goerr.HasTag(err, errutil.TagBenign)).True()
}

func TestValidateToken_RepoInvalidSecretIsTaggedBenign(t *testing.T) {
	// Same idea but skipping the cache: the repo returns a token whose
	// secret does not match. Still benign.
	repo := memory.New()
	uc := newAuthUC(t, repo)
	tok := newRandomToken(t)
	gt.NoError(t, repo.PutToken(context.Background(), tok)).Required()

	_, err := uc.ValidateToken(context.Background(), tok.ID, auth.TokenSecret("wrong"))
	gt.Value(t, err).NotNil().Required()
	gt.Bool(t, goerr.HasTag(err, errutil.TagBenign)).True()
}

func TestValidateToken_ExpiredTokenIsTaggedBenign(t *testing.T) {
	// Token whose ExpiresAt is in the past: usecase deletes it from the
	// repo and returns a benign "token expired" error.
	repo := memory.New()
	uc := newAuthUC(t, repo)
	tok := newRandomToken(t)
	tok.ExpiresAt = time.Now().Add(-time.Minute)
	gt.NoError(t, repo.PutToken(context.Background(), tok)).Required()

	_, err := uc.ValidateToken(context.Background(), tok.ID, tok.Secret)
	gt.Value(t, err).NotNil().Required()
	gt.Bool(t, goerr.HasTag(err, errutil.TagBenign)).True()
}
