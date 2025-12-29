# Authentication Configuration

Hecatoncheires supports Slack OAuth authentication via OpenID Connect (OIDC). The system can operate in two modes:

1. **Slack OAuth Mode**: Production authentication using Slack workspace
2. **Anonymous Mode**: Development mode with no authentication (default when Slack is not configured)

## Slack OAuth Setup

### 1. Create a Slack App

1. Go to [https://api.slack.com/apps](https://api.slack.com/apps)
2. Click "Create New App" → "From scratch"
3. Enter your app name and select your workspace
4. Click "Create App"

### 2. Configure OAuth & Permissions

1. In your app settings, go to **OAuth & Permissions**
2. Under **Redirect URLs**, add your callback URL:

   For local development, use a tunneling service like ngrok:
   ```
   https://your-ngrok-id.ngrok.io/api/auth/callback
   ```

   For production, use your actual domain:
   ```
   https://your-domain.com/api/auth/callback
   ```

   **Note**: Slack requires HTTPS for OAuth callbacks. HTTP URLs (including `http://localhost`) are not supported.

3. Under **Scopes**:
   - **User Token Scopes** (required for user authentication):
     - `openid` (required for OpenID Connect)
     - `profile` (to get user's name and basic info)
     - `email` (to get user's email address)

   - **Bot Token Scopes** (optional, for displaying user avatars):
     - `users:read` (to fetch user profile information including avatar images)

   **Important**: User Token Scopes and Bot Token Scopes serve different purposes:
   - User Token Scopes authenticate users via OpenID Connect
   - Bot Token Scopes allow the application to fetch additional user information using the Bot Token

4. Click "Save Changes"

### 3. Install App to Workspace (for Bot Token)

If you want to display user avatars, you need to install the app to your workspace to get a Bot Token:

1. Go to **Install App** in the left sidebar
2. Click "Install to Workspace"
3. Review and authorize the permissions
4. After installation, you'll see **Bot User OAuth Token** - copy this value

**Note**: The Bot Token is only needed if you want to fetch user avatars. The application works without it, but avatars won't be displayed.

### 4. Get Credentials

1. Go to **Basic Information**
2. Under **App Credentials**, you'll find:
   - **Client ID**: Copy this value
   - **Client Secret**: Click "Show" and copy this value

3. If you installed the app, go to **OAuth & Permissions**:
   - **Bot User OAuth Token**: Copy this value (starts with `xoxb-`)

### 5. Configure Environment Variables

Set the following environment variables:

```bash
# Required for Slack authentication
export HECATONCHEIRES_BASE_URL="https://your-ngrok-id.ngrok.io"
export HECATONCHEIRES_SLACK_CLIENT_ID="your-client-id"
export HECATONCHEIRES_SLACK_CLIENT_SECRET="your-client-secret"

# Optional: for displaying user avatars
export HECATONCHEIRES_SLACK_BOT_TOKEN="xoxb-your-bot-token"
```

For local development with ngrok:
1. Start ngrok: `ngrok http 8080`
2. Set `HECATONCHEIRES_BASE_URL` to the HTTPS URL provided by ngrok (without trailing slash)
3. The callback URL will be automatically constructed as `${BASE_URL}/api/auth/callback`

Or use CLI flags:

```bash
./hecatoncheires serve \
  --base-url="https://your-ngrok-id.ngrok.io" \
  --slack-client-id="your-client-id" \
  --slack-client-secret="your-client-secret" \
  --slack-bot-token="xoxb-your-bot-token"
```

### 6. Start the Server

```bash
./hecatoncheires serve
```

If Slack authentication is properly configured, you'll see:
```
Slack authentication enabled
```

If any required configuration is missing, the system will run in anonymous mode:
```
No authentication configured, running in anonymous mode
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

### 5. User Avatar

If `HECATONCHEIRES_SLACK_BOT_TOKEN` is configured, the frontend fetches user avatar from:

```bash
curl https://your-server.com/api/auth/user-info?user=U-xxxxxxxxx
```

Response:
```json
{
  "id": "U-xxxxxxxxx",
  "name": "User Name",
  "profile": {
    "image_48": "https://avatars.slack-edge.com/..."
  }
}
```

The backend uses Slack's `users.info` API with the Bot Token to fetch this information.

### 6. Logout

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

## Troubleshooting

### Login fails with "invalid_client"

- Verify `HECATONCHEIRES_SLACK_CLIENT_ID` and `HECATONCHEIRES_SLACK_CLIENT_SECRET`
- Check that the client secret hasn't been regenerated in Slack

### Callback fails with "redirect_uri_mismatch"

- Ensure the callback URL in Slack app settings exactly matches `${HECATONCHEIRES_BASE_URL}/api/auth/callback`
- Check for trailing slashes (BASE_URL should not have trailing slash)
- Verify you're using HTTPS (Slack does not accept HTTP URLs)

### Token verification fails

- Check system time synchronization (JWT verification is time-sensitive)
- Verify network access to `https://slack.com/.well-known/openid-configuration`

### Anonymous mode when it shouldn't be

- Verify all required environment variables are set:
  - `HECATONCHEIRES_BASE_URL`
  - `HECATONCHEIRES_SLACK_CLIENT_ID`
  - `HECATONCHEIRES_SLACK_CLIENT_SECRET`
- Check for typos in variable names
- Ensure values are not empty strings
- Verify `BASE_URL` doesn't have a trailing slash

### User avatars not displaying

- Verify `HECATONCHEIRES_SLACK_BOT_TOKEN` is set
- Ensure the app is installed to your workspace
- Verify the bot token has `users:read` scope
- Check that the bot token starts with `xoxb-`

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

## Environment Variables Reference

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `HECATONCHEIRES_BASE_URL` | Yes* | - | Base URL of the application (e.g., `https://your-domain.com`). No trailing slash. |
| `HECATONCHEIRES_SLACK_CLIENT_ID` | Yes* | - | Slack OAuth client ID |
| `HECATONCHEIRES_SLACK_CLIENT_SECRET` | Yes* | - | Slack OAuth client secret |
| `HECATONCHEIRES_SLACK_BOT_TOKEN` | No | - | Slack Bot User OAuth Token (for fetching user avatars) |

\* If any of `BASE_URL`, `CLIENT_ID`, or `CLIENT_SECRET` are missing, the system runs in anonymous mode. The callback URL is automatically constructed as `${BASE_URL}/api/auth/callback`.
