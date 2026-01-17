## ADDED Requirements

### Requirement: Socket.IO Connection
The system SHALL establish Socket.IO connection using the same host and port as the current page URL, eliminating hardcoded port values.

#### Scenario: Dynamic port connection
- **WHEN** the page loads on any port (e.g., 5050, 5051, 5001)
- **THEN** Socket.IO connects to that same port automatically

#### Scenario: Console confirmation
- **WHEN** Socket.IO connects successfully
- **THEN** "Connected to server" appears in browser console

### Requirement: AI Chat Streaming Response
The system SHALL correctly parse and display SSE streaming responses from the AI chat endpoint.

#### Scenario: Streaming message display
- **WHEN** user sends a message and backend streams SSE chunks
- **THEN** each chunk is appended to the assistant message in real-time

#### Scenario: Stream completion
- **WHEN** the stream ends with [DONE] marker
- **THEN** the loading indicator stops and message is complete
