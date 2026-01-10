# Tasks: Add AI Chat Assistant

## Phase 1: Infrastructure
- [x] Install dependencies: `pydantic-ai`, `openai`, `python-dotenv`
- [x] Add AI configuration to `config.py` (API Key, Base URL, Model)
- [x] Verify environment variables loading in `app.py`

## Phase 2: AI Service Development
- [x] Create `services/ai_service.py`
- [x] Implement `Deps` class for dependency injection
- [x] Define `Agent` with system prompt
- [x] Implement core tools:
    - [x] `search_posts_tool`
    - [x] `read_post_tool`
    - [x] `write_post_tool`
    - [x] `git_status_tool`
    - [x] `deploy_tool`
- [x] Add unit tests for `AIService` tool calls

## Phase 3: API Integration
- [x] Implement `/api/ai/chat` in `app.py`
- [x] Support streaming response from PydanticAI to Flask
- [x] Test API endpoint with mock requests

## Phase 4: Frontend Implementation
- [x] Add chat box HTML/Tailwind structure to `templates/base.html`
- [x] Implement Alpine.js logic for chat interaction
- [x] Add `marked.js` for Markdown rendering
- [x] Implement "copy code" or "apply change" UI helpers if applicable

## Phase 5: Verification
- [x] End-to-end test: "Search for posts about Hugo"
- [x] End-to-end test: "Create a new post titled AI Test"
- [x] End-to-end test: "Check git status"
