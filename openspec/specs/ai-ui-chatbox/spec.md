# ai-ui-chatbox Specification

## Purpose
TBD - created by archiving change add-ai-chat-assistant. Update Purpose after archive.
## Requirements
### Requirement: Real-time chat interface
The system SHALL provide a floating chat box in the web interface for interacting with the AI Agent.

#### Scenario: Opening the chat box
- **Given** the user is on the Dashboard page
- **When** the user clicks the AI Chat icon in the bottom-right corner
- **Then** the chat window should expand
- **And** show a welcome message from the AI.

#### Scenario: Sending a message
- **Given** the chat box is open
- **When** the user types "Hello" and presses Enter
- **Then** the message should appear in the chat history
- **And** a loading indicator should appear
- **And** the AI's response should be rendered (supporting Markdown).

