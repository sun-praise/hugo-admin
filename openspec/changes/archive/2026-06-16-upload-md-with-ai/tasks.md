## 1. Orchestrator service (backend core)

- [x] 1.1 Create `services/article_import_service.py` with the `import_markdown(filename, raw_bytes, *, title, generate_frontmatter=True, generate_cover=True, post_service, ai_cfg, image_cfg, socketio=None, event_scope=None)` entrypoint returning `{"path", "title", "warnings"}`.
- [x] 1.2 Implement title derivation (existing frontmatter `title` → first `# ` H1 → sanitized filename stem) and `<date>-<slug>` directory creation under `content/post`, appending a short uuid suffix on collision; reject path traversal in any client-supplied title.
- [x] 1.3 Implement frontmatter enrichment: split existing frontmatter with `python-frontmatter` (`frontmatter.loads`) + `yaml`, call `frontmatter_gen_service.generate_frontmatter` with `ai_cfg` (api_key/base_url/model), and merge `description`/`tags`/`categories` only for absent fields; on missing key or failure, append a warning and continue.
- [x] 1.4 Implement cover attachment: when enabled and `image_cfg.api_key` is present, call `image_gen_service.generate_cover_image` + `save_generated_image`, then set the `cover` frontmatter field; on failure append a warning and continue without a cover.
- [x] 1.5 Write the final `index.md` via `post_service.save_file` (atomic write handled there) with merged frontmatter (`draft: true`, generated CST `date`) and the preserved body; invalidate the post cache via `post_service`.

## 2. Background vs synchronous cover wiring

- [x] 2.1 Add `generate_and_attach_cover` + `_run_cover_with_emits` so cover generation runs via `socketio.start_background_task` emitting `article_import.progress` / `article_import.cover_done` / `article_import.cover_failed` (keyed by `event_scope`) when a `socketio` instance is supplied, and falls back to synchronous in-request generation otherwise.

## 3. Route

- [x] 3.1 Add `POST /api/article/import` (multipart) inside `register_file_routes` in `routes/file_routes.py`: read `file`, optional `title`, and flags from `request.form`; read `AI_API_KEY`/`AI_BASE_URL`/`AI_MODEL`/`OPENROUTER_API_KEY`/`IMAGE_GEN_MODEL` from `current_app.config`.
- [x] 3.2 Validate a file part is present, extension is `.md`/`.markdown` (reject → 400), and rely on `MAX_CONTENT_LENGTH` for size; pass `socketio` from the registry into the orchestrator and return `{success, path, title, warnings, cover_pending, event_scope}`.

## 4. App wiring

- [x] 4.1 Socket.IO is already exposed via `registry.socketio`; the import route reads it with `getattr(registry, "socketio", None)` (no new wiring needed in `app.py`). `register_file_routes` now accepts an optional injected `blueprint` for test isolation.

## 5. Frontend

- [x] 5.1 Add `uploadMarkdown(file, opts)` + `ImportResult` type in `frontend/src/utils/api.ts` that POSTs multipart/form-data to `/api/article/import` via the existing `request` helper.
- [x] 5.2 Add an "上传 Markdown" button + hidden `<input type=file accept=.md,.markdown>` to `frontend/src/pages/Posts.tsx`.
- [x] 5.3 Show an importing state, surface any `warnings` on partial success, subscribe to `article_import.cover_done`/`cover_failed` (scoped by `event_scope`) to report cover progress, and on success refresh the post list and open the Editor for the new `path`.

## 6. Tests & lint

- [x] 6.1 Add unit tests in `tests/test_article_import_service.py` for: title derivation (frontmatter/H1/filename), slug + collision suffix, frontmatter merge & preservation, graceful degradation when AI key missing or generation fails (mocked), and atomic file write / cover attachment.
- [x] 6.2 Add route tests in `tests/test_article_import_api.py` for happy path, unsupported extension, missing file, and partial-success `warnings`.
- [x] 6.3 Add a test for the background-vs-sync cover path (`FakeSocketIO` asserts the task is scheduled + emits `cover_done` when run, vs the sync fallback when `socketio=None`).
- [x] 6.4 Run `pytest` and `ruff check .` from the project root: 268 tests pass; the changed files are ruff-clean (2 pre-existing ruff errors remain in untouched files `proto/plugin_pb2.py` and `tests/test_ai_service.py`).
