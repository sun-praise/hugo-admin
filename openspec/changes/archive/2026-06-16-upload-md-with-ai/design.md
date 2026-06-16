## Context

hugo-admin is a Flask + flask-socketio admin UI for a Hugo blog. Articles are stored as
`content/post/<YYYY-MM-DD>-<slug>/index.md` with YAML frontmatter (`title`, `date`, `draft`,
`categories`, `tags`, `description`, `cover`). Two AI services already exist and are wired to
manual per-article buttons:

- `services/frontmatter_gen_service.generate_frontmatter(content, api_key, base_url, model)` →
  text model, ~seconds, returns `{description, tags, categories}`, has an in-memory cache, and
  is exposed at `POST /api/frontmatter/generate`.
- `services/image_gen_service.generate_cover_image(...)` + `save_generated_image(...)` →
  OpenRouter image model, can take up to **180s**, saves into the article's `pics/` directory and
  returns a relative URL; exposed at `POST /api/image/generate-cover`.

`services/post_service` already owns path/slug creation (`create_post`), safe-path checks
(`_is_safe_path`, `_validate_file_path`), frontmatter stripping/merging (`_strip_leading_frontmatter`,
`save_file`), and atomic image writes (`save_image`). The app uses
`flask-socketio` with `SOCKETIO_ASYNC_MODE = "threading"` and a `ServiceRegistry`.

There is currently **no** upload entry point for `.md` files — only image upload exists. Authors
must create an empty post, paste content, then manually trigger frontmatter + cover.

## Goals / Non-Goals

**Goals:**
- One-step import: upload a `.md` file → a draft post is created with AI-generated frontmatter
  and an AI-generated cover image.
- Reuse the existing AI services and `post_service` helpers verbatim — no new AI provider, prompt,
  or config is introduced.
- Graceful degradation: an uploaded file is never lost because AI is unconfigured or a generation
  step fails.
- Give the user honest progress feedback, given cover generation is slow.

**Non-Goals:**
- Bulk/multi-file import — one file per request in this change.
- Transforming the body text (only frontmatter is enriched and a cover attached; the body is
  preserved verbatim). Body polishing already exists via the inline-AI-edit feature.
- Importing into the `content/page/` section — only `content/post/` in this change.
- AI-suggested title — title is derived deterministically and remains editable in the Editor.

## Decisions

### 1. New orchestrator service, route stays thin
Create `services/article_import_service.py` exposing
`import_markdown(filename, raw_bytes, *, title, generate_frontmatter, generate_cover, post_service, ai_cfg, image_cfg)`
returning `{path, title, warnings}`. The pipeline (parse → derive title/slug → create dir →
generate frontmatter → generate/attach cover → write merged file) is cohesive and unit-testable,
keeping the route handler small. **Alternative considered:** add the method to `post_service` —
rejected to avoid bloating the already-large `PostService` and to keep import logic in one place.

### 2. Frontmatter synchronous; cover runs in a background thread with Socket.IO progress
The HTTP request returns as soon as the article is created and frontmatter is written (~seconds).
Cover generation (≤180s) is launched via `socketio.start_background_task` and reports progress
through the existing socket connection (`article_import.progress`,
`article_import.cover_done`/`cover_failed`). The new post's `path` is returned immediately so the
user can start editing while the cover renders. **Why not fully synchronous:** a ≤180s request is
fragile under browser/proxy timeouts. **Why not a task queue (Celery/RQ):** no such dependency
exists today and socketio background tasks already cover the need. **Fallback:** if Socket.IO is
unavailable on the server, generate the cover synchronously within the request so the feature
still works end-to-end in minimal setups.

### 3. Title derivation is deterministic
Title = existing frontmatter `title` → first `# ` H1 in the body → sanitized filename stem. No
extra LLM call. Slug/directory reuse `create_post`-style naming (`<YYYY-MM-DD>-<slug>`). The Editor
remains the place to refine it.

### 4. Preserve and merge existing frontmatter
If the uploaded `.md` already has frontmatter, parse it and only fill `description`/`tags`/
`categories` when absent; keep any existing `title`/`date`/`cover`. Reuse
`post_service._strip_leading_frontmatter` + `yaml` for splitting. **Alternative considered:**
always overwrite — rejected as lossy for exported posts that already carry metadata.

### 5. Call the existing AI services directly
Invoke `frontmatter_gen_service.generate_frontmatter` and
`image_gen_service.generate_cover_image` / `save_generated_image` unchanged. They already handle
prompting, JSON sanitization, frontmatter caching, and image extraction/saving. **Alternative
considered:** duplicating prompts in the orchestrator — rejected (prompt drift).

### 6. Route placement: `routes/file_routes.py`
Add `POST /api/article/import` (multipart) inside the existing `register_file_routes(registry)`,
next to `/api/post/create`. It reads AI keys from `current_app.config` and passes them into the
orchestrator, mirroring how `routes/image_routes.py` reads `OPENROUTER_API_KEY`. **Alternative
considered:** a new `import_routes.py` blueprint — fine, but one endpoint does not yet justify a
new registration line; revisit if more import endpoints appear.

### 7. Validation & safety
Enforce `.md`/`.markdown` extension; honor `MAX_CONTENT_LENGTH` (16MB); resolve the created path
strictly under `content/post` via the existing `_is_safe_path`/`_validate_file_path` checks;
reject path traversal in any client-supplied `title`. Write the file atomically (temp + rename)
like `save_image`. Decode bytes as UTF-8 with `errors="replace"` and warn — never crash.

### 8. Draft by default
Imported posts are `draft: true` with a generated CST (UTC+8) `date`, matching `create_post`, so
nothing is accidentally published.

## Risks / Trade-offs

- **Cover latency / API cost** → cover is opt-out per request (`generate_cover=false`), runs in the
  background, and failures degrade gracefully (post saved without cover; the existing
  `/api/image/generate-cover` button can retry).
- **AI key unconfigured** → orchestrator detects the missing key, skips that enrichment, and adds a
  human-readable entry to `warnings[]`; import still succeeds with minimal frontmatter.
- **Socket.IO not connected on the client** → server falls back to synchronous cover generation
  within the request; the UI also re-fetches the post list so the cover appears once written.
- **Title/slug collisions** → reuse date+slug naming; if the directory exists, append a short uuid
  suffix (consistent with `save_image` collision handling).
- **Large or malformed markdown** → only a 2000-char snippet is sent to the frontmatter prompt
  (already enforced by `frontmatter_gen_service`); the full body is still saved.
- **No migration needed** — the change is purely additive and touches no database or schema. Rollback
  = revert the changeset; existing posts are untouched.
