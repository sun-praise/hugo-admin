# Change: Fix AI Tool Call Results Not Streaming

## Why
When users request actions that require tool calls (e.g., "search articles", "check git status"), the AI assistant only streams the initial text response ("I'll help you search...") but fails to stream the tool execution results. The stream terminates after the AI decides to call a tool, leaving users without the actual results they requested.

## What Changes
- Modify AI chat endpoint to properly handle tool call results in streaming responses
- After streaming initial text, if there are tool calls, wait for results and stream them back
- Ensure the `[DONE]` marker is only sent after all content (including tool results) is streamed

## Impact
- Affected specs: `ai-agent-core`
- Affected code: `routes/ai_routes.py`, `services/ai_service.py`
