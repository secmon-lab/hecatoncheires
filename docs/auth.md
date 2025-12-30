# Authentication Configuration

Hecatoncheires supports authentication via Slack OAuth using OpenID Connect (OIDC). For complete Slack setup instructions, see [docs/slack.md](./slack.md).

The system can operate in two modes:

1. **Authenticated Mode**: Production authentication using Slack workspace
2. **Anonymous Mode**: Development mode with no authentication (default when Slack is not configured)

## Quick Start

For detailed Slack configuration, see [docs/slack.md](./slack.md#slack-oauth-authentication).

### Basic Setup

```bash
# Required for Slack authentication
export HECATONCHEIRES_BASE_URL="https://your-domain.com"
export HECATONCHEIRES_SLACK_CLIENT_ID="your-client-id"
export HECATONCHEIRES_SLACK_CLIENT_SECRET="your-client-secret"

# Optional: for displaying user avatars
export HECATONCHEIRES_SLACK_BOT_TOKEN="xoxb-your-bot-token"
```

Then start the server:

```bash
./hecatoncheires serve
```

## Authentication Flow

### 1. Login

The frontend automatically handles the login flow. When an unauthenticated user accesses the application:

1. The frontend's `AuthGuard` component detects unauthenticated state
2. It displays a login page with a "Sign in with Slack" button
3. Clicking the button redirects to `/api/auth/login`
4. The backend redirects to Slack for authentication

### 2. Authorization

1. Slack asks the user to authorize the app
2. After authorization, Slack redirects back to `/api/auth/callback`
3. The backend:
   - Validates the OAuth callback
   - Exchanges the authorization code for user tokens
   - Creates a session token
   - Sets HTTPOnly cookies (`token_id` and `token_secret`)
   - Redirects to `/` (home page)

### 3. Access Protected Resources

After login, authentication tokens are stored in HTTPOnly cookies:
- `token_id`: Token identifier
- `token_secret`: Token secret (for verification)

These cookies are automatically sent with subsequent requests. The backend middleware validates these tokens for all protected endpoints.

### 4. Check Authentication Status

The frontend uses `/api/auth/me` to check authentication status:

```bash
curl https://your-server.com/api/auth/me
```

Response for authenticated users:
```json
{
  "sub": "U-xxxxxxxxx",
  "email": "user@example.com",
  "name": "User Name",
  "is_anonymous": false
}
```

Response for anonymous mode:
```json
{
  "sub": "anonymous",
  "email": "anonymous@localhost",
  "name": "Anonymous",
  "is_anonymous": true
}
```

### 5. Logout

The frontend handles logout by calling `/api/auth/logout` (POST):

```bash
curl -X POST https://your-server.com/api/auth/logout
```

The backend:
1. Deletes the session token from storage
2. Clears authentication cookies
3. Returns success response

The frontend then redirects to `/` and shows the login page.

## Anonymous Mode (Development)

When Slack OAuth is not configured (missing `BASE_URL`, `CLIENT_ID`, or `CLIENT_SECRET`), the system runs in anonymous mode:

- No login required
- All requests are treated as anonymous user
- User info:
  - `sub`: `anonymous`
  - `email`: `anonymous@localhost`
  - `name`: `Anonymous`
  - `is_anonymous`: `true`

This is useful for:
- Local development
- Testing
- Environments where authentication is not needed

## Security Considerations

### Production Deployment

1. **HTTPS Required**: Always use HTTPS in production
   - Cookies are set with `Secure` flag when using HTTPS
   - Token secrets are transmitted securely

2. **Set Base URL**: Use your actual domain
   ```bash
   export HECATONCHEIRES_BASE_URL="https://your-domain.com"
   ```
   The callback URL (`/api/auth/callback`) is automatically appended

3. **Secure Token Storage**:
   - Tokens are stored with masked secrets in logs
   - HTTPOnly cookies prevent XSS attacks
   - SameSite=Lax for CSRF protection

4. **Token Expiration**:
   - Tokens expire after 7 days
   - Expired tokens are automatically deleted
   - Token cache TTL: 5 minutes

5. **Error Handling**:
   - Backend returns HTTP 401 for unauthenticated requests
   - Frontend handles authentication redirects
   - Backend never redirects to login page (frontend responsibility)

### Firestore Security Rules

If using Firestore for token storage, configure security rules:

```javascript
rules_version = '2';
service cloud.firestore {
  match /databases/{database}/documents {
    // Tokens collection - server-side access only
    match /tokens/{tokenId} {
      allow read, write: if false;  // Deny all client access
    }
  }
}
```

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/auth/login` | GET | Initiates OAuth flow (redirects to Slack) |
| `/api/auth/callback` | GET | OAuth callback handler (internal use) |
| `/api/auth/logout` | POST | Logs out and deletes token |
| `/api/auth/me` | GET | Returns current user info |
| `/api/auth/user-info` | GET | Returns Slack user profile (requires `user` query param) |

## Token Management

### Token Structure

```go
{
  "id": "unique-token-id",        // Public token identifier
  "secret": "secret-value",        // Secret for verification (masked in logs)
  "sub": "slack-user-id",         // Slack user ID
  "email": "user@example.com",    // User email
  "name": "User Name",            // User display name
  "expires_at": "2025-01-05...",  // Expiration timestamp (7 days)
  "created_at": "2024-12-29..."   // Creation timestamp
}
```

### Token Lifecycle

1. **Creation**: On successful OAuth callback
2. **Storage**: In Firestore or Memory (depending on configuration)
3. **Caching**: In-memory cache for 5 minutes (reduces DB load)
4. **Validation**: On each request via middleware
5. **Expiration**: Automatically after 7 days
6. **Deletion**: On logout or when expired

## Troubleshooting

For Slack-specific troubleshooting, see [docs/slack.md](./slack.md#troubleshooting).

### General Issues

#### Token verification fails

- Check system time synchronization (JWT verification is time-sensitive)
- Verify network access to `https://slack.com/.well-known/openid-configuration`

#### Anonymous mode when it shouldn't be

- Verify all required environment variables are set:
  - `HECATONCHEIRES_BASE_URL`
  - `HECATONCHEIRES_SLACK_CLIENT_ID`
  - `HECATONCHEIRES_SLACK_CLIENT_SECRET`
- Check for typos in variable names
- Ensure values are not empty strings
- Verify `BASE_URL` doesn't have a trailing slash

## Environment Variables Reference

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `HECATONCHEIRES_BASE_URL` | Yes* | - | Base URL of the application (e.g., `https://your-domain.com`). No trailing slash. |
| `HECATONCHEIRES_SLACK_CLIENT_ID` | Yes* | - | Slack OAuth client ID |
| `HECATONCHEIRES_SLACK_CLIENT_SECRET` | Yes* | - | Slack OAuth client secret |
| `HECATONCHEIRES_SLACK_BOT_TOKEN` | No | - | Slack Bot User OAuth Token (for fetching user avatars) |

\* If any of `BASE_URL`, `CLIENT_ID`, or `CLIENT_SECRET` are missing, the system runs in anonymous mode. The callback URL is automatically constructed as `${BASE_URL}/api/auth/callback`.

## See Also

- [Slack Integration Guide](./slack.md) - Complete Slack setup including OAuth and Webhooks
- [Slack Events API Configuration](./slack.md#slack-events-api-webhooks) - Setting up event webhooks
