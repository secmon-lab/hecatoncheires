package http

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/auth"
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
)

type AuthUseCase = usecase.AuthUseCaseInterface
type slackService = slack.Service

type userInfoResponse struct {
	ID      string        `json:"id"`
	Name    string        `json:"name"`
	Profile profileImages `json:"profile"`
}

type profileImages struct {
	Image48 string `json:"image_48"`
}

type userMeResponse struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

type errorResponse struct {
	Error string `json:"error"`
}

type successResponse struct {
	Success bool `json:"success"`
}

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
			errutil.HandleHTTP(r.Context(), w, err, http.StatusInternalServerError)
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
			errutil.HandleHTTP(r.Context(), w, err, http.StatusBadRequest)
			return
		}

		state := r.URL.Query().Get("state")
		if state == "" || state != stateCookie.Value {
			errutil.HandleHTTP(r.Context(), w, goerr.New("invalid state parameter"), http.StatusBadRequest)
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
			errutil.HandleHTTP(r.Context(), w, goerr.New("missing authorization code"), http.StatusBadRequest)
			return
		}

		// Exchange code for token
		token, err := authUC.HandleCallback(r.Context(), code)
		if err != nil {
			errutil.HandleHTTP(r.Context(), w, err, http.StatusInternalServerError)
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
				errutil.HandleHTTP(r.Context(), w, goerr.Wrap(err, "failed to logout"), http.StatusInternalServerError)
				return
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

		writeJSON(r.Context(), w, http.StatusOK, successResponse{Success: true})
	}
}

// writeJSON writes a JSON response with proper error handling
func writeJSON(ctx context.Context, w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		errutil.Handle(ctx, err, "failed to encode JSON response")
	}
}

// authUserInfoHandler returns Slack user information including avatar
func authUserInfoHandler(slackSvc slackService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := r.URL.Query().Get("user")
		if userID == "" {
			writeJSON(r.Context(), w, http.StatusBadRequest, errorResponse{Error: "user parameter required"})
			return
		}

		// If Slack service is not configured, return placeholder
		if slackSvc == nil {
			writeJSON(r.Context(), w, http.StatusOK, userInfoResponse{
				ID:   userID,
				Name: "Unknown",
				Profile: profileImages{
					Image48: "",
				},
			})
			return
		}

		userInfo, err := slackSvc.GetUserInfo(r.Context(), userID)
		if err != nil {
			errutil.HandleHTTP(r.Context(), w, err, http.StatusInternalServerError)
			return
		}

		writeJSON(r.Context(), w, http.StatusOK, userInfoResponse{
			ID:   userInfo.ID,
			Name: userInfo.RealName,
			Profile: profileImages{
				Image48: userInfo.ImageURL,
			},
		})
	}
}

// authMeHandler returns current user information
func authMeHandler(authUC AuthUseCase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// For NoAuthn mode, get user info from ValidateToken (which returns the configured user)
		if authUC.IsNoAuthn() {
			token, err := authUC.ValidateToken(r.Context(), "", "")
			if err != nil {
				errutil.HandleHTTP(r.Context(), w, err, http.StatusInternalServerError)
				return
			}
			writeJSON(r.Context(), w, http.StatusOK, userMeResponse{
				Sub:   token.Sub,
				Email: token.Email,
				Name:  token.Name,
			})
			return
		}
		// Get tokens from cookies
		tokenIDCookie, err := r.Cookie("token_id")
		if err != nil {
			errutil.HandleHTTP(r.Context(), w, err, http.StatusUnauthorized)
			return
		}

		tokenSecretCookie, err := r.Cookie("token_secret")
		if err != nil {
			errutil.HandleHTTP(r.Context(), w, err, http.StatusUnauthorized)
			return
		}

		tokenID := auth.TokenID(tokenIDCookie.Value)
		tokenSecret := auth.TokenSecret(tokenSecretCookie.Value)

		// Validate token
		token, err := authUC.ValidateToken(r.Context(), tokenID, tokenSecret)
		if err != nil {
			errutil.HandleHTTP(r.Context(), w, err, http.StatusUnauthorized)
			return
		}

		// Return user info
		writeJSON(r.Context(), w, http.StatusOK, userMeResponse{
			Sub:   token.Sub,
			Email: token.Email,
			Name:  token.Name,
		})
	}
}
