# Proposal: Add AI Chat Assistant

## Problem
Users currently manage Hugo blog content through traditional GUI elements (buttons, forms, lists). While functional, it lacks the flexibility and efficiency of natural language operations, especially for complex tasks like multi-step content updates, semantic searches, or automated deployment flows based on high-level intent.

## Proposed Solution
Introduce an AI-powered Chat Assistant integrated into the Hugo Admin interface. 
- Use **PydanticAI** as the agent framework for type-safe tool execution.
- Use **DeepSeek** (OpenAI-compatible) as the primary LLM provider.
- Implement a floating chat box in the UI for seamless interaction.
- Empower the agent with tools to interact with existing `PostService`, `GitService`, and `HugoServerManager`.

## Scope
- Backend: `AIService` implementation using PydanticAI.
- Backend: Tool definitions for blog management.
- API: `/api/ai/chat` endpoint (supporting streaming).
- Frontend: Floating chat UI using Alpine.js and Tailwind CSS.
- Configuration: AI provider and API key management in `config.py`.

## Deliverables
- `services/ai_service.py`: Core agent logic.
- `app.py` updates: New API routes.
- `templates/base.html` updates: Chatbox component.
- `requirements.txt` updates: `pydantic-ai`, `openai`, `python-dotenv`.
