# Phase 1: Data Model

**Feature**: Article Publishing
**Date**: 2025-11-14
**Purpose**: Define data entities, validation rules, and state transitions for article publishing

## Data Entities

### Article

**Description**: Represents a blog post with frontmatter metadata and content

**Fields**:
- `file_path` (string, required): Absolute path to the markdown file
- `filename` (string, required): Just the filename without path
- `frontmatter` (dict, required): YAML metadata at top of file
  - `title` (string, optional): Article title
  - `draft` (boolean, required): Whether article is draft (true) or published (false)
  - `date` (datetime, optional): Publication date
  - `categories` (list of strings, optional): Article categories
  - `tags` (list of strings, optional): Article tags
  - `author` (string, optional): Article author
- `content` (string, required): Markdown content body
- `last_modified` (datetime, required): File modification timestamp

**Validation Rules**:
- `file_path` must exist and be readable/writable
- `file_path` must be within allowed content directory
- `frontmatter.draft` must be boolean if present
- `frontmatter` must be valid YAML format
- `content` can be empty but must be present

**State Transitions**:
```
Draft (draft: true) → Published (draft: false)
     ↑                        ↓
   ← Unpublish Operation      ← (No direct return to draft)
```

### PublishOperation

**Description**: Represents a publish action and its result

**Fields**:
- `operation_id` (string, required): Unique identifier for the operation
- `article_path` (string, required): Path to article being published
- `operation_type` (enum, required): "publish" | "bulk_publish"
- `status` (enum, required): "pending" | "success" | "failed"
- `started_at` (datetime, required): Operation start timestamp
- `completed_at` (datetime, optional): Operation completion timestamp
- `error_message` (string, optional): Error details if operation failed
- `changed_draft_status` (boolean, required): Whether draft status was actually changed

**Validation Rules**:
- `operation_id` must be unique (UUID4 recommended)
- `article_path` must exist and be accessible
- `operation_type` must be valid enum value
- `completed_at` is required when status is "success" or "failed"
- `error_message` is required when status is "failed"

### PublishStatus

**Description**: Current publishing status of an article for UI display

**Fields**:
- `article_path` (string, required): Path to article
- `is_draft` (boolean, required): Current draft status
- `is_publishable` (boolean, required): Whether article can be published
- `last_published` (datetime, optional): Last time article was published
- `publish_errors` (list of strings, optional): Any issues preventing publication

**Validation Rules**:
- `is_publishable` is false when `is_draft` is true
- `last_published` is null if article has never been published
- `publish_errors` is empty when `is_publishable` is true

## File Structure Mapping

### Markdown File Structure
```markdown
---
title: "Article Title"
draft: true
date: 2025-11-14
categories: ["tech"]
tags: ["hugo", "blog"]
author: "Author Name"
---

# Article Content

This is the markdown content of the article...
```

### Parsed Data Model
```json
{
  "file_path": "/path/to/content/post/article.md",
  "filename": "article.md",
  "frontmatter": {
    "title": "Article Title",
    "draft": true,
    "date": "2025-11-14",
    "categories": ["tech"],
    "tags": ["hugo", "blog"],
    "author": "Author Name"
  },
  "content": "# Article Content\n\nThis is the markdown content...",
  "last_modified": "2025-11-14T10:30:00Z"
}
```

## State Machine Diagram

```
    [File Load]
         ↓
    [Parse Frontmatter]
         ↓
    [Validate Draft Status]
         ↓
    [Is Draft?] ──No──→ [Return "Already Published"]
         ↓Yes
    [Attempt File Lock]
         ↓
    [Lock Acquired?] ──No──→ [Return "Operation in Progress"]
         ↓Yes
    [Update Draft: false]
         ↓
    [Save File]
         ↓
    [Release Lock]
         ↓
    [Return Success]
```

## Error States

### File System Errors
- **FileNotFound**: Article file doesn't exist
- **PermissionDenied**: Cannot read/write file
- **InvalidYAML**: Frontmatter cannot be parsed
- **ConcurrentAccess**: File locked by another operation

### Validation Errors
- **InvalidPath**: File path outside allowed directory
- **MissingFrontmatter**: No YAML frontmatter found
- **InvalidDraftValue**: Draft field is not boolean

### Operation Errors
- **AlreadyPublished**: Article is already published
- **LockTimeout**: Cannot acquire file lock within timeout
- **SaveFailed**: File cannot be written after modification

## Cache Keys

For integration with existing cache_service.py:

```python
# Article publish status
CACHE_KEY_ARTICLE_STATUS = f"article:publish_status:{file_path}"

# Article metadata
CACHE_KEY_ARTICLE_META = f"article:metadata:{file_path}"

# Publish operation status
CACHE_KEY_OPERATION = f"operation:publish:{operation_id}"
```

## Performance Considerations

### File Operations
- Use file locking to prevent concurrent modifications
- Batch operations for bulk publishing
- Cache status lookups to avoid repeated file reads

### Memory Usage
- Load only frontmatter for status checks, not full content
- Stream large files when possible
- Clear operation data after completion

### Caching Strategy
- Cache article status for 5 minutes
- Invalidate cache on successful publish operations
- Use cache for UI status indicators

## Integration Points

### PostService Extensions
```python
class PostService:
    def publish_article(self, file_path: str) -> Tuple[bool, str]
    def bulk_publish_articles(self, file_paths: List[str]) -> Dict
    def get_publish_status(self, file_path: str) -> PublishStatus
    def is_article_publishable(self, file_path: str) -> Tuple[bool, List[str]]
```

### API Response Formats
```json
// Single publish result
{
  "success": true,
  "message": "Article published successfully",
  "article_path": "/path/to/article.md",
  "operation_id": "uuid4-string"
}

// Bulk publish result
{
  "success": true,
  "published_count": 3,
  "failed_count": 1,
  "results": [
    {"path": "article1.md", "success": true},
    {"path": "article2.md", "success": false, "error": "Permission denied"}
  ]
}
```
