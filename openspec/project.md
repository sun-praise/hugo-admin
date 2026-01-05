# Project Context

## Purpose
Hugo Admin is a lightweight web-based admin interface for managing a Hugo static site. It provides a GUI to browse/search posts, edit Markdown (including frontmatter), manage per-article images, control the Hugo development server, and optionally run a Git-based “publish” flow (add/commit/push).

## Tech Stack
- Backend: Python 3.9+, Flask, Flask-SocketIO (Socket.IO)
- Frontend: Jinja2 templates + Tailwind CSS + Alpine.js
- Real-time: WebSocket via Socket.IO (used for log streaming)
- Caching/Data: SQLite-based cache (post index/stats)
- Parsing: `python-frontmatter`, `PyYAML`
- Process/System: `hugo` CLI, `git` CLI, `psutil`
- Tooling: `pytest`, `pytest-cov`, `black`, `flake8`

## Project Conventions

### Code Style
- Format Python with Black; lint with flake8.
- Prefer `pathlib.Path` over string paths.
- Keep the route/controller layer thin (`app.py`) and put business logic in `services/`.
- Use explicit, descriptive names; avoid clever abstractions.

### Architecture Patterns
- Entry point / routing:
  - `app.py`: Flask app + page routes + JSON APIs + Socket.IO wiring.
- Services:
  - `services/hugo_service.py`: starts/stops `hugo server` via `subprocess`, monitors output in a background thread, emits logs over Socket.IO.
  - `services/post_service.py`: content read/save/search, publish (toggle `draft`), image upload/list, cache invalidation, safety checks.
  - `services/git_service.py`: wraps `git status/add/commit/push` and provides a “system publish” API.
- Models:
  - `models/database.py`: SQLite helpers used by caching.
- UI:
  - `templates/*.html`: Jinja2 pages.
  - `static/`: CSS/JS assets.

### Testing Strategy
- Use `pytest` (see `pytest.ini` for discovery and markers).
- Tests live in `tests/` and cover cache, API, path handling, publish flows, and git service behavior.
- Prefer isolated tests using temporary dirs / temporary sqlite DBs; avoid depending on a real Hugo site state.

### Git Workflow
- Primary branches observed: `main` and `dev`.
- Commit messages are generally short; many use conventional prefixes like `feat:` / `fix:`.
- Typical workflow: feature branch → PR → GitHub Actions “Tests” workflow.

## Domain Context
- Hugo content lives under `content/`.
- Posts are commonly organized as page bundles, e.g. `content/post/<slug>/index.md`, with images stored alongside the post.
- Markdown files contain frontmatter; this project commonly reads/writes fields like:
  - `title`, `date` (RFC3339 with timezone, commonly `+08:00`), `draft`, `categories`, `tags`, optional `publishDate`.
- “Publishing” in this project means updating frontmatter (primarily `draft: true` → `false`) and optionally adding `publishDate`.

## Important Constraints
- Safety: file operations are restricted to allowed content subdirectories (see `config.py` / `config_local.py`), including path traversal protection.
- Concurrency: some write operations use file locking (`fcntl`) to prevent concurrent corruption.
- Runtime prerequisites: `hugo` must be installed and available in `PATH`.
- Git operations require a valid git repository in the configured Hugo root.
- Security: intended for local/dev usage; do not expose publicly without authentication and network hardening. (Defaults may bind to `0.0.0.0` depending on config.)
- Upload limits: max 16MB; allowed extensions are constrained by config.

## External Dependencies
- Hugo CLI (`hugo server` and site tooling)
- Git CLI (status/add/commit/push)
- GitHub Actions for CI
