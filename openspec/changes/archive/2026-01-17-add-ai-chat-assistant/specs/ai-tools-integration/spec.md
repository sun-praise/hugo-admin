# Capability: AI Tools Integration

## ADDED Requirements

### Requirement: Tool access to system services
The AI Agent SHALL have controlled access to existing blog management services.

#### Scenario: Git status check
- **Given** the `git_status_tool` is active
- **When** the user asks "Are there any unsaved changes?"
- **Then** the agent should call the `git_service.get_status()`
- **And** report if there are staged, unstaged, or untracked files.

#### Scenario: Hugo server management
- **Given** the `server_control_tool` is active
- **When** the user says "Start the preview server"
- **Then** the agent should call `hugo_manager.start()`
- **And** provide a link to the preview URL.
