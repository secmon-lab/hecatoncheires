# Slack Integration

Hecatoncheires integrates with Slack for both authentication and event webhooks. This document covers the complete Slack setup process.

## Table of Contents

1. [Slack OAuth Authentication](#slack-oauth-authentication)
2. [Slack Events API (Webhooks)](#slack-events-api-webhooks)
3. [Slack Interactivity (Action Notifications)](#slack-interactivity-action-notifications)
4. [Slack Slash Commands (Case Creation)](#slack-slash-commands-case-creation)
5. [Automatic Risk Channel Creation](#automatic-risk-channel-creation)
6. [Enterprise Grid (Org-Level App) Setup](#enterprise-grid-org-level-app-setup)
7. [Complete Setup Guide](#complete-setup-guide)
8. [Environment Variables Reference](#environment-variables-reference)

---

## Slack OAuth Authentication

Slack OAuth is used for user authentication via OpenID Connect (OIDC). The system can operate in two modes:

1. **Slack OAuth Mode**: Production authentication using Slack workspace
2. **No-Auth Mode**: Development mode that skips OAuth flow but still requires a valid Slack user ID

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

   - **Bot Token Scopes**:
     - `admin.conversations:write` (to connect channels across workspaces in Enterprise Grid; only needed for org-level apps)
     - `bookmarks:write` (to add bookmarks to case channels)
     - `channels:history` (to receive message events from public channels via Events API)
     - `channels:manage` (to create, rename, and invite users to public channels)
     - `channels:read` (to list and read channel information, and receive membership events)
     - `chat:write` (to post and update action notification messages in channels)
     - `commands` (to receive slash command invocations)
     - `files:read` (to access file metadata and download files via `url_private`)
     - `groups:read` (to read private channel information and receive membership events)
     - `groups:write` (to create private channels for private cases)
     - `team:read` (to list workspaces for org-level Slack app support)
     - `usergroups:read` (to list user groups and their members for auto-invite)
     - `users:read` (to fetch user profile information including avatar images)
     - `users:read.email` (to access user email addresses from profiles)

   **Important**: User Token Scopes and Bot Token Scopes serve different purposes:
   - User Token Scopes authenticate users via OpenID Connect
   - Bot Token Scopes allow the application to manage channels, receive events, and fetch user information using the Bot Token

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

| Event | Description | Required Bot Scope |
|-------|-------------|-------------------|
| `message.channels` | Messages posted to public channels the app is in | `channels:history` |
| `app_mention` | When someone mentions your app with @app_name | (no additional scope) |
| `member_joined_channel` | When a user joins a channel | `channels:read` |
| `member_left_channel` | When a user leaves a channel | `channels:read` |

The `member_joined_channel` and `member_left_channel` events are required for **Private Case** access control. When these events fire, the application automatically syncs the channel member list to the associated case, keeping access permissions up to date.

**Optional events** (if you need private channel or DM support):

| Event | Description | Required Bot Scope |
|-------|-------------|-------------------|
| `message.groups` | Messages posted to private channels the app is in | `groups:history` |
| `message.im` | Messages posted in direct message channels with the app | `im:history` |
| `message.mpim` | Messages posted in multi-person direct messages the app is in | `mpim:history` |

**Note**: Subscribing to optional events requires adding the corresponding scopes to **Bot Token Scopes** in OAuth & Permissions.

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

## Slack Interactivity (Action Notifications)

When an action is created in Hecatoncheires, a notification message is automatically posted to the associated case's Slack channel. This message includes interactive buttons that allow users to update the action status directly from Slack.

### How It Works

1. When an action is created, if the associated case has a Slack channel, a Block Kit message is posted
2. The message contains the action title, description, a link to the web UI, assignees, and interactive buttons
3. Users can click buttons to:
   - **Assign to me**: Add themselves as an assignee
   - **In Progress**: Change the action status to IN_PROGRESS
   - **Completed**: Change the action status to COMPLETED
4. After a button click, the Slack message is updated to reflect the new state

### Interactivity Setup

#### 1. Enable Interactivity

1. In your Slack app settings, go to **Interactivity & Shortcuts**
2. Toggle **Interactivity** to **On**

#### 2. Set Request URL

Enter your interactivity endpoint URL:

For local development with ngrok:
```
https://your-ngrok-id.ngrok.io/hooks/slack/interaction
```

For production:
```
https://your-domain.com/hooks/slack/interaction
```

**Note**:
- Slack requires HTTPS
- The interactivity endpoint uses the same Slack signing secret as the Events API webhook for request verification
- Your app must be running with the correct signing secret configured

#### 3. Add Required Bot Scopes

The bot token must have the `chat:write` scope to post and update messages:

1. In your app settings, go to **OAuth & Permissions**
2. Under **Bot Token Scopes**, add:
   - `chat:write` (required for posting and updating messages in channels)
3. Click **Save Changes**
4. **Reinstall the app** to apply the new scope

### Message Format

The notification message includes:

- **Header**: "Action: {emoji} {title}"
- **Description**: Action description (if provided)
- **Link**: Link to the action detail page
- **Context**: Assignees (as @mentions) and current status
- **Buttons**: "Assign to me", "In Progress", "Completed"

The message is automatically updated when the action is modified (title, assignees, status, etc.) from the web UI or Slack buttons.

### Requirements

- The case must have a Slack channel (`slackChannelID` must be set)
- The bot must be a member of the channel
- The bot token must have `chat:write` scope
- The signing secret must be configured for interaction verification
- Slack message posting is best-effort: if it fails, action creation still succeeds

---

## Slack Slash Commands (Case Creation)

Slack slash commands allow users to create cases directly from Slack without opening the web UI. When a slash command is invoked, a modal dialog appears where users can fill in the case title, description, and custom fields defined in the workspace configuration.

### How It Works

1. User types a slash command (e.g., `/create-case`) in Slack
2. Slack sends a request to Hecatoncheires
3. A Block Kit modal opens with the case creation form
4. User fills in the form and submits
5. A new case is created and a confirmation message is posted to the channel

#### Workspace Selection

The behavior depends on how many workspaces are configured and whether a workspace ID is specified in the URL:

- **Workspace ID in URL** (e.g., `/hooks/slack/command/risk`): Opens the case creation modal directly for that workspace
- **Single workspace configured**: Opens the case creation modal automatically
- **Multiple workspaces configured**: Shows a workspace selection modal first, then the case creation modal

### Slash Command Setup

#### 1. Create a Slash Command in Slack

1. In your Slack app settings, go to **Slash Commands**
2. Click **Create New Command**
3. Configure the command:
   - **Command**: Enter the command name (e.g., `/create-case`)
   - **Request URL**: Set to your slash command endpoint:
     ```
     https://your-domain.com/hooks/slack/command
     ```
     Or to target a specific workspace:
     ```
     https://your-domain.com/hooks/slack/command/{workspace_id}
     ```
   - **Short Description**: e.g., "Create a new case"
4. Click **Save**

You can create multiple slash commands targeting different workspaces. For example:
- `/create-risk` → `https://your-domain.com/hooks/slack/command/risk`
- `/create-incident` → `https://your-domain.com/hooks/slack/command/incident`

#### 2. Enable Interactivity (Required)

Slash commands use Block Kit modals, which require **Interactivity** to be enabled. If you have already configured [Slack Interactivity](#slack-interactivity-action-notifications), this is already done. Otherwise:

1. Go to **Interactivity & Shortcuts** in your Slack app settings
2. Toggle **Interactivity** to **On**
3. Set **Request URL** to:
   ```
   https://your-domain.com/hooks/slack/interaction
   ```
4. Click **Save Changes**

#### 3. Required Bot Scopes

The bot token must have the `chat:write` scope to post confirmation messages after case creation. Add this scope in **OAuth & Permissions** → **Bot Token Scopes** if not already present.

### Custom Fields in Modal

The case creation modal dynamically includes input fields based on the workspace's field configuration (defined in TOML config). Supported field types:

| Field Type | Modal Input |
|------------|------------|
| `text` | Plain text input |
| `number` | Number input |
| `select` | Single-select dropdown |
| `multi_select` | Multi-select dropdown |
| `user` | Slack user selector |
| `multi_user` | Multi-user selector |
| `date` | Date picker |
| `url` | URL text input |

### Requirements

- Slack signing secret must be configured (for request verification)
- Bot token must have `chat:write` scope (for posting confirmation messages)
- Interactivity must be enabled in the Slack app
- At least one workspace must be configured

---

## Automatic Risk Channel Creation

Hecatoncheires can automatically create dedicated Slack channels for each risk when it is registered. This feature helps teams collaborate and discuss specific risks in dedicated channels.

### How It Works

When a new case is created through the GraphQL API:

1. A Slack channel is automatically created with a standardized name
2. If the case is marked as **Private**, the channel is created as a **private channel**; otherwise, it is a public channel
3. The channel ID is stored with the case in the `slackChannelID` field
4. If channel creation fails, the case creation is rolled back (transactional)

When a case is updated and its name changes:

1. The associated Slack channel is automatically renamed to match the new case name
2. The channel ID remains the same

### Private Case Channel Behavior

For private cases, additional behavior applies:

- The Slack channel is created as a **private channel**, restricting visibility to invited members only
- Channel member IDs are synced to the case and used for **access control** — only channel members can view the case, its actions, knowledges, and assist logs via the API and UI
- Member sync happens automatically when `member_joined_channel` or `member_left_channel` events are received, or manually via the **Sync** button on the case detail page
- Bot users are automatically filtered out from the stored member list

### Auto-Invite

Hecatoncheires can automatically invite specified users and user group members to case channels upon creation. This is configured in the `[slack.invite]` section of the TOML configuration file.

```toml
[slack.invite]
users = ["U12345678", "U87654321"]
groups = ["S0614TZR7", "@security-response"]
```

- **`users`**: A list of Slack user IDs to invite directly
- **`groups`**: A list of Slack user group IDs or `@`-prefixed handle names. Group members are resolved and invited automatically

When a case is created:
1. The creator and assignees are invited first
2. Auto-invite users are added to the invite list
3. Group identifiers are resolved to member lists (`@`-prefixed handle names are looked up via `usergroups.list`)
4. All users are deduplicated before the invitation API call

Group resolution failures are logged but do not block case creation. The `usergroups:read` bot scope is required.

### Channel Naming Convention

Channels are named using the format: `{prefix}-{risk-id}-{normalized-risk-name}`

For example:
- Risk ID: `42`
- Risk Name: "SQL Injection in User Auth"
- Default Prefix: `risk`
- Result: `#risk-42-sql-injection-in-user-auth`

With a custom prefix:
- Prefix: `incident`
- Result: `#incident-42-sql-injection-in-user-auth`

Japanese characters are supported:
- Risk Name: "認証システムの脆弱性"
- Result: `#risk-42-認証システムの脆弱性`

### Channel Name Normalization

Risk names are automatically normalized to comply with Slack's channel naming rules:

- Uppercase letters → converted to lowercase
- Spaces → replaced with hyphens
- Special characters (slashes, periods, commas, etc.) → removed
- Japanese characters (hiragana, katakana, kanji) → preserved
- Maximum length: 80 characters (truncated if longer)

### Customizing the Channel Prefix

You can customize the channel name prefix using the `--slack-channel-prefix` flag or environment variable:

```bash
# Using CLI flag (default: "risk")
./hecatoncheires serve --slack-channel-prefix="incident"

# Using environment variable
export HECATONCHEIRES_SLACK_CHANNEL_PREFIX="security"
./hecatoncheires serve
```

This allows you to organize channels by different categories (e.g., `incident-*`, `security-*`, `vulnerability-*`).

### Required Bot Permissions

For automatic channel creation and full Slack integration, the bot token must have the following scopes:

- `admin.conversations:write` - Connect channels across workspaces in Enterprise Grid (only needed for org-level apps)
- `bookmarks:write` - Add bookmarks to case channels
- `channels:history` - Receive message events from public channels via Events API
- `channels:manage` - Create, rename, and invite users to public channels
- `channels:read` - List and read channel information, receive membership events
- `chat:write` - Post and update action notification messages in channels
- `commands` - Receive slash command invocations
- `files:read` - Access file metadata and download files attached to messages
- `groups:read` - Read private channel information, receive membership events for private channels
- `groups:write` - Create private channels for private cases
- `team:read` - List workspaces for org-level Slack app support
- `usergroups:read` - List user groups and their members (for auto-invite feature)
- `users:read` - Fetch user profile information (name, avatar)
- `users:read.email` - Access user email addresses from profiles

Add these scopes in **OAuth & Permissions** → **Bot Token Scopes** in your Slack app settings.

### Configuration

Enable automatic channel creation by providing a bot token:

```bash
export HECATONCHEIRES_SLACK_BOT_TOKEN="xoxb-your-bot-token"
export HECATONCHEIRES_SLACK_CHANNEL_PREFIX="risk"  # Optional, defaults to "risk"
```

Or using CLI flags:

```bash
./hecatoncheires serve \
  --slack-bot-token="xoxb-your-bot-token" \
  --slack-channel-prefix="incident"
```

If no bot token is provided, risks will be created without Slack channels (the `slackChannelID` field will be empty).

### Examples

**Example 1: Default configuration**
```bash
export HECATONCHEIRES_SLACK_BOT_TOKEN="xoxb-..."
./hecatoncheires serve
```

Creating a risk named "XSS Vulnerability in Dashboard" (ID: 5) will create channel: `#risk-5-xss-vulnerability-in-dashboard`

**Example 2: Custom prefix**
```bash
export HECATONCHEIRES_SLACK_BOT_TOKEN="xoxb-..."
export HECATONCHEIRES_SLACK_CHANNEL_PREFIX="sec"
./hecatoncheires serve
```

Creating a risk named "Data Leak in API" (ID: 12) will create channel: `#sec-12-data-leak-in-api`

**Example 3: Japanese risk names**
```bash
export HECATONCHEIRES_SLACK_BOT_TOKEN="xoxb-..."
./hecatoncheires serve
```

Creating a risk named "データベースのSQLインジェクション" (ID: 7) will create channel: `#risk-7-データベースのsqlインジェクション`

---

## Enterprise Grid (Org-Level App) Setup

Hecatoncheires supports Slack Enterprise Grid with org-level app installation. This allows a single app to operate across all workspaces in the organization.

### Overview

When using an org-level Slack app:

- **Auto-detection**: At startup, hecatoncheires calls `auth.test` and checks for `enterprise_id` to automatically detect whether the app is org-level or workspace-level
- **Channel creation**: Channels are created in the workspace specified by `slack.team_id` in the TOML config
- **Cross-workspace channel connect**: When a case is created from a slash command in a different workspace than the configured `slack.team_id`, the case channel is automatically connected to the source workspace via `admin.conversations.setTeams`, making the channel visible to users in both workspaces. This requires the `admin.conversations:write` bot scope.
- **User sync**: The background user refresh worker automatically discovers all workspaces via `auth.teams.list` and fetches users from each workspace
- **Backward compatible**: Existing workspace-level app configurations work without any changes

### Step 1: Create an Org-Level Slack App

1. Go to [https://api.slack.com/apps](https://api.slack.com/apps)
2. Create a new app or use an existing one
3. Under **Org Level Apps** (in the app settings sidebar), enable org-level distribution
4. Configure the same OAuth scopes as described in the [authentication setup](#2-configure-oauth--permissions), including `admin.conversations:write` for cross-workspace channel connectivity

### Step 2: Install the App to All Workspaces

The app must be installed to each workspace where you want to fetch users.

1. Go to the Enterprise Grid admin console: `https://app.slack.com/manage/{ENTERPRISE_ID}`
2. Click **Manage Organization** (top-right)
3. Navigate to **Integrations** in the left sidebar
4. Click **Manage installed apps**
5. Find your app and click on it
6. On the **Installations** tab, install the app to all relevant workspaces

> **Important**: The user refresh worker uses `auth.teams.list` to discover workspaces, which only returns workspaces where the app is installed. If the app is only installed in one workspace, only users from that workspace will be fetched.

### Step 3: Find Workspace Team IDs

Each workspace has a Team ID (starts with `T`). You need this for the `slack.team_id` setting in your TOML config.

**From the Enterprise admin console:**

1. Go to `https://app.slack.com/manage/{ENTERPRISE_ID}`
2. Click **Manage Organization**
3. Navigate to **Workspaces** in the left sidebar
4. Click on a workspace
5. The URL will contain the Team ID: `https://app.slack.com/manage/{ENTERPRISE_ID}/workspaces/{TEAM_ID}/settings`

**From the startup log:**

At startup, hecatoncheires logs the detected Team ID:
```
INFO Detected org-level Slack app enterprise_id=E02GQTQ0E48 team_id=T2G71SMA8
```

### Step 4: Configure Workspace TOML

Add `slack.team_id` to each workspace TOML configuration. This tells hecatoncheires which Slack workspace to create channels in for each hecatoncheires workspace.

```toml
[workspace]
id = "risk"
name = "Risk Management"

[slack]
channel_prefix = "risk"
team_id = "T2G71SMA8"  # Required for org-level apps

[slack.invite]
users = ["U12345678"]
```

### Validation Rules

At startup, hecatoncheires validates the `slack.team_id` configuration:

| App Type | `slack.team_id` set | `slack.team_id` empty |
|----------|--------------------|-----------------------|
| **Org-Level App** | OK | **Startup error** (required) |
| **WS-Level App** | Must match `auth.test` team_id, otherwise **startup error** | OK (default) |

- For org-level apps, every workspace config MUST have `slack.team_id`
- For workspace-level apps, `slack.team_id` is optional. If set, it must match the bot's actual workspace
- If no bot token is configured, validation is skipped entirely

### User Refresh Behavior

The background user refresh worker behaves differently based on the app type:

- **Org-Level App**: Calls `auth.teams.list` to discover all installed workspaces, then calls `users.list` with each Team ID. Users are deduplicated across workspaces
- **WS-Level App**: Calls `users.list` without a Team ID (single workspace, same as before)

### Troubleshooting

#### Only users from one workspace are fetched

Check the startup log for `fetched users from all workspaces`:
```
INFO fetched users from all workspaces team_count=11 unique_user_count=1500
```

If `team_count` is 1, the app is likely only installed in one workspace. Install it to all workspaces via the Enterprise admin console (see [Step 2](#step-2-install-the-app-to-all-workspaces)).

#### Startup error: "org-level Slack app requires slack.team_id"

All workspace TOML configs must have `slack.team_id` set when using an org-level app. See [Step 3](#step-3-find-workspace-team-ids) for how to find your Team IDs.

#### Startup error: "slack.team_id does not match the bot's workspace"

You have `slack.team_id` set in a workspace config, but the app is a workspace-level (not org-level) app. Either remove the `slack.team_id` setting or switch to an org-level app.

---

## Complete Setup Guide

### Step-by-Step Configuration

Follow these steps to set up both authentication and webhooks:

1. **Create Slack App** (see [Create a Slack App](#1-create-a-slack-app))

2. **Configure OAuth** (see [Configure OAuth & Permissions](#2-configure-oauth--permissions))
   - Set redirect URL: `${BASE_URL}/api/auth/callback`
   - Add user scopes: `openid`, `profile`, `email`
   - Add bot scopes: `admin.conversations:write` (org-level only), `bookmarks:write`, `channels:history`, `channels:manage`, `channels:read`, `chat:write`, `commands`, `files:read`, `groups:read`, `groups:write`, `team:read`, `usergroups:read`, `users:read`, `users:read.email`

3. **Configure Events API** (see [Events API Setup](#events-api-setup))
   - Enable Event Subscriptions
   - Set request URL: `${BASE_URL}/hooks/slack/event`
   - Subscribe to bot events: `message.channels`, `app_mention`, `member_joined_channel`, `member_left_channel`

4. **Configure Interactivity** (see [Interactivity Setup](#interactivity-setup))
   - Enable Interactivity in **Interactivity & Shortcuts**
   - Set request URL: `${BASE_URL}/hooks/slack/interaction`

5. **Configure Slash Commands** (optional, see [Slash Command Setup](#slash-command-setup))
   - Create slash command(s) in **Slash Commands**
   - Set request URL: `${BASE_URL}/hooks/slack/command` or `${BASE_URL}/hooks/slack/command/{workspace_id}`

6. **Get Credentials**:
   - Client ID (from Basic Information)
   - Client Secret (from Basic Information)
   - Signing Secret (from Basic Information)

7. **Install App to Workspace**:
   - Install the app
   - Copy Bot User OAuth Token (`xoxb-...`)

8. **Set Environment Variables** (see below)

9. **Start the Server**:
   ```bash
   ./hecatoncheires serve
   ```

10. **Verify Setup**:
   - Check logs for "Slack authentication enabled"
   - Check logs for "Slack webhook handler enabled"
   - Check logs for "Slack interaction handler enabled"
   - Check logs for "Slack slash command handler enabled" (if slash commands are configured)
   - Test authentication by accessing the web UI
   - Test webhook by posting a message in a channel (after inviting the bot)
   - Test interactivity by creating an action and clicking buttons in the Slack message
   - Test slash command by typing your command in Slack (e.g., `/create-case`)

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
- `member_joined_channel` - When a user joins a channel (triggers channel member sync for private cases)
- `member_left_channel` - When a user leaves a channel (triggers channel member sync for private cases)

Messages are stored with:
- Channel ID
- User ID and name
- Message text
- Timestamp
- Thread information (if it's a threaded message)
- File attachment metadata (if files are attached to the message)

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

## No-Auth Mode (Development)

For local development and testing, you can use the `--no-auth` flag to skip OAuth flow while still operating as a real Slack user:

```bash
# Requires bot token for user validation
export HECATONCHEIRES_SLACK_BOT_TOKEN="xoxb-your-bot-token"
export HECATONCHEIRES_NO_AUTH="U1234567890"  # Your Slack user ID

./hecatoncheires serve
```

**Requirements:**
- `--slack-bot-token` is required for user validation
- The specified user ID must exist in your Slack workspace
- `--no-auth` cannot be used with `--slack-client-id` or `--slack-client-secret`

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

#### Authentication not working
- Verify all required environment variables are set
- Check for typos in variable names
- Ensure values are not empty strings

#### No-auth mode fails to start
- Verify `HECATONCHEIRES_SLACK_BOT_TOKEN` is set
- Ensure the user ID exists in your Slack workspace
- Check that `--slack-client-id` and `--slack-client-secret` are not set (they are mutually exclusive with `--no-auth`)

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
| `/hooks/slack/interaction` | POST | Receives Slack interactive component payloads (button clicks, modal submissions) |
| `/hooks/slack/command` | POST | Receives Slack slash command invocations (opens case creation modal) |
| `/hooks/slack/command/{ws_id}` | POST | Receives Slack slash command invocations for a specific workspace |

All webhook endpoints require valid Slack signature verification.

---

## Environment Variables Reference

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `HECATONCHEIRES_BASE_URL` | Yes* | - | Base URL of the application (e.g., `https://your-domain.com`). No trailing slash. |
| `HECATONCHEIRES_SLACK_CLIENT_ID` | Yes* | - | Slack OAuth client ID |
| `HECATONCHEIRES_SLACK_CLIENT_SECRET` | Yes* | - | Slack OAuth client secret |
| `HECATONCHEIRES_SLACK_BOT_TOKEN` | No*** | - | Slack Bot User OAuth Token (starts with `xoxb-`) |
| `HECATONCHEIRES_SLACK_SIGNING_SECRET` | Yes** | - | Slack Events API signing secret |
| `HECATONCHEIRES_SLACK_CHANNEL_PREFIX` | No | `risk` | Prefix for auto-created Slack channel names for risks (e.g., `incident` creates `#incident-1-risk-name`) |
| `HECATONCHEIRES_NO_AUTH` | No | - | Slack user ID for no-auth mode (development only) |

\* Required for OAuth mode.

\** Required only if you want to enable Slack webhook integration. Without this, webhook endpoints will not be enabled.

\*** Required when using `HECATONCHEIRES_NO_AUTH`.

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
    - files: array (file attachment metadata, may be empty)
      - id: string (Slack file ID)
      - name: string (file name)
      - mimetype: string (MIME type)
      - filetype: string (Slack file type code)
      - size: int (file size in bytes)
      - url_private: string (Slack authenticated access URL)
      - permalink: string (Slack file permalink)
      - thumb_url: string (thumbnail URL, if available)
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

---

## Permissions Reference

### Bot Token Scopes

These scopes are required for the Bot User OAuth Token (`xoxb-...`):

| Scope | Slack API Method | Purpose | Code Location |
|-------|-----------------|---------|---------------|
| `bookmarks:write` | `bookmarks.add` | Add bookmarks to case channels | `pkg/service/slack/client.go` |
| `channels:history` | Events API | Receive `message.channels` events from public channels | Webhook handler |
| `admin.conversations:write` | `admin.conversations.setTeams` | Connect channels across workspaces in Enterprise Grid (org-level only) | `pkg/service/slack/client.go` |
| `channels:manage` | `conversations.create` | Create new public Slack channels for cases | `pkg/service/slack/client.go` |
| `channels:manage` | `conversations.rename` | Rename Slack channels when case name changes | `pkg/service/slack/client.go` |
| `channels:manage` | `conversations.invite` | Invite users to case channels | `pkg/service/slack/client.go` |
| `channels:read` | `conversations.list` | List public channels the bot has joined | `pkg/service/slack/client.go` |
| `channels:read` | `conversations.info` | Get channel name and info (with caching) | `pkg/service/slack/client.go` |
| `channels:read` | Events API | Receive `member_joined_channel` / `member_left_channel` events | `pkg/usecase/slack.go` |
| `chat:write` | `chat.postMessage` | Post action notification messages to channels | `pkg/service/slack/client.go` |
| `chat:write` | `chat.update` | Update action notification messages after button clicks | `pkg/service/slack/client.go` |
| `commands` | Slash commands | Receive slash command invocations (e.g., case creation) | `pkg/controller/http/slack_command.go` |
| `files:read` | Events API | Access file metadata attached to messages via `url_private` | Webhook handler |
| `groups:read` | `conversations.info` | Read private channel info and receive membership events | `pkg/service/slack/client.go` |
| `groups:write` | `conversations.create` | Create private Slack channels for private cases | `pkg/service/slack/client.go` |
| `team:read` | `auth.teams.list` | List workspaces for org-level Slack app support | `pkg/service/slack/client.go` |
| `usergroups:read` | `usergroups.list` | List user groups for handle name resolution (auto-invite) | `pkg/service/slack/client.go` |
| `usergroups:read` | `usergroups.users.list` | Get user group members (auto-invite) | `pkg/service/slack/client.go` |
| `users:read` | `users.info` | Fetch user profile (name, avatar) | `pkg/service/slack/client.go` |
| `users:read` | `users.list` | List all non-deleted, non-bot users in workspace | `pkg/service/slack/client.go` |
| `users:read.email` | `users.info`, `users.list` | Access user email addresses from profiles | `pkg/service/slack/client.go` |

### User Token Scopes

These scopes are required for user authentication via OpenID Connect:

| Scope | Purpose | Code Location |
|-------|---------|---------------|
| `openid` | OpenID Connect authentication flow | `pkg/usecase/auth.go` |
| `profile` | Access user's name | `pkg/usecase/auth.go` |
| `email` | Access user's email address | `pkg/usecase/auth.go` |

### Event Subscriptions

These bot events must be subscribed to in the Slack app settings:

| Event | Required Bot Scope | Currently Handled |
|-------|-------------------|-------------------|
| `message.channels` | `channels:history` | Yes |
| `app_mention` | (none) | Yes |
| `member_joined_channel` | `channels:read` (or `groups:read` for private channels) | Yes |
| `member_left_channel` | `channels:read` (or `groups:read` for private channels) | Yes |
| `message.groups` | `groups:history` | Optional |
| `message.im` | `im:history` | Optional |
| `message.mpim` | `mpim:history` | Optional |

### Other Requirements

| Requirement | Purpose |
|------------|---------|
| Signing Secret | HMAC-SHA256 verification of webhook requests |
| OAuth Client ID | OpenID Connect authentication |
| OAuth Client Secret | OpenID Connect token exchange |
| Redirect URL | OAuth callback (`${BASE_URL}/api/auth/callback`) |
| Request URL (Events) | Events API webhook endpoint (`${BASE_URL}/hooks/slack/event`) |
| Request URL (Interactivity) | Interactivity endpoint (`${BASE_URL}/hooks/slack/interaction`) |

---

## See Also

- [Configuration Guide](./config.md) - TOML config file, CLI flags, and field definitions
- [Authentication Guide](./auth.md) - Slack OAuth and no-auth mode setup
