# Change: Add AI Chat History Persistence

## Why
Currently AI chat history is only stored in browser memory and lost on page refresh. Users need to persist and retrieve their AI conversation history for continuity and reference.

## What Changes
- Add SQLite tables for chat sessions and messages
- Create ChatHistoryService for CRUD operations
- Add REST API endpoints for session management
- Update frontend to persist and load sessions

## Impact
- Affected specs: New `ai-chat-history` capability
- Affected code: models/database.py, services/, routes/ai_routes.py, templates/base.html
