package http

import (
	"net/http"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/auth"
)

// authMiddleware validates authentication for protected requests
func authMiddleware(authUC AuthUseCase) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// For NoAuthn mode or when authUC is not configured, always use anonymous user
			if authUC == nil || authUC.IsNoAuthn() {
				token := auth.NewAnonymousUser()
				ctx := auth.ContextWithToken(r.Context(), token)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Get tokens from cookies
			tokenIDCookie, err := r.Cookie("token_id")
			if err != nil {
				// No token cookie - return 401
				http.Error(w, "Authentication required", http.StatusUnauthorized)
				return
			}

			tokenSecretCookie, err := r.Cookie("token_secret")
			if err != nil {
				// No secret cookie - return 401
				http.Error(w, "Authentication required", http.StatusUnauthorized)
				return
			}

			tokenID := auth.TokenID(tokenIDCookie.Value)
			tokenSecret := auth.TokenSecret(tokenSecretCookie.Value)

			// Validate token
			token, err := authUC.ValidateToken(r.Context(), tokenID, tokenSecret)
			if err != nil {
				// Invalid token - return 401
				http.Error(w, "Invalid authentication token", http.StatusUnauthorized)
				return
			}

			// Add token to request context
			ctx := auth.ContextWithToken(r.Context(), token)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// graphqlAuthMiddleware validates authentication for GraphQL requests specifically
func graphqlAuthMiddleware(authUC AuthUseCase) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// For NoAuthn mode or when authUC is not configured, always use anonymous user
			if authUC == nil || authUC.IsNoAuthn() {
				token := auth.NewAnonymousUser()
				ctx := auth.ContextWithToken(r.Context(), token)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Get tokens from cookies
			tokenIDCookie, err := r.Cookie("token_id")
			if err != nil {
				http.Error(w, `{"errors": [{"message": "Authentication required"}]}`, http.StatusUnauthorized)
				return
			}

			tokenSecretCookie, err := r.Cookie("token_secret")
			if err != nil {
				http.Error(w, `{"errors": [{"message": "Authentication required"}]}`, http.StatusUnauthorized)
				return
			}

			tokenID := auth.TokenID(tokenIDCookie.Value)
			tokenSecret := auth.TokenSecret(tokenSecretCookie.Value)

			// Validate token
			token, err := authUC.ValidateToken(r.Context(), tokenID, tokenSecret)
			if err != nil {
				http.Error(w, `{"errors": [{"message": "Invalid authentication token"}]}`, http.StatusUnauthorized)
				return
			}

			// Add token to request context
			ctx := auth.ContextWithToken(r.Context(), token)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
