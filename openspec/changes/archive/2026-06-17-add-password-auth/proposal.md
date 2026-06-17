## Why

Every `hugo-admin` route — editing posts, publishing via git, managing the Hugo server, the AI chat — is currently open: anyone who can reach the port can act as the admin. The app runs on `0.0.0.0` for LAN access, so on any shared network the blog is fully writable by anyone. We need a password-based login so only the configured admin can use the tool. Scope is deliberately "支持登录即可": a single admin account, login/logout, and gate the existing APIs and SPA — no registration, roles, or reset flows.

## What Changes

- **Credential store (new).** A small `AuthService` backed by a JSON file at `WEB_ADMIN_DIR/data/auth.json` (repo-install-local, independent of `HUGO_ROOT`, so credentials survive a blog-repo switch). Passwords are hashed with `werkzeug.security` (ships with Flask — no new dependency). On first run, if the store is empty, bootstrap a default `admin` account from the `ADMIN_USERNAME`/`ADMIN_PASSWORD` env vars (default `admin`/`admin`) and log a prominent warning to change it.
- **Auth API (new).** A new `register_auth_routes(registry)` blueprint exposing `POST /api/auth/login`, `POST /api/auth/logout`, `GET /api/auth/me`, and `POST /api/auth/password` (change the logged-in user's password). Login sets a Flask signed-cookie `session` (the `SECRET_KEY` already exists in `config.py`); bad credentials return 401.
- **Auth guard on the API (new).** A global `before_request` hook that rejects every `/api/*` request with 401 JSON unless a valid session exists, with a small public allowlist (`/api/auth/login`, `/api/auth/me`, `/api/version`). All existing post/file/image/publish/git/AI/settings endpoints become login-gated with no per-route changes.
- **SocketIO gate (new).** Reject unauthenticated `connect` events in `register_socketio_handlers` so the realtime channel (AI chat, publish, Hugo server) can't bypass the API guard.
- **Frontend login (new).** A `frontend/src/pages/Login.tsx` page (outside the app `Layout`), a `RequireAuth` wrapper that calls `/api/auth/me` and redirects to `/login` on 401, a 401 interceptor in `utils/api.ts` that bounces stale sessions to `/login`, and a "退出登录" item in the Sidebar. `App.tsx` gains a `/login` route and wraps the protected routes in `RequireAuth`.
- **Config (additive).** Document `ADMIN_USERNAME`/`ADMIN_PASSWORD` in `.env.example`; add `PERMANENT_SESSION_LIFETIME` to `config.py` for a sane session TTL.

## Capabilities

### New Capabilities
- `user-auth`: Password-based login for the single admin account — credential storage, the login/logout/me/change-password API, session enforcement across the HTTP API and SocketIO, and the frontend login flow that gates the SPA.

### Modified Capabilities
<!-- None. Existing capabilities (post editing, git/publish, settings, AI chat) keep their requirements; this change only adds an auth precondition in front of their HTTP/SocketIO entry points, which is a cross-cutting guard, not a change to their spec-level behavior. -->

## Impact

- **Backend (new code)**: `services/auth_service.py` (`AuthService` — JSON load/save, hash/verify, bootstrap, change-password), `routes/auth_routes.py` (`register_auth_routes(registry)` + `install_auth_guard(app)` `before_request`), SocketIO `connect` gate inside `routes/socketio_routes.py`, an `auth_service` property on `ServiceRegistry`, and wiring in `app.py`.
- **Backend (reused)**: Flask `session` + existing `SECRET_KEY`; `werkzeug.security.generate_password_hash`/`check_password_hash` (no new dependency); the existing `ServiceRegistry`/blueprint registration pattern.
- **Config**: `config.py` gains `PERMANENT_SESSION_LIFETIME`; `.env.example` gains `ADMIN_USERNAME`/`ADMIN_PASSWORD`. `data/auth.json` is runtime state, added to `.gitignore`/`.dockerignore`.
- **Frontend**: new `Login.tsx`, new `RequireAuth` component, `App.tsx` route changes, a 401 branch in `utils/api.ts` + `login`/`logout`/`getMe`/`changePassword` helpers, a logout item in `Sidebar.tsx`. No frontend test runner exists today.
- **Tests**: `pytest` cases for `AuthService` (hash/verify, bootstrap default, change-password), the auth routes (login success/failure, `/me`, logout, change-password), and the guard (protected `/api/*` returns 401 when logged out, allowlist is public, SocketIO rejects unauthed connect) following the repo's Flask test-client conventions.
- **Operational**: First launch with no env vars boots a default `admin`/`admin` and prints a warning; existing users must set `ADMIN_USERNAME`/`ADMIN_PASSWORD` (or change the password in-app) to secure the instance.
