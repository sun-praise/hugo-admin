## 1. Credential store (`AuthService`)

- [x] 1.1 Create `services/auth_service.py` with an `AuthService` class: constructor takes a JSON `store_path` (default `Path(__file__).parent.parent / "data" / "auth.json"`), ensures the parent dir exists, and loads the account dict (shape `{"username", "password_hash", "updated_at"}`) on init; missing/empty file → no account.
- [x] 1.2 Implement password hashing with `werkzeug.security.generate_password_hash` / `check_password_hash` (no new dependency). Add `verify(username, password) -> bool`, `set_password(username, new_password)` (re-hash + persist), and `get_user() -> dict | None`.
- [x] 1.3 Add `bootstrap_default()` that, when no account exists, creates one from `os.environ["ADMIN_USERNAME"]` (default `"admin"`) and `os.environ["ADMIN_PASSWORD"]` (default `"admin"`), persists it, and `print()`s a loud `⚠` warning listing the username and urging a password change. Call it from the constructor so it runs once on startup; existing accounts are never overwritten.
- [x] 1.4 Add atomic save (`json.dump` to a temp file in the same dir then `os.replace`) so a crash mid-write cannot corrupt `data/auth.json`. Keep all reads/writes UTF-8 with `ensure_ascii=False`.

## 2. Config & env

- [x] 2.1 In `config.py` `Config`, add `PERMANENT_SESSION_LIFETIME = timedelta(days=30)` (import `timedelta`), `SESSION_COOKIE_SAMESITE = "Lax"`, and leave `SESSION_COOKIE_HTTPONLY` at its default (True). Do not set `SESSION_COOKIE_SECURE` (so HTTP-on-LAN still works).
- [x] 2.2 Append `ADMIN_USERNAME` and `ADMIN_PASSWORD` documentation to `.env.example` (after the existing `SECRET_KEY` block), noting the first-run default `admin`/`admin` and that the password can be rotated in-app.

## 3. Auth API routes + guard

- [x] 3.1 Create `routes/auth_routes.py` exposing `register_auth_routes(registry)` returning a `Blueprint("auth", __name__)` with: `POST /api/auth/login` (read `username`/`password` from JSON; 400 on missing fields; `registry.auth_service.verify(...)` → on success set `session["username"]=username`, `session.permanent=True`, return `200 {success:true, user:{username}}`; on failure `401 {success:false, message:"用户名或密码错误"}`), `GET /api/auth/me` (return `200 {success:true,user:{username}}` if `session` has a user else `401 {success:false,message:"未登录"}`), `POST /api/auth/logout` (`session.clear()` → `200 {success:true}`), and `POST /api/auth/password` (require session; read `current_password`/`new_password`; 400 if missing; `verify` current else `401`; then `set_password`, return `200 {success:true}`).
- [x] 3.2 In the same module add `install_auth_guard(app)`: registers an `app.before_request` that, for any `request.path` starting with `/api/`, returns `401 {success:false, message:"未登录或会话已过期"}` when `"username" not in session`, **unless** the path is in the public allowlist (`/api/auth/login`, `/api/auth/me`, `/api/version`). Non-`/api/` paths (SPA + static) pass through untouched.
- [x] 3.3 Add a module-level `login_required` decorator (checks `session` for `username`, else returns the same 401 JSON) for any future per-route need; not required on existing routes because the global guard covers them.
- [x] 3.4 Export `register_auth_routes` (and `install_auth_guard`) from `routes/__init__.py` and add them to `__all__`.

## 4. Wiring (`ServiceRegistry` + `app.py`)

- [x] 4.1 Add an `auth_service` property (getter+setter) to `services/registry.py` mirroring the existing `git_service`/`settings_service` pattern.
- [x] 4.2 In `app.py`, after the config/`SECRET_KEY` load and before registering blueprints, construct `auth_service = AuthService(app.config["HUGO_ROOT"].parent / "data" / "auth.json")` — verify the path resolves to the repo-install `data/` dir (i.e. `WEB_ADMIN_DIR / "data" / "auth.json"`); use `Path(__file__).parent / "data" / "auth.json"` for clarity — then `registry.auth_service = auth_service` (set it on the existing `registry` construction or immediately after).
- [x] 4.3 In `app.py`, call `app.register_blueprint(register_auth_routes(registry))` alongside the other blueprints, and call `install_auth_guard(app)` **after** all blueprints are registered (so the guard sees every route) but the order relative to `before_request` registration only requires the function to be registered before the first request.

## 5. SocketIO connect gate

- [x] 5.1 In `routes/socketio_routes.py` (`register_socketio_handlers(registry)`), add a `@socketio.on("connect")` handler that returns `False` (reject) when `"username" not in session` (import `session` from `flask`); optionally emit an `auth_error` event before rejecting. Authenticated connects pass through unchanged.
- [x] 5.2 Verify the frontend only establishes the socket after login (the socket-init code lives in the protected `Layout`, which mounts only for authed routes); if any code connects globally, move/lazy-init it so the `/login` page does not connect.

## 6. Frontend

- [x] 6.1 In `frontend/src/utils/api.ts`, add a 401 interceptor in `request()`: when `response.status === 401` (and the URL is not `/api/auth/login` itself), clear any local auth state and `window.location.href = "/login"` before throwing. Add typed helpers `login(username, password)`, `logout()`, `getMe()` → `{success, user?:{username}}`, and `changePassword(currentPassword, newPassword)`.
- [x] 6.2 Create `frontend/src/pages/Login.tsx`: a centered card (Tailwind, matching existing pages) with username + password fields, error message area, and submit that calls `login(...)`; on success navigate to the `from` location or `/`. Render it **outside** the app `Layout`.
- [x] 6.3 Create `frontend/src/components/RequireAuth.tsx`: on mount calls `getMe()`; while loading show a minimal spinner; on 401/failure render `<Navigate to="/login" state={{from}} replace />`; on success render `<Outlet />` (or `children`). Accept the intended `from` from `useLocation`.
- [x] 6.4 In `frontend/src/App.tsx`, add `<Route path="/login" element={<Login />} />` outside the `Layout`, and wrap the existing `<Route path="/" element={<Layout />}>` branch so protected routes render through `RequireAuth` (e.g. `<Route element={<RequireAuth />}><Route path="/" element={<Layout />}>…</Route></Route>`).
- [x] 6.5 In `frontend/src/components/Sidebar.tsx`, add a "退出登录" item (footer area) that calls `logout()` then navigates to `/login`; conditionally render nothing auth-related on the login page (login renders outside Layout so this is naturally satisfied).
- [x] 6.6 Build the frontend (`npm run build` in `frontend/`) so `static/dist/` includes the login flow; verify `static/dist/index.html` is updated. (Frontend has no test runner — manual smoke test only.)

## 7. Update existing backend tests for the guard

- [x] 7.1 Add a `logged_in_client` fixture (alongside the existing Flask test-client fixtures) that constructs the app with a known `ADMIN_USERNAME`/`ADMIN_PASSWORD`, boots it so `AuthService` bootstraps, and `client.post("/api/auth/login", json={...})` before returning the client. (Implemented as shared `auth_store` + `login` fixtures in `tests/conftest.py`.)
- [x] 7.2 Sweep existing `tests/test_*_api.py` suites that hit protected `/api/*` endpoints and switch them to the `logged_in_client` fixture so they no longer receive `401` from the new guard (focus on suites actually run in CI; do not touch unrelated failures). (Updated `client` fixtures in `test_git_history_api`, `test_publish_api`, `test_publish_integration`, `test_settings_api`, `test_spa_routes`; other suites use standalone apps or test services directly and are unaffected.)

## 8. New auth tests

- [x] 8.1 Create `tests/test_auth_service.py`: bootstrap default from env (hash present, not plaintext); `verify` true/false; `set_password` rotates the hash and old password fails; existing store is not overwritten on re-init.
- [x] 8.2 Create `tests/test_auth_api.py` (Flask test client): `POST /api/auth/login` success → 200 + `/api/auth/me` returns user; wrong password → 401; unknown user → 401; missing fields → 400; `POST /api/auth/logout` then protected GET → 401; `POST /api/auth/password` success rotates, wrong current rejected, requires session.
- [x] 8.3 Add guard tests (extend `tests/test_auth_api.py` or a new `tests/test_auth_guard.py`): logged-out `GET /api/posts` (or another protected endpoint) → 401 JSON; allowlist endpoints (`/api/version`, `/api/auth/me`) respond without being blocked by the guard; logged-in client gets the normal payload. (Covered in `tests/test_auth_api.py`.)
- [x] 8.4 Add a SocketIO connect test: an unauthenticated `socketio.test_client(app)` fails to connect (or receives no events), while a logged-in one connects — following the repo's socketio test conventions if present; if socketio tests are impractical in CI, cover the connect-handler logic with a focused unit test instead and note it. (`tests/test_socketio_auth.py`: anonymous connect rejected, session-carrying connect accepted via `flask_test_client=`.)

## 9. Lint & verification

- [x] 9.1 Run `pytest` from the repo root and ensure the full suite (existing + new) passes.
- [x] 9.2 Run `ruff check .` and ensure new files (`services/auth_service.py`, `routes/auth_routes.py`, `tests/test_auth_*.py`) are clean; do not "fix" pre-existing ruff errors in untouched files.
- [x] 9.3 Smoke-test manually: boot the app, confirm the startup warning, confirm `/login` loads and a correct password enters the app while a wrong one stays; confirm a logged-out `curl /api/posts` returns 401 and `/api/version` returns 200.
