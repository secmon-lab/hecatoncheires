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

3. Under **Scopes** → **User Token Scopes** (NOT Bot Token Scopes), add:
   - `openid` (required for OpenID Connect)
   - `profile` (to get user's name and basic info)
   - `email` (to get user's email address)

   **Important**: These must be **User Token Scopes**, not Bot Token Scopes, because we're authenticating users, not installing a bot.

4. Click "Save Changes"

### 3. Get Credentials

1. Go to **Basic Information**
2. Under **App Credentials**, you'll find:
   - **Client ID**: Copy this value
   - **Client Secret**: Click "Show" and copy this value

### 4. Configure Environment Variables

Set the following environment variables:

```bash
export HECATONCHEIRES_BASE_URL="https://your-ngrok-id.ngrok.io"
export HECATONCHEIRES_SLACK_CLIENT_ID="your-client-id"
export HECATONCHEIRES_SLACK_CLIENT_SECRET="your-client-secret"
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
  --slack-client-secret="your-client-secret"
```

### 5. Start the Server

```bash
./hecatoncheires serve
```

If Slack authentication is properly configured, you'll see:
```
Slack authentication enabled
```

If any configuration is missing, the system will run in anonymous mode:
```
No authentication configured, running in anonymous mode
```

## Authentication Flow

### 1. Login

Navigate to your server's login endpoint:
```
https://your-server.com/api/auth/login
```
(or `https://your-ngrok-id.ngrok.io/api/auth/login` for local development)

This redirects you to Slack for authentication.

### 2. Authorization

1. Slack will ask you to authorize the app
2. After authorization, Slack redirects back to your callback URL
3. The server creates a session token and sets cookies

### 3. Access Protected Resources

After login, authentication tokens are stored in HTTPOnly cookies:
- `token_id`: Token identifier
- `token_secret`: Token secret (for verification)

These cookies are automatically sent with subsequent requests.

### 4. Check Authentication Status

```bash
curl https://your-server.com/api/auth/me
```

Response:
```json
{
  "id": "T-xxxxxxxxx",
  "sub": "U-xxxxxxxxx",
  "email": "user@example.com",
  "name": "User Name",
  "is_anonymous": false
}
```

### 5. Logout

Navigate to your server's logout endpoint:
```
https://your-server.com/api/auth/logout
```

This deletes the session token and clears cookies.

## Anonymous Mode (Development)

When Slack OAuth is not configured, the system runs in anonymous mode:

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

- Ensure the callback URL in Slack app settings exactly matches `HECATONCHEIRES_SLACK_CALLBACK_URL`
- Check for trailing slashes
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

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/auth/login` | GET | Initiates OAuth flow |
| `/api/auth/callback` | GET | OAuth callback handler |
| `/api/auth/logout` | GET | Logs out and deletes token |
| `/api/auth/me` | GET | Returns current user info |

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
| `HECATONCHEIRES_BASE_URL` | No* | - | Base URL of the application (e.g., `https://your-domain.com`) |
| `HECATONCHEIRES_SLACK_CLIENT_ID` | No* | - | Slack OAuth client ID |
| `HECATONCHEIRES_SLACK_CLIENT_SECRET` | No* | - | Slack OAuth client secret |

\* If any of these are missing, the system runs in anonymous mode. The callback URL is automatically constructed as `${BASE_URL}/api/auth/callback`.
