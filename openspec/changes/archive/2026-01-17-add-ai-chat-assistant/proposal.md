# Change: Add AI Chat Assistant

## Why
Users currently manage Hugo blog content through traditional GUI elements (buttons, forms, lists). This lacks the flexibility and efficiency of natural language operations, especially for complex tasks like multi-step content updates, semantic searches, or automated deployment flows based on high-level intent.

## What Changes
- Add AI-powered Chat Assistant integrated into the Hugo Admin interface
- Use **PydanticAI** as the agent framework for type-safe tool execution
- Use **DeepSeek** (OpenAI-compatible) as the primary LLM provider
- Implement a floating chat box in the UI for seamless interaction
- Empower the agent with tools to interact with existing `PostService`, `GitService`, and `HugoServerManager`

## Impact
- Affected specs: ai-agent-core, ai-tools-integration, ai-ui-chatbox
- Affected code:
  - `services/ai_service.py`: Core agent logic
  - `app.py`: New API routes (`/api/ai/chat`)
  - `templates/base.html`: Chatbox component
  - `requirements.txt`: `pydantic-ai`, `openai`, `python-dotenv`
