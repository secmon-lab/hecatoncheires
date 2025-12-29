package http

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/auth"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/safe"
)

type AuthUseCase = usecase.AuthUseCaseInterface

// generateState generates a random state parameter for OAuth
func generateState() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", goerr.Wrap(err, "failed to generate random state")
	}
	return hex.EncodeToString(bytes), nil
}

// authLoginHandler handles the OAuth login initiation
func authLoginHandler(authUC AuthUseCase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// For NoAuthn mode, redirect to home
		if authUC.IsNoAuthn() {
			http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
			return
		}

		// Generate state parameter to prevent CSRF
		state, err := generateState()
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Store state in session cookie for verification
		stateCookie := &http.Cookie{
			Name:     "oauth_state",
			Value:    state,
			Path:     "/",
			HttpOnly: true,
			Secure:   r.TLS != nil,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   600, // 10 minutes
		}
		http.SetCookie(w, stateCookie)

		// Redirect to Slack OAuth
		authURL := authUC.GetAuthURL(state)
		http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
	}
}

// authCallbackHandler handles the OAuth callback
func authCallbackHandler(authUC AuthUseCase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Verify state parameter
		stateCookie, err := r.Cookie("oauth_state")
		if err != nil {
			http.Error(w, "Missing state parameter", http.StatusBadRequest)
			return
		}

		state := r.URL.Query().Get("state")
		if state == "" || state != stateCookie.Value {
			http.Error(w, "Invalid state parameter", http.StatusBadRequest)
			return
		}

		// Clear state cookie
		clearCookie := &http.Cookie{
			Name:     "oauth_state",
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			Secure:   r.TLS != nil,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   -1,
		}
		http.SetCookie(w, clearCookie)

		// Get authorization code
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "Missing authorization code", http.StatusBadRequest)
			return
		}

		// Exchange code for token
		token, err := authUC.HandleCallback(r.Context(), code)
		if err != nil {
			logging.From(r.Context()).Error("Authentication failed", logging.ErrAttr(err))
			http.Error(w, "Authentication failed", http.StatusInternalServerError)
			return
		}

		// Set authentication cookies
		tokenIDCookie := &http.Cookie{
			Name:     "token_id",
			Value:    token.ID.String(),
			Path:     "/",
			HttpOnly: true,
			Secure:   r.TLS != nil,
			SameSite: http.SameSiteLaxMode,
			Expires:  token.ExpiresAt,
		}

		tokenSecretCookie := &http.Cookie{
			Name:     "token_secret",
			Value:    token.Secret.String(),
			Path:     "/",
			HttpOnly: true,
			Secure:   r.TLS != nil,
			SameSite: http.SameSiteLaxMode,
			Expires:  token.ExpiresAt,
		}

		http.SetCookie(w, tokenIDCookie)
		http.SetCookie(w, tokenSecretCookie)

		// Redirect to dashboard
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
	}
}

// authLogoutHandler handles user logout
func authLogoutHandler(authUC AuthUseCase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get token ID from cookie
		tokenIDCookie, err := r.Cookie("token_id")
		if err == nil {
			tokenID := auth.TokenID(tokenIDCookie.Value)
			if err := authUC.Logout(r.Context(), tokenID); err != nil {
				logging.From(r.Context()).Error("Failed to logout, but ignored", logging.ErrAttr(err))
			}
		}

		// Clear authentication cookies
		clearTokenID := &http.Cookie{
			Name:     "token_id",
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			Secure:   r.TLS != nil,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   -1,
		}

		clearTokenSecret := &http.Cookie{
			Name:     "token_secret",
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			Secure:   r.TLS != nil,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   -1,
		}

		http.SetCookie(w, clearTokenID)
		http.SetCookie(w, clearTokenSecret)

		w.WriteHeader(http.StatusOK)
		safe.Write(r.Context(), w, []byte(`{"success": true}`))
	}
}

// authUserInfoHandler returns Slack user information including avatar
func authUserInfoHandler(authUC AuthUseCase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := r.URL.Query().Get("user")
		if userID == "" {
			http.Error(w, `{"error": "user parameter required"}`, http.StatusBadRequest)
			return
		}

		// Type assertion to get *AuthUseCase
		concreteAuth, ok := authUC.(*usecase.AuthUseCase)
		if !ok {
			// NoAuthn mode - return placeholder
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			safe.Write(r.Context(), w, []byte(`{"id": "anonymous", "name": "Anonymous", "profile": {"image_48": ""}}`))
			return
		}

		userInfo, err := concreteAuth.GetSlackUserInfo(r.Context(), userID)
		if err != nil {
			logging.From(r.Context()).Error("Failed to get user info", logging.ErrAttr(err))
			http.Error(w, `{"error": "Failed to get user info"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		response := `{"id": "` + userInfo.ID + `", "name": "` + userInfo.RealName + `", "profile": {"image_48": "` + userInfo.Profile.Image48 + `"}}`
		safe.Write(r.Context(), w, []byte(response))
	}
}

// authMeHandler returns current user information
func authMeHandler(authUC AuthUseCase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// For NoAuthn mode, always return anonymous user
		if authUC.IsNoAuthn() {
			token := auth.NewAnonymousUser()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			userInfo := `{"sub": "` + token.Sub + `", "email": "` + token.Email + `", "name": "` + token.Name + `", "is_anonymous": true}`
			safe.Write(r.Context(), w, []byte(userInfo))
			return
		}
		// Get tokens from cookies
		tokenIDCookie, err := r.Cookie("token_id")
		if err != nil {
			http.Error(w, `{"error": "Not authenticated"}`, http.StatusUnauthorized)
			return
		}

		tokenSecretCookie, err := r.Cookie("token_secret")
		if err != nil {
			http.Error(w, `{"error": "Not authenticated"}`, http.StatusUnauthorized)
			return
		}

		tokenID := auth.TokenID(tokenIDCookie.Value)
		tokenSecret := auth.TokenSecret(tokenSecretCookie.Value)

		// Validate token
		token, err := authUC.ValidateToken(r.Context(), tokenID, tokenSecret)
		if err != nil {
			http.Error(w, `{"error": "Invalid token"}`, http.StatusUnauthorized)
			return
		}

		// Return user info with is_anonymous flag
		isAnonymous := "false"
		if token.IsAnonymous() {
			isAnonymous = "true"
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		userInfo := `{"sub": "` + token.Sub + `", "email": "` + token.Email + `", "name": "` + token.Name + `", "is_anonymous": ` + isAnonymous + `}`
		safe.Write(r.Context(), w, []byte(userInfo))
	}
}
