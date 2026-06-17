## ADDED Requirements

### Requirement: Admin account is bootstrapped with hashed credentials
The system SHALL persist the admin account in a JSON store at `WEB_ADMIN_DIR/data/auth.json` with the password stored only as a self-describing salted hash (never plaintext). On startup, if the store has no account, the system SHALL create one using `ADMIN_USERNAME` (default `admin`) and `ADMIN_PASSWORD` (default `admin`) from the environment, and SHALL print a warning that the credentials should be changed.

#### Scenario: First run bootstraps a default admin from environment
- **WHEN** the app starts and `data/auth.json` does not exist or contains no account, with `ADMIN_USERNAME=admin` and `ADMIN_PASSWORD=s3cret` in the environment
- **THEN** the system creates an account `admin` whose stored record contains only a password hash (never the plaintext `s3cret`) and prints a startup warning listing the username and urging a password change.

#### Scenario: Defaults apply when environment is unset
- **WHEN** the app starts with an empty store and neither `ADMIN_USERNAME` nor `ADMIN_PASSWORD` is set
- **THEN** the system bootstraps the account `admin` with password `admin` (hashed) and prints the warning.

#### Scenario: Existing store is not overwritten on startup
- **WHEN** the app starts and `data/auth.json` already contains an account
- **THEN** the system does not recreate or reset the account, and any previously changed password is preserved.

#### Scenario: Corrupt credential file fails closed
- **WHEN** `data/auth.json` exists but is unreadable, contains invalid JSON, or is missing the `username`/`password_hash` fields
- **THEN** the system raises an error at startup rather than bootstrapping a new default admin, so a disk/permission/edit error can never silently reset or weaken credentials.

### Requirement: Login establishes an authenticated session
The system SHALL expose `POST /api/auth/login` accepting `username` and `password`. On a correct username/password pair it SHALL store the authenticated user identity in the Flask session and return `{success: true, user: {username}}`; on any mismatch it SHALL return `401` with `{success: false, message}` and SHALL NOT modify the session.

#### Scenario: Valid credentials log in
- **WHEN** a client sends `POST /api/auth/login` with the correct username and password
- **THEN** the response is `200` with `{success: true, user: {username}}` and a subsequent `GET /api/auth/me` (same session) returns the logged-in user.

#### Scenario: Wrong password is rejected
- **WHEN** a client sends `POST /api/auth/login` with a valid username but the wrong password
- **THEN** the response is `401` with `{success: false, message}`, and the session does not contain an authenticated user.

#### Scenario: Unknown user is rejected
- **WHEN** a client sends `POST /api/auth/login` with a username that does not exist
- **THEN** the response is `401` with `{success: false, message}`, and the session is not modified.

#### Scenario: Missing fields are rejected
- **WHEN** a client sends `POST /api/auth/login` without a `username` or `password`
- **THEN** the response is `400` with `{success: false, message}`.

### Requirement: Authenticated identity is queryable
The system SHALL expose `GET /api/auth/me` returning the currently logged-in user, or `401` with `{success: false, message}` when no session exists. This endpoint SHALL remain reachable while logged out (so the SPA can detect logged-out state) and SHALL NOT be subject to the API auth guard.

#### Scenario: Logged-in identity returned
- **WHEN** a client with a valid session sends `GET /api/auth/me`
- **THEN** the response is `200` with `{success: true, user: {username}}`.

#### Scenario: Logged-out identity returns 401
- **WHEN** a client with no session sends `GET /api/auth/me`
- **THEN** the response is `401` with `{success: false, message}`.

### Requirement: Logout terminates the session
The system SHALL expose `POST /api/auth/logout` that clears the authenticated session. After logout, previously protected endpoints SHALL return `401` for that client.

#### Scenario: Logout clears the session
- **WHEN** a logged-in client sends `POST /api/auth/logout`
- **THEN** the response is `200` with `{success: true}`, the session no longer contains an authenticated user, and a subsequent protected `GET` returns `401`.

### Requirement: Password can be changed
The system SHALL expose `POST /api/auth/password` (session-protected) accepting `current_password` and `new_password`. It SHALL verify `current_password` against the stored hash before accepting the change, then store a fresh hash of `new_password`. A wrong current password or missing fields SHALL be rejected without changing the stored hash.

#### Scenario: Correct current password changes it
- **WHEN** a logged-in client sends `POST /api/auth/password` with the correct `current_password` and a new `new_password`
- **THEN** the response is `200` with `{success: true}`, the stored record holds a hash of `new_password`, and the old password no longer authenticates via `POST /api/auth/login`.

#### Scenario: Wrong current password rejected
- **WHEN** a logged-in client sends `POST /api/auth/password` with an incorrect `current_password`
- **THEN** the response is `400` with `{success: false, message}` and the stored password hash is unchanged (401 is reserved for "not logged in", so the frontend 401→login redirect is not triggered).

#### Scenario: Requires authentication
- **WHEN** a client with no session sends `POST /api/auth/password`
- **THEN** the response is `401`.

### Requirement: Protected API endpoints require a session
The system SHALL reject every `GET`/`POST`/`PUT`/`DELETE` request to `/api/*` with `401 {success: false, message}` when no authenticated session is present, except for an explicit public allowlist: `POST /api/auth/login`, `GET /api/auth/me`, and `GET /api/version`. When authenticated, these endpoints SHALL behave exactly as before.

#### Scenario: Protected endpoint rejects logged-out client
- **WHEN** a client with no session requests any non-allowlisted endpoint such as `GET /api/posts` or `GET /api/git/commits`
- **THEN** the response is `401` with `{success: false, message}`.

#### Scenario: Allowlisted endpoints remain public
- **WHEN** a client with no session requests `GET /api/version` or `GET /api/auth/me`
- **THEN** the endpoint responds normally (not `401` due to the guard) — `/api/version` returns its payload and `/api/auth/me` returns its own `401` body.

#### Scenario: Authenticated client is unaffected
- **WHEN** a logged-in client requests a previously-protected endpoint such as `GET /api/posts`
- **THEN** the response is the endpoint's normal pre-auth payload, identical in shape and status to before this change.

### Requirement: SocketIO connections require a session
The system SHALL reject SocketIO `connect` attempts that have no authenticated session, so the realtime channel cannot bypass the API auth guard.

#### Scenario: Unauthenticated connect is rejected
- **WHEN** a SocketIO client without a session attempts to `connect`
- **THEN** the connection is rejected and no realtime events (AI chat, publish progress, Hugo server) are accepted from that client.

#### Scenario: Authenticated connect succeeds
- **WHEN** a SocketIO client with a valid session attempts to `connect`
- **THEN** the connection is accepted and events flow as before this change.

### Requirement: The SPA redirects unauthenticated users to login
The admin UI SHALL require authentication to view any app page: when no valid session exists it SHALL redirect to a `/login` page, and on any API `401` it SHALL return the user to `/login`. A successful login SHALL navigate back into the app. The `/login` page and the static bundle assets SHALL remain loadable while logged out.

#### Scenario: Visit app while logged out
- **WHEN** an unauthenticated user navigates to any app route (e.g. `/`)
- **THEN** the UI redirects to `/login`.

#### Scenario: Stale session bounces to login
- **WHEN** the session expires or is cleared while the user is in the app and an API call returns `401`
- **THEN** the UI redirects to `/login`.

#### Scenario: Login returns to the app
- **WHEN** the user submits valid credentials on `/login`
- **THEN** the UI navigates away from `/login` into the previously-requested app page (or the dashboard).

#### Scenario: Logout control is available
- **WHEN** the sidebar is rendered for a logged-in user
- **THEN** it contains a logout control that, when activated, logs the user out and returns them to `/login`.
