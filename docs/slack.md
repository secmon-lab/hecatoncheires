# Slack Integration

Hecatoncheires integrates with Slack for both authentication and event webhooks. This document covers the complete Slack setup process.

## Table of Contents

1. [Slack OAuth Authentication](#slack-oauth-authentication)
2. [Slack Events API (Webhooks)](#slack-events-api-webhooks)
3. [Complete Setup Guide](#complete-setup-guide)
4. [Environment Variables Reference](#environment-variables-reference)

---

## Slack OAuth Authentication

Slack OAuth is used for user authentication via OpenID Connect (OIDC). The system can operate in two modes:

1. **Slack OAuth Mode**: Production authentication using Slack workspace
2. **Anonymous Mode**: Development mode with no authentication (default when Slack is not configured)

### Authentication Setup

#### 1. Create a Slack App

1. Go to [https://api.slack.com/apps](https://api.slack.com/apps)
2. Click "Create New App" → "From scratch"
3. Enter your app name and select your workspace
4. Click "Create App"

#### 2. Configure OAuth & Permissions

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

#### 3. Get OAuth Credentials

1. Go to **Basic Information**
2. Under **App Credentials**, you'll find:
   - **Client ID**: Copy this value
   - **Client Secret**: Click "Show" and copy this value

---

## Slack Events API (Webhooks)

Slack Events API allows Hecatoncheires to receive real-time events from your Slack workspace, such as messages in channels.

### Events API Setup

#### 1. Enable Event Subscriptions

1. In your app settings, go to **Event Subscriptions**
2. Toggle **Enable Events** to **On**

#### 2. Set Request URL

Enter your webhook endpoint URL:

For local development with ngrok:
```
https://your-ngrok-id.ngrok.io/hooks/slack/event
```

For production:
```
https://your-domain.com/hooks/slack/event
```

**Note**:
- Slack requires HTTPS
- Slack will send a verification challenge immediately - your app must be running with the correct signing secret configured

#### 3. Subscribe to Bot Events

Under **Subscribe to bot events**, add the events you want to receive:

- `message.channels` - Messages posted to public channels the app is in
- `message.groups` - Messages posted to private channels the app is in
- `message.im` - Messages posted in direct message channels with the app
- `message.mpim` - Messages posted in multi-person direct messages the app is in
- `app_mention` - When someone mentions your app with @app_name

Click **Save Changes**

#### 4. Get Signing Secret

1. Go to **Basic Information**
2. Under **App Credentials**, find **Signing Secret**
3. Click "Show" and copy this value

This secret is used to verify that webhook requests actually come from Slack.

#### 5. Install App to Workspace

1. Go to **Install App** in the left sidebar
2. Click "Install to Workspace"
3. Review and authorize the permissions
4. After installation, you'll see **Bot User OAuth Token** - copy this value

**Note**:
- The Bot Token is needed for both fetching user avatars (authentication) and for the Events API
- The Bot Token starts with `xoxb-`

#### 6. Invite Bot to Channels

For the bot to receive messages from channels, you need to invite it:

1. Go to the Slack channel where you want to receive events
2. Type `/invite @your-bot-name`
3. The bot will now receive message events from that channel

---

## Complete Setup Guide

### Step-by-Step Configuration

Follow these steps to set up both authentication and webhooks:

1. **Create Slack App** (see [Create a Slack App](#1-create-a-slack-app))

2. **Configure OAuth** (see [Configure OAuth & Permissions](#2-configure-oauth--permissions))
   - Set redirect URL: `${BASE_URL}/api/auth/callback`
   - Add user scopes: `openid`, `profile`, `email`
   - Add bot scope: `users:read`

3. **Configure Events API** (see [Events API Setup](#events-api-setup))
   - Enable Event Subscriptions
   - Set request URL: `${BASE_URL}/hooks/slack/event`
   - Subscribe to bot events: `message.channels`, `app_mention`, etc.

4. **Get Credentials**:
   - Client ID (from Basic Information)
   - Client Secret (from Basic Information)
   - Signing Secret (from Basic Information)

5. **Install App to Workspace**:
   - Install the app
   - Copy Bot User OAuth Token (`xoxb-...`)

6. **Set Environment Variables** (see below)

7. **Start the Server**:
   ```bash
   ./hecatoncheires serve
   ```

8. **Verify Setup**:
   - Check logs for "Slack authentication enabled"
   - Check logs for "Slack webhook handler enabled"
   - Test authentication by accessing the web UI
   - Test webhook by posting a message in a channel (after inviting the bot)

### Environment Variables

Set the following environment variables:

```bash
# Required for Slack authentication
export HECATONCHEIRES_BASE_URL="https://your-ngrok-id.ngrok.io"
export HECATONCHEIRES_SLACK_CLIENT_ID="your-client-id"
export HECATONCHEIRES_SLACK_CLIENT_SECRET="your-client-secret"

# Required for Slack webhooks
export HECATONCHEIRES_SLACK_SIGNING_SECRET="your-signing-secret"

# Required for both (fetching user info and bot operations)
export HECATONCHEIRES_SLACK_BOT_TOKEN="xoxb-your-bot-token"
```

For local development with ngrok:
1. Start ngrok: `ngrok http 8080`
2. Set `HECATONCHEIRES_BASE_URL` to the HTTPS URL provided by ngrok (without trailing slash)
3. Update both OAuth redirect URL and Events request URL in Slack app settings

Or use CLI flags:

```bash
./hecatoncheires serve \
  --base-url="https://your-ngrok-id.ngrok.io" \
  --slack-client-id="your-client-id" \
  --slack-client-secret="your-client-secret" \
  --slack-signing-secret="your-signing-secret" \
  --slack-bot-token="xoxb-your-bot-token"
```

---

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

### 4. Logout

The frontend handles logout by calling `/api/auth/logout` (POST). The backend deletes the session token and clears cookies.

---

## Webhook Event Processing

When a message is posted in a channel where your bot is present:

1. Slack sends a POST request to `/hooks/slack/event`
2. The middleware verifies the request signature using the signing secret
3. If verification succeeds, the handler:
   - Responds immediately with HTTP 200 (within 3 seconds as required by Slack)
   - Processes the event asynchronously in the background
4. The event is parsed and the message is saved to the database

### Supported Events

Currently supported event types:

- `message` - Regular channel messages
- `app_mention` - When someone @mentions your app

Messages are stored with:
- Channel ID
- User ID and name
- Message text
- Timestamp
- Thread information (if it's a threaded message)

### Message Storage

Messages are stored in Firestore using a subcollection structure:

```
slack_channels/{channelID}/
  - metadata (channel info, last message time, message count)
  messages/{messageID}
    - message data
```

This structure allows efficient querying and pagination per channel.

---

## Anonymous Mode (Development)

When Slack OAuth is not configured (missing `BASE_URL`, `CLIENT_ID`, or `CLIENT_SECRET`), the system runs in anonymous mode:

- No login required
- All requests are treated as anonymous user
- User info:
  - `sub`: `anonymous`
  - `email`: `anonymous@localhost`
  - `name`: `Anonymous`
  - `is_anonymous`: `true`

This is useful for local development and testing.

---

## Security Considerations

### Production Deployment

1. **HTTPS Required**: Always use HTTPS in production
   - OAuth callbacks require HTTPS
   - Webhook endpoints require HTTPS
   - Cookies are set with `Secure` flag when using HTTPS

2. **Signing Secret Verification**:
   - All webhook requests are verified using HMAC-SHA256
   - Timestamp verification prevents replay attacks (5-minute window)
   - Requests with invalid signatures are rejected with HTTP 401

3. **Secure Token Storage**:
   - Tokens are stored with masked secrets in logs
   - HTTPOnly cookies prevent XSS attacks
   - SameSite=Lax for CSRF protection

4. **Token Expiration**:
   - Session tokens expire after 7 days
   - Expired tokens are automatically deleted
   - Token cache TTL: 5 minutes

### Firestore Security Rules

If using Firestore for storage, configure security rules:

```javascript
rules_version = '2';
service cloud.firestore {
  match /databases/{database}/documents {
    // Tokens collection - server-side access only
    match /tokens/{tokenId} {
      allow read, write: if false;
    }

    // Slack messages - server-side access only
    match /slack_channels/{channelId} {
      allow read, write: if false;
      match /messages/{messageId} {
        allow read, write: if false;
      }
    }
  }
}
```

---

## Troubleshooting

### Authentication Issues

#### Login fails with "invalid_client"
- Verify `HECATONCHEIRES_SLACK_CLIENT_ID` and `HECATONCHEIRES_SLACK_CLIENT_SECRET`
- Check that the client secret hasn't been regenerated in Slack

#### Callback fails with "redirect_uri_mismatch"
- Ensure the callback URL in Slack app settings exactly matches `${HECATONCHEIRES_BASE_URL}/api/auth/callback`
- Check for trailing slashes (BASE_URL should not have trailing slash)
- Verify you're using HTTPS

#### Anonymous mode when it shouldn't be
- Verify all required environment variables are set
- Check for typos in variable names
- Ensure values are not empty strings

### Webhook Issues

#### Webhook verification fails
- Verify `HECATONCHEIRES_SLACK_SIGNING_SECRET` is correct
- Ensure the signing secret matches the one in Slack app settings
- Check server logs for signature verification errors

#### Events not being received
- Verify the app is installed to the workspace
- Check that the bot is invited to the channels (`/invite @bot-name`)
- Verify Event Subscriptions are enabled in Slack app settings
- Check that you've subscribed to the correct bot events
- Ensure the request URL is correctly set in Slack

#### Request URL verification fails
- Make sure the server is running with correct signing secret
- Verify the endpoint is accessible from the internet (check ngrok URL)
- Check server logs for incoming requests

### General Issues

#### User avatars not displaying
- Verify `HECATONCHEIRES_SLACK_BOT_TOKEN` is set
- Ensure the app is installed to your workspace
- Verify the bot token has `users:read` scope
- Check that the bot token starts with `xoxb-`

---

## API Endpoints

### Authentication Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/auth/login` | GET | Initiates OAuth flow (redirects to Slack) |
| `/api/auth/callback` | GET | OAuth callback handler (internal use) |
| `/api/auth/logout` | POST | Logs out and deletes token |
| `/api/auth/me` | GET | Returns current user info |
| `/api/auth/user-info` | GET | Returns Slack user profile (requires `user` query param) |

### Webhook Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/hooks/slack/event` | POST | Receives Slack Events API webhooks |

All webhook endpoints require valid Slack signature verification.

---

## Environment Variables Reference

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `HECATONCHEIRES_BASE_URL` | Yes* | - | Base URL of the application (e.g., `https://your-domain.com`). No trailing slash. |
| `HECATONCHEIRES_SLACK_CLIENT_ID` | Yes* | - | Slack OAuth client ID |
| `HECATONCHEIRES_SLACK_CLIENT_SECRET` | Yes* | - | Slack OAuth client secret |
| `HECATONCHEIRES_SLACK_BOT_TOKEN` | No | - | Slack Bot User OAuth Token (starts with `xoxb-`) |
| `HECATONCHEIRES_SLACK_SIGNING_SECRET` | Yes** | - | Slack Events API signing secret |

\* If any of `BASE_URL`, `CLIENT_ID`, or `CLIENT_SECRET` are missing, authentication runs in anonymous mode.

\** Required only if you want to enable Slack webhook integration. Without this, webhook endpoints will not be enabled.

---

## Message Storage and Retrieval

Messages received from Slack webhooks are stored in Firestore and can be queried via GraphQL (when implemented) or directly through the repository layer.

### Storage Structure

```
slack_channels/{channelID}
  - channel_id: string
  - team_id: string
  - last_message_at: timestamp
  - message_count: integer

  messages/{messageID}
    - id: string (message timestamp)
    - thread_ts: string (thread timestamp, if threaded)
    - user_id: string
    - user_name: string
    - text: string
    - event_ts: string
    - created_at: timestamp
```

### Message Lifecycle

1. **Reception**: Webhook receives event from Slack
2. **Verification**: Signature is verified
3. **Async Processing**: Event is processed in background
4. **Storage**: Message is saved to Firestore
5. **Metadata Update**: Channel metadata is updated (last message time, count)

### Future Features

- GraphQL queries for message retrieval
- Message search functionality
- Threaded conversation display
- User mention notifications
- Message reactions support
