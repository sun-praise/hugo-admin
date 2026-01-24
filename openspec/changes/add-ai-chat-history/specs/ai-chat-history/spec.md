## ADDED Requirements

### Requirement: Chat Session Persistence
The system SHALL persist AI chat sessions and messages in SQLite database for retrieval across browser sessions.

#### Scenario: Create new session
- **WHEN** user starts a new AI conversation
- **THEN** system creates a new session with unique ID and timestamp

#### Scenario: Save messages
- **WHEN** user sends a message or AI responds
- **THEN** system saves the message with role, content, and timestamp to the session

#### Scenario: Load session history
- **WHEN** user selects a previous session
- **THEN** system loads all messages in chronological order

### Requirement: Session Management API
The system SHALL provide REST API endpoints for managing chat sessions.

#### Scenario: List sessions
- **WHEN** client requests GET /api/ai/sessions
- **THEN** system returns list of sessions ordered by updated_at descending

#### Scenario: Get session details
- **WHEN** client requests GET /api/ai/sessions/{id}
- **THEN** system returns session metadata and all messages

#### Scenario: Delete session
- **WHEN** client requests DELETE /api/ai/sessions/{id}
- **THEN** system removes session and all associated messages

### Requirement: Frontend Session UI
The system SHALL provide UI for users to manage their chat sessions.

#### Scenario: View session list
- **WHEN** user opens AI chat panel
- **THEN** system displays list of recent sessions with titles

#### Scenario: Switch sessions
- **WHEN** user clicks on a session in the list
- **THEN** system loads and displays that session's conversation

#### Scenario: Create new session
- **WHEN** user clicks "New Chat" button
- **THEN** system creates empty session and clears conversation view
