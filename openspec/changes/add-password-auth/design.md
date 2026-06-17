## Context

`hugo-admin` is a Flask + SocketIO app (`app.py`) that manages a Hugo blog. It registers ~12 blueprints via `register_*_routes(registry)` (post/file/image/publish/git/AI/settings/plugin/email/server/page), exposes everything under `/api/*`, serves a React SPA from `static/dist/index.html` via `routes/page_routes.py`, and runs on `0.0.0.0:5050` for LAN access. There is **no authentication anywhere** today: any client that can reach the port can read/edit posts, publish via git, and drive the AI. `socketio = SocketIO(app, cors_allowed_origins="*")` carries the realtime channel (AI chat, Hugo server, publish progress) with no connect gate.

State today lives in two places: a per-repo SQLite `Database` at `CONTENT_DIR/.admin/cache.db` (`models/database.py` — posts cache, chat, push history), and a JSON settings file via `SettingsService` at `HUGO_ROOT/.admin/settings.json`. `ServiceRegistry` (`services/registry.py`) holds the singletons; `app.py` constructs them and `settings_routes.py` rebuilds `git_service`/`post_service`/`hugo_manager` when `HUGO_ROOT` changes.

`config.py` already defines `SECRET_KEY` (so Flask signed-cookie sessions work out of the box) and `WEB_ADMIN_DIR = Path(__file__).parent` (the repo/install root). The repo's `.gitignore` already ignores the whole `data/` directory. Frontend is React + react-router + Tailwind (`frontend/src`): pages in `src/pages/`, nav in `src/components/Sidebar.tsx`, routes in `src/App.tsx`, all HTTP through `src/utils/api.ts` (`request/get/post/put/del` with centralized error handling). There is no frontend test runner; backend tests use Flask's test client + pytest.

## Goals / Non-Goals

**Goals:**
- A single admin account, password-protected, that gates the entire tool — every `/api/*` endpoint and the SocketIO realtime channel.
- Login/logout via the SPA, with a clearly-stated "you are logged out" UX and automatic bounce-back to `/login` on a stale/missing session.
- Zero new runtime dependencies: reuse Flask `session`, `werkzeug.security`, the existing `SECRET_KEY`, and the `ServiceRegistry`/blueprint conventions.
- Credentials stored independently of which Hugo blog is being managed (survive a `HUGO_ROOT` switch), never in plaintext.
- Trivial first-run setup (env vars or in-app change-password) and a safe default that loudly warns when unset.

**Non-Goals:**
- Multi-user accounts, roles/permissions, or registration — this is a single-admin tool.
- Password reset via email, 2FA, or "remember me" tokens beyond Flask's session lifetime.
- Rate limiting / brute-force protection on login (single-admin LAN tool; noted as a future hardening).
- CSRF tokens beyond browser SameSite defaults (single-origin SPA; see Risks).
- An `AUTH_DISABLED` escape hatch for trusted/reverse-proxied deployments (deferred; can be added later without rework if needed).
- Frontend automated tests (no runner exists today).

## Decisions

### 1. Store credentials in a JSON file at `WEB_ADMIN_DIR/data/auth.json`, not in the per-repo SQLite DB
Auth describes *who may use this admin tool*, which is orthogonal to which Hugo blog is configured. The existing `Database` lives at `CONTENT_DIR/.admin/cache.db` (per-repo); putting users there would couple login to whichever repo is active and would drop/reset credentials on a `HUGO_ROOT` switch (the same per-repo scope the `git-history` change deliberately accepted for push history — wrong for auth). A small JSON file at `data/auth.json` (repo/install-local, already gitignored) is the natural home and mirrors the existing `SettingsService` JSON pattern. **Alternative considered:** a `users` table in `cache.db` — rejected for the per-repo coupling above and because that DB is optional/not-present in some configs; **env-only credentials** — rejected because they preclude changing the password from the UI, which "用户系统" implies and which is needed to escape the default password safely.

### 2. Hash passwords with `werkzeug.security`, not bcrypt/argon2
`generate_password_hash`/`check_password_hash` ship with Flask (Werkzeug) and use a modern salted scrypt/pbkdf2 scheme with a versioned, self-describing hash string. This adds **zero dependencies** — the explicit non-goal from the `git-history` change ("No new dependency") applies equally here. **Alternative considered:** `bcrypt`/`argon2-cffi` — rejected as a new dep for a single-admin tool with low brute-force exposure; the hash string remains self-describing, so a future swap is a drop-in.

### 3. Authenticate the session with Flask's signed cookie `session`, not JWT or Flask-Login
`SECRET_KEY` is already configured, so `session[...]` gives a tamper-proof, server-side-meaningful identity with no extra library. Logout is trivial (`session.clear()`); a stateless JWT would require a revocation list to invalidate on logout. **Alternative considered:** `Flask-Login` (`@login_required`, `current_user`) — convenient but a new dependency for a one-account system; a hand-rolled `login_required` + `session` check is a few lines. **Cookie hardening:** set `SESSION_COOKIE_SAMESITE="Lax"` and rely on the default `SESSION_COOKIE_HTTPONLY=True` to mitigate CSRF/XSS; `SESSION_COOKIE_SECURE` is left unset so login still works over plain HTTP on a LAN (it can be flipped on behind HTTPS).

### 4. Enforce auth with one global `before_request` guard on `/api/*`, not per-route decorators
A single `install_auth_guard(app)` hook returns `401 {"success": false, "message": "未登录或会话已过期"}` for any `/api/*` path lacking a valid session, with a small public allowlist: `/api/auth/login`, `/api/auth/me`, `/api/version`. This protects all ~12 blueprints at once with **no per-route edits**, so it cannot be accidentally omitted from a new route. **Alternative considered:** a `@login_required` decorator on every endpoint — rejected as tedious and fragile (easy to forget on the next new route). **Allowlist rationale:** `/api/auth/me` must be reachable while logged out so the SPA can *detect* logged-out state (it returns 401 with a JSON body); `/api/version` stays public as a harmless health/version probe.

### 5. SocketIO `connect` is gated by the same session
Because the SPA and SocketIO are same-origin, the Flask session cookie is sent on the `socketio` handshake. Add a `connect` handler in `register_socketio_handlers` that rejects the connection (callback `False` / emits an `auth_error` and returns) when `session` has no user — otherwise the realtime channel (AI chat, publish, Hugo server) would silently bypass the API guard. **Non-goal:** per-event re-validation; the `connect` gate plus the same-origin cookie is sufficient for this scope.

### 6. SPA gating is client-side UX; page routes stay public and serve the bundle
The SPA is a single bundle whose `/login` route is part of it, so `static/dist/*` assets and the `page_routes` SPA fallback must remain reachable unauthenticated (otherwise the login page cannot load). The real security boundary is the API guard (Decision 4); the frontend layering is purely UX: a `RequireAuth` wrapper calls `GET /api/auth/me` on mount and `<Navigate to="/login" />` on 401, and `utils/api.ts` intercepts any 401 response to bounce a stale session to `/login`. The `/login` route renders outside the `Layout` (no sidebar); all current routes render inside `RequireAuth`.

### 7. Bootstrap a default admin from env on first run; allow in-app password change
On `AuthService` init, if `data/auth.json` is absent/empty, create a single account: username `ADMIN_USERNAME` or `"admin"`, password `ADMIN_PASSWORD` or `"admin"` (hashed), and print a loud `⚠` warning listing the credentials and urging a change. `POST /api/auth/password` (`current_password` + `new_password`) lets the admin rotate it; it verifies the current password first and re-hashes the new one. **Alternative considered:** refusing to start without env vars — rejected (poor local DX, breaks `pytest`); interactive prompt — rejected (not viable under Docker/systemd). The default is a deliberate, noisy trade-off so the tool is usable out of the box.

### 8. Session lifetime and config
Add `PERMANENT_SESSION_LIFETIME` (e.g. 30 days) to `config.py` and set `session.permanent = True` on login so a browser restart keeps the admin logged in for a reasonable window rather than expiring when the tab closes. `ADMIN_USERNAME`/`ADMIN_PASSWORD` are documented in `.env.example`. `data/auth.json` is already covered by the existing `data/` gitignore rule.

## Risks / Trade-offs

- **Default `admin`/`admin` when env unset** → loud startup warning + the `/api/auth/password` endpoint + README/.env.example note. This is the accepted cost of out-of-the-box usability.
- **`SECRET_KEY` left as the dev default in production** → session cookies could be forged. Mitigation: `.env.example` already flags this; the design reuses the existing key rather than introducing one. Document that a strong `SECRET_KEY` is required in prod alongside the new admin password.
- **CSRF on cookie-authenticated state-changing POSTs** → mitigated by `SameSite=Lax` (the SPA is same-origin and sends `Content-Type: application/json`, which is not a "simple" CSRF-able request). Full CSRF tokens are a deliberate non-goal for this single-origin tool.
- **Logout does not invalidate a stolen/old cookie server-side** → inherent to stateless signed-cookie sessions; acceptable at this scope (rotating `SECRET_KEY` invalidates all sessions if ever needed).
- **No login rate limiting** → brute-force is theoretically possible. Accepted for a single-admin LAN tool; a future `flask-limiter` integration is noted but out of scope.
- **Same-origin assumption** → if the SPA is ever served from a different origin than the API, the session cookie and SocketIO cookie won't flow and `cors_allowed_origins="*"` won't carry credentials. This change assumes the existing same-origin deployment (SPA served by Flask); cross-origin is explicitly out of scope.
- **`data/auth.json` is per-install** → moving the install or switching machines requires re-setting the password (or copying the file). Acceptable; it is the same class of runtime state as `data/cache.db`.

## Migration Plan

- **Deploy:** set `ADMIN_USERNAME`/`ADMIN_PASSWORD` (and a strong `SECRET_KEY`) in `.env`/container env before restart. On first boot, `data/auth.json` is created with the (hashed) credentials; a warning is printed. There are no pre-existing users or sessions to migrate.
- **Rollback:** remove the `register_auth_routes` + `install_auth_guard` calls from `app.py`, the SocketIO `connect` gate, and the frontend `RequireAuth` wrapper. All `/api/*` endpoints revert to open with no data migration; `data/auth.json` can be deleted. (Existing tests will need the new auth fixtures removed.)
- **Tests:** the guard means existing Flask test-client tests for protected endpoints will start returning 401. The task plan adds a small `logged_in_client` fixture (POSTs to `/api/auth/login`) and updates affected suites to use it, so the change is adoption-safe.

## Open Questions

- Should there be an `AUTH_DISABLED=1` escape hatch for deployments already behind auth/a reverse proxy? Recommended: defer (keep scope to "登录即可"); the guard is easy to short-circuit later via an env check in `install_auth_guard`.
