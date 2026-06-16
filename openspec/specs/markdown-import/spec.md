# markdown-import Specification

## Purpose
TBD - created by archiving change upload-md-with-ai. Update Purpose after archive.
## Requirements
### Requirement: Upload a Markdown file to create a draft post
The system SHALL accept a Markdown file upload via `POST /api/article/import` and create a new draft article under `content/post/`, preserving the uploaded body content verbatim and writing valid Hugo frontmatter.

#### Scenario: Successful import of a plain markdown file
- **WHEN** a client uploads a `.md` file with body content and no flags
- **THEN** the system creates `<content>/post/<date>-<slug>/index.md` with `draft: true`, a generated `date`, and the uploaded body preserved verbatim, and returns the new article's relative `path`.

#### Scenario: Unsupported file type rejected
- **WHEN** a client uploads a file whose extension is not `.md` or `.markdown`
- **THEN** the system rejects the request with HTTP 400 and does not write any file.

#### Scenario: Missing file rejected
- **WHEN** a client posts the request without a file part
- **THEN** the system rejects the request with HTTP 400.

#### Scenario: Oversize file rejected
- **WHEN** the uploaded file exceeds `MAX_CONTENT_LENGTH`
- **THEN** the system rejects the request without writing a file.

### Requirement: Title derivation
The system SHALL derive the article title from existing frontmatter, else the first level-1 heading (`# `) in the body, else the sanitized filename stem, and SHALL derive the slug and directory name from that title.

#### Scenario: Title taken from existing frontmatter
- **WHEN** the uploaded markdown begins with frontmatter containing a `title`
- **THEN** that title is used for the article and the slug is derived from it.

#### Scenario: Title taken from first H1
- **WHEN** the markdown has no title frontmatter but the body's first heading is a level-1 heading
- **THEN** that heading text becomes the title and the slug is derived from it.

#### Scenario: Title taken from filename
- **WHEN** the markdown has no title frontmatter and no level-1 heading
- **THEN** the filename stem (without extension) becomes the title and the slug is derived from it.

#### Scenario: Directory name collision is handled
- **WHEN** the derived `<date>-<slug>` directory already exists
- **THEN** the system appends a short unique suffix to avoid overwriting an existing article.

### Requirement: AI-generated frontmatter enrichment
The system SHALL enrich the imported article with AI-generated `description`, `tags`, and `categories` by reusing `frontmatter_gen_service`, merging generated values into any frontmatter already present without overwriting existing fields.

#### Scenario: Frontmatter generated when AI is configured
- **WHEN** `AI_API_KEY` is configured and `generate_frontmatter` is enabled (default)
- **THEN** the written article's frontmatter includes AI-generated `description`, `tags`, and `categories`.

#### Scenario: Existing frontmatter fields are preserved
- **WHEN** the uploaded markdown already provides a metadata field that the AI would generate
- **THEN** the existing value is kept and the AI value is not written for that field.

#### Scenario: Graceful degradation when AI key is missing
- **WHEN** `AI_API_KEY` is not configured
- **THEN** the article is still created with minimal frontmatter, and a warning noting frontmatter generation was skipped is returned.

#### Scenario: Graceful degradation when AI returns an error
- **WHEN** the frontmatter generation service fails or times out
- **THEN** the article is still created without AI frontmatter and a warning is returned.

### Requirement: AI-generated cover image
The system SHALL optionally generate and attach a cover image by reusing `image_gen_service`, generating the image in the background and reporting progress over Socket.IO, and SHALL fall back to synchronous generation when Socket.IO is unavailable.

#### Scenario: Cover generated and attached
- **WHEN** `OPENROUTER_API_KEY` is configured and `generate_cover` is enabled (default)
- **THEN** a cover image is generated, saved into the article's `pics/` directory, and referenced via the `cover` frontmatter field.

#### Scenario: Cover disabled by flag is skipped
- **WHEN** the client sends `generate_cover=false`
- **THEN** no cover is generated and no `cover` field is written.

#### Scenario: Cover generation failure does not block import
- **WHEN** cover generation fails or times out
- **THEN** the article is still created (without a cover) and a warning is returned.

#### Scenario: Progress reported via Socket.IO
- **WHEN** cover generation runs in the background
- **THEN** the system emits Socket.IO progress/completion events keyed to the import so the client can show status.

### Requirement: Import result reports partial success
The system SHALL return the created article path together with a `warnings` list describing any enrichment steps that were skipped or failed, so that partial success is never silent.

#### Scenario: Fully successful import
- **WHEN** all requested enrichment steps succeed
- **THEN** the response contains the `path`, `title`, and an empty `warnings` list.

#### Scenario: Partial success reports warnings
- **WHEN** one enrichment step fails but the article is created
- **THEN** the response contains the `path`, `title`, and a non-empty `warnings` list describing what was skipped or failed.

### Requirement: Frontend upload action
The Posts page SHALL provide an "Upload Markdown" action that uploads a chosen `.md` file, surfaces progress for the frontmatter/cover steps, and on success opens the created article in the editor.

#### Scenario: User uploads and the editor opens
- **WHEN** the user picks a `.md` file and confirms
- **THEN** the file is uploaded, progress is shown, and on success the editor opens for the newly created article.

#### Scenario: Partial failures surfaced to the user
- **WHEN** the import succeeds with warnings (e.g., cover failed)
- **THEN** the UI surfaces the warnings while still opening the editor for the created article.
