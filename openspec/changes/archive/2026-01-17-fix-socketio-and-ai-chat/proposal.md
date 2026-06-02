# Change: Fix Socket.IO Port Configuration and AI Chat Response

## Why
Testing revealed two critical issues: (1) Socket.IO connection fails because the frontend hardcodes port 5050 while the server may run on different ports (e.g., 5051); (2) AI chat assistant shows no response despite the backend returning 200 OK, because streaming SSE is not being rendered correctly.

## What Changes
- **BREAKING**: Remove hardcoded Socket.IO connection URL, use dynamic port detection
- Fix AI chat SSE streaming to correctly handle and display response chunks
- Ensure Socket.IO connects to the same port as the Flask server

## Impact
- Affected specs: realtime-websocket, ai-ui-chatbox
- Affected code:
  - `templates/base.html`: Socket.IO connection URL (line 132), AI chat SSE handling
