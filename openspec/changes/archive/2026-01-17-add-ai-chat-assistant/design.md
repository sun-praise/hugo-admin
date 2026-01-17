# Design: AI Chat Assistant

## Architecture
The AI Assistant follows a classic Agent-Tool-User loop:
1. **User** sends a message via the **Chat UI**.
2. **Flask API** forwards the request to the **AIService**.
3. **AIService** (built with PydanticAI) initializes an **Agent**.
4. **Agent** uses **DeepSeek LLM** to reason about the request.
5. **Agent** calls **Tools** (methods in `AIService` that wrap `PostService`, `GitService`, etc.).
6. **Agent** returns a response (streamed or static) to the **Chat UI**.

## Components

### 1. AIService (`services/ai_service.py`)
- **Agent Definition**: Using `pydantic_ai.Agent`.
- **Dependencies (`Deps`)**: A dataclass holding instances of `PostService`, `GitService`, and `HugoServerManager`.
- **Tools**:
    - `list_posts`: Search and list articles.
    - `read_post`: Fetch article content.
    - `write_post`: Create or update articles.
    - `get_git_status`: Check pending changes.
    - `publish_blog`: Commit and push changes.
    - `server_control`: Start/Stop Hugo server.

### 2. API Layer (`app.py`)
- `POST /api/ai/chat`: Main endpoint.
- Uses `Response.stream` if possible for real-time feedback.

### 3. Frontend Chat UI
- **Alpine.js State**:
    - `messages`: List of `{role, content}`.
    - `isOpen`: Chatbox visibility.
    - `isLoading`: Waiting for AI.
- **Tailwind Components**:
    - Floating button (bottom-right).
    - Chat window with scrollable message area.
    - Markdown rendering via `marked.js`.

## Data Model
Messages are not persisted in the database for the initial version to keep it lightweight. Context is maintained in the frontend state and passed to the backend if needed, or maintained via PydanticAI's session management.

## Safety & Security
- All tool calls that modify files (`write_post`, `publish_blog`) must be explicitly explained by the AI.
- Tools will use existing safety checks in `PostService` (e.g., `_is_safe_path`).
- API keys will be loaded from `config_local.py` or environment variables.
