## Why

Today, getting an already-written markdown file into Hugo requires three manual steps in the admin UI: create a new (empty) post, paste the body, then separately trigger "generate frontmatter" and "generate cover". Authors who draft elsewhere (other editors, exports, AI drafts) want to drop a `.md` in once and let the existing AI services fill in the frontmatter (description / tags / categories) and a cover image. The building blocks already exist (`frontmatter_gen_service`, `image_gen_service`, cover/frontmatter endpoints) but there is no upload path that wires them together.

## What Changes

- New backend endpoint `POST /api/article/import` (multipart upload) accepting a `.md` / `.markdown` file plus optional `title` and flags `generate_frontmatter` (default `true`) and `generate_cover` (default `true`).
- New import orchestrator service that: parses the uploaded markdown, preserves/merges any frontmatter already present, derives a title (existing frontmatter → first `#` heading → filename), creates the article under `content/post/<date>-<slug>/index.md` as a draft, generates AI frontmatter by reusing `services/frontmatter_gen_service.generate_frontmatter`, and optionally generates + saves a cover image by reusing `services/image_gen_service`.
- Graceful degradation: if AI is unconfigured or a generation step fails, the article is still imported with minimal frontmatter and the failure is reported as a partial result — import never silently loses the content.
- Frontend: an "Upload Markdown" action on the Posts page that opens a file picker, shows progress through the frontmatter/cover steps, and opens the Editor for the newly created post on success.

## Capabilities

### New Capabilities
- `markdown-import`: Import an external Markdown file into Hugo content, automatically enriching it with AI-generated frontmatter and an AI-generated cover image, reusing the existing AI services.

### Modified Capabilities
<!-- None. This change only adds a new import path; it reuses (does not alter the spec-level behavior of) the existing AI services and post management. -->

## Impact

- **Backend (new code)**: one route added to `routes/file_routes.py` (which already hosts `/api/post/create`) and a new `services/article_import_service.py` orchestrator. No changes to existing service signatures.
- **Backend (reused)**: `services/frontmatter_gen_service.generate_frontmatter`, `services/image_gen_service.generate_cover_image` / `save_generated_image`, and `services/post_service` path/slug/save helpers.
- **Config**: depends on existing `AI_API_KEY` / `AI_BASE_URL` / `AI_MODEL` and `OPENROUTER_API_KEY` / `IMAGE_GEN_MODEL`. No new required configuration.
- **Frontend**: new upload entry + small upload component in `frontend/src/pages/Posts.tsx`, plus an API helper in `frontend/src/utils`.
- **Tests**: new `pytest` cases for the orchestrator and route (mocking the AI generation functions), following the repo's async/mock conventions.
