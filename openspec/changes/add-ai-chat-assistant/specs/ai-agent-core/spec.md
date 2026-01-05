# Capability: AI Agent Core

## MODIFIED Requirements

### Requirement: AI reasoning and tool execution
The system shall provide an AI agent capable of reasoning and executing tools to manage Hugo blog content.

#### Scenario: User requests a search for articles
- **Given** an AI Agent initialized with DeepSeek model
- **And** the `list_posts` tool is available
- **When** the user asks "Find articles about Python"
- **Then** the agent should call the `list_posts` tool with `query="Python"`
- **And** provide a natural language summary of the results.

#### Scenario: User requests to create a new post
- **Given** the `create_post` tool is available
- **When** the user says "Create a new post about PydanticAI"
- **Then** the agent should call the `create_post` tool with `title="PydanticAI"`
- **And** confirm the successful creation with the file path.
