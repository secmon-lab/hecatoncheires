package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/m-mizutani/gt"
	httpctrl "github.com/secmon-lab/hecatoncheires/pkg/controller/http"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/auth"
)

// fakeAuthUC is a minimal in-memory stand-in for usecase.AuthUseCaseInterface
// that exercises authLoginHandler and authCallbackHandler without touching
// Slack or any persistence backend.
type fakeAuthUC struct {
	isNoAuthn        bool
	authURL          string
	handleCallbackFn func(ctx context.Context, code string) (*auth.Token, error)
}

func (f *fakeAuthUC) GetAuthURL(state string) string {
	return f.authURL + "?state=" + state
}

func (f *fakeAuthUC) HandleCallback(ctx context.Context, code string) (*auth.Token, error) {
	if f.handleCallbackFn != nil {
		return f.handleCallbackFn(ctx, code)
	}
	return auth.NewToken("U1", "user@example.com", "Test User"), nil
}

func (f *fakeAuthUC) ValidateToken(ctx context.Context, id auth.TokenID, secret auth.TokenSecret) (*auth.Token, error) {
	return auth.NewToken("U1", "user@example.com", "Test User"), nil
}

func (f *fakeAuthUC) Logout(ctx context.Context, id auth.TokenID) error {
	return nil
}

func (f *fakeAuthUC) IsNoAuthn() bool { return f.isNoAuthn }

// cookiesByName returns every Set-Cookie entry from the recorded response
// whose name matches. Callback flows can emit two entries for the same
// cookie (set + clear), so callers may need both.
func cookiesByName(rec *httptest.ResponseRecorder, name string) []*http.Cookie {
	var out []*http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			out = append(out, c)
		}
	}
	return out
}

func firstCookie(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	cs := cookiesByName(rec, name)
	if len(cs) == 0 {
		return nil
	}
	return cs[0]
}

func TestValidateReturnTo(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"empty", "", false},
		{"plain root", "/", true},
		{"normal path", "/ws/abc/cases/xyz", true},
		{"with query", "/ws/abc/cases/xyz?tab=actions", true},
		{"with fragment", "/ws/abc/cases/xyz#top", true},
		{"with query and fragment", "/ws/abc/cases/xyz?tab=actions#top", true},
		{"protocol relative", "//evil.example.com/", false},
		{"protocol relative deep", "//evil.example.com/path", false},
		{"backslash trick", `/\evil.example.com`, false},
		{"absolute http", "http://evil.example.com/", false},
		{"absolute https", "https://evil.example.com/", false},
		{"missing leading slash", "ws/abc/cases/xyz", false},
		{"tab control char", "/ws/abc\t/cases", false},
		{"newline control char", "/ws/abc\n/cases", false},
		{"DEL control char", "/ws/abc\x7f", false},
		{"too long", "/" + strings.Repeat("a", 2048), false},
		{"max length boundary OK", "/" + strings.Repeat("a", 2047), true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := httpctrl.ValidateReturnToForTest(tc.in)
			gt.Value(t, got).Equal(tc.want)
		})
	}
}

func TestAuthLoginHandler(t *testing.T) {
	t.Run("valid return_to sets oauth_return_to cookie and redirects to auth URL", func(t *testing.T) {
		uc := &fakeAuthUC{authURL: "https://slack.example/auth"}
		h := httpctrl.AuthLoginHandlerForTest(uc)

		req := httptest.NewRequest(http.MethodGet, "/api/auth/login?return_to=/ws/abc/cases/xyz", nil)
		rec := httptest.NewRecorder()
		h(rec, req)

		gt.Number(t, rec.Code).Equal(http.StatusTemporaryRedirect)
		gt.String(t, rec.Header().Get("Location")).Contains("https://slack.example/auth?state=")

		stateCookie := firstCookie(rec, "oauth_state")
		gt.Value(t, stateCookie).NotNil()
		gt.String(t, stateCookie.Value).NotEqual("")

		rt := firstCookie(rec, httpctrl.ReturnToCookieNameForTest)
		gt.Value(t, rt).NotNil()
		gt.String(t, rt.Value).Equal("/ws/abc/cases/xyz")
		gt.Bool(t, rt.HttpOnly).True()
		gt.Number(t, rt.MaxAge).Equal(600)
	})

	t.Run("missing return_to does not set the cookie", func(t *testing.T) {
		uc := &fakeAuthUC{authURL: "https://slack.example/auth"}
		h := httpctrl.AuthLoginHandlerForTest(uc)

		req := httptest.NewRequest(http.MethodGet, "/api/auth/login", nil)
		rec := httptest.NewRecorder()
		h(rec, req)

		gt.Number(t, rec.Code).Equal(http.StatusTemporaryRedirect)
		gt.Value(t, firstCookie(rec, "oauth_state")).NotNil()
		gt.Value(t, firstCookie(rec, httpctrl.ReturnToCookieNameForTest)).Nil()
	})

	t.Run("invalid return_to is silently dropped", func(t *testing.T) {
		uc := &fakeAuthUC{authURL: "https://slack.example/auth"}
		h := httpctrl.AuthLoginHandlerForTest(uc)

		req := httptest.NewRequest(http.MethodGet, "/api/auth/login?return_to=//evil.example.com/", nil)
		rec := httptest.NewRecorder()
		h(rec, req)

		gt.Number(t, rec.Code).Equal(http.StatusTemporaryRedirect)
		gt.Value(t, firstCookie(rec, "oauth_state")).NotNil()
		gt.Value(t, firstCookie(rec, httpctrl.ReturnToCookieNameForTest)).Nil()
	})

	t.Run("no-auth mode honours valid return_to", func(t *testing.T) {
		uc := &fakeAuthUC{isNoAuthn: true}
		h := httpctrl.AuthLoginHandlerForTest(uc)

		req := httptest.NewRequest(http.MethodGet, "/api/auth/login?return_to=/ws/abc/cases/xyz", nil)
		rec := httptest.NewRecorder()
		h(rec, req)

		gt.Number(t, rec.Code).Equal(http.StatusTemporaryRedirect)
		gt.String(t, rec.Header().Get("Location")).Equal("/ws/abc/cases/xyz")
		gt.Value(t, firstCookie(rec, httpctrl.ReturnToCookieNameForTest)).Nil()
	})

	t.Run("no-auth mode rejects invalid return_to and falls back to /", func(t *testing.T) {
		uc := &fakeAuthUC{isNoAuthn: true}
		h := httpctrl.AuthLoginHandlerForTest(uc)

		req := httptest.NewRequest(http.MethodGet, "/api/auth/login?return_to=//evil.example.com/", nil)
		rec := httptest.NewRecorder()
		h(rec, req)

		gt.Number(t, rec.Code).Equal(http.StatusTemporaryRedirect)
		gt.String(t, rec.Header().Get("Location")).Equal("/")
	})

	t.Run("no-auth mode without return_to falls back to /", func(t *testing.T) {
		uc := &fakeAuthUC{isNoAuthn: true}
		h := httpctrl.AuthLoginHandlerForTest(uc)

		req := httptest.NewRequest(http.MethodGet, "/api/auth/login", nil)
		rec := httptest.NewRecorder()
		h(rec, req)

		gt.Number(t, rec.Code).Equal(http.StatusTemporaryRedirect)
		gt.String(t, rec.Header().Get("Location")).Equal("/")
	})
}

func TestAuthCallbackHandler(t *testing.T) {
	// Each subtest crafts the request with matching state cookie + state
	// query so the CSRF check passes, since the callback is interesting
	// only past that point.
	const stateValue = "abc123"

	makeCallbackRequest := func(returnToCookie string) *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/api/auth/callback?code=ok&state="+stateValue, nil)
		req.AddCookie(&http.Cookie{Name: "oauth_state", Value: stateValue})
		if returnToCookie != "" {
			req.AddCookie(&http.Cookie{Name: httpctrl.ReturnToCookieNameForTest, Value: returnToCookie})
		}
		return req
	}

	t.Run("valid oauth_return_to cookie redirects to its value and clears the cookie", func(t *testing.T) {
		uc := &fakeAuthUC{}
		h := httpctrl.AuthCallbackHandlerForTest(uc)

		rec := httptest.NewRecorder()
		h(rec, makeCallbackRequest("/ws/abc/cases/xyz"))

		gt.Number(t, rec.Code).Equal(http.StatusTemporaryRedirect)
		gt.String(t, rec.Header().Get("Location")).Equal("/ws/abc/cases/xyz")

		clears := cookiesByName(rec, httpctrl.ReturnToCookieNameForTest)
		gt.Array(t, clears).Length(1).Required()
		gt.Number(t, clears[0].MaxAge).Equal(-1)
		gt.String(t, clears[0].Value).Equal("")
	})

	t.Run("missing oauth_return_to cookie redirects to / and still emits a clear", func(t *testing.T) {
		uc := &fakeAuthUC{}
		h := httpctrl.AuthCallbackHandlerForTest(uc)

		rec := httptest.NewRecorder()
		h(rec, makeCallbackRequest(""))

		gt.Number(t, rec.Code).Equal(http.StatusTemporaryRedirect)
		gt.String(t, rec.Header().Get("Location")).Equal("/")

		// The clear is emitted unconditionally so that any stale or
		// duplicate cookie carried by the browser is wiped on every
		// callback, not only when the server happened to read one.
		clears := cookiesByName(rec, httpctrl.ReturnToCookieNameForTest)
		gt.Array(t, clears).Length(1).Required()
		gt.Number(t, clears[0].MaxAge).Equal(-1)
		gt.String(t, clears[0].Value).Equal("")
	})

	t.Run("invalid oauth_return_to cookie redirects to / and still clears the cookie", func(t *testing.T) {
		uc := &fakeAuthUC{}
		h := httpctrl.AuthCallbackHandlerForTest(uc)

		rec := httptest.NewRecorder()
		h(rec, makeCallbackRequest("//evil.example.com/"))

		gt.Number(t, rec.Code).Equal(http.StatusTemporaryRedirect)
		gt.String(t, rec.Header().Get("Location")).Equal("/")

		clears := cookiesByName(rec, httpctrl.ReturnToCookieNameForTest)
		gt.Array(t, clears).Length(1).Required()
		gt.Number(t, clears[0].MaxAge).Equal(-1)
	})
}
