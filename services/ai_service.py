"""AI Service using Claude Agent SDK with DeepSeek provider."""

import os
from dataclasses import dataclass
from typing import List, Optional, Dict, Any, AsyncGenerator

from claude_agent_sdk import (
    tool,
    create_sdk_mcp_server,
    ClaudeAgentOptions,
    ClaudeSDKClient,
    AssistantMessage,
    TextBlock,
    ToolUseBlock,
    ToolResultBlock,
)
from claude_agent_sdk.types import StreamEvent

from services.post_service import PostService
from services.git_service import GitService
from services.hugo_service import HugoServerManager


@dataclass
class Deps:
    """Agent dependencies."""

    post_service: PostService
    git_service: GitService
    hugo_manager: HugoServerManager


class AIService:
    """AI Service using Claude Agent SDK with DeepSeek provider."""

    def __init__(
        self,
        api_key: str,
        base_url: str,
        model_name: str,
        post_service: PostService,
        git_service: GitService,
        hugo_manager: HugoServerManager,
    ):
        self.deps = Deps(
            post_service=post_service,
            git_service=git_service,
            hugo_manager=hugo_manager,
        )

        self.enabled = bool(api_key and api_key.strip())

        # Configure DeepSeek Anthropic-compatible endpoint
        if self.enabled:
            os.environ["ANTHROPIC_BASE_URL"] = "https://api.deepseek.com/anthropic"
            os.environ["ANTHROPIC_AUTH_TOKEN"] = api_key
            os.environ["ANTHROPIC_API_KEY"] = api_key
            os.environ["ANTHROPIC_MODEL"] = model_name

        self.model_name = model_name

        if self.enabled:
            # Register tools and create MCP server
            self.mcp_server = create_sdk_mcp_server(
                name="hugo-tools", version="1.0.0", tools=self._create_tools()
            )

            # Create client options
            self.options = ClaudeAgentOptions(
                mcp_servers={"hugo-tools": self.mcp_server},
                allowed_tools=[
                    "mcp__hugo-tools__search_posts",
                    "mcp__hugo-tools__read_post",
                    "mcp__hugo-tools__write_post",
                    "mcp__hugo-tools__git_status",
                    "mcp__hugo-tools__deploy_blog",
                    "mcp__hugo-tools__manage_server",
                ],
                model=model_name,
                include_partial_messages=True,  # Enable streaming
                env={
                    "ANTHROPIC_BASE_URL": "https://api.deepseek.com/anthropic",
                    "ANTHROPIC_AUTH_TOKEN": api_key,
                    "ANTHROPIC_API_KEY": api_key,
                    "ANTHROPIC_MODEL": model_name,
                },
                system_prompt=(
                    "You are a helpful AI assistant for managing a Hugo blog."
                    "You can search, read, and write blog posts, check git status, and deploy the blog."
                    "Always explain what you are doing before calling tools that modify content or deploy changes."
                ),
            )
        else:
            print("⚠ AI service disabled: DEEPSEEK_API_KEY not configured")
            self.mcp_server = None
            self.options = None

    def _create_tools(self):
        """Create all tools for the Hugo blog assistant."""

        deps = self.deps

        @tool("search_posts", "Search for blog posts", {"query": str})
        async def search_posts(args: Dict[str, Any]) -> Dict[str, Any]:
            """Search for blog posts.

            Args:
                query: Search query string.
            """
            query = args.get("query", "")
            return deps.post_service.get_posts(query=query)

        @tool("read_post", "Read the content of a blog post", {"file_path": str})
        async def read_post(args: Dict[str, Any]) -> Dict[str, Any]:
            """Read the content of a blog post.

            Args:
                file_path: Relative path to the post file.
            """
            file_path = args["file_path"]
            success, content = deps.post_service.read_file(file_path)
            return {"success": success, "content": content, "path": file_path}

        @tool(
            "write_post",
            "Create or update a blog post",
            {"file_path": str, "content": str},
        )
        async def write_post(args: Dict[str, Any]) -> Dict[str, Any]:
            """Create or update a blog post.

            Args:
                file_path: Relative path to the post file.
                content: Markdown content of the post.
            """
            file_path = args["file_path"]
            content = args["content"]
            success, message = deps.post_service.save_file(file_path, content)
            return {"success": success, "message": message}

        @tool("git_status", "Check the current git status of the blog repository", {})
        async def git_status(args: Dict[str, Any]) -> Dict[str, Any]:
            """Check the current git status of the blog repository."""
            return deps.git_service.get_status()

        @tool(
            "deploy_blog",
            "Deploy the blog by committing and pushing changes",
            {"commit_message": str},
        )
        async def deploy_blog(args: Dict[str, Any]) -> Dict[str, Any]:
            """Deploy the blog by committing and pushing changes.

            Args:
                commit_message: Message for the git commit.
            """
            commit_message = args.get("commit_message", "Update via AI Assistant")
            return deps.git_service.publish_system(commit_message)

        @tool("manage_server", "Start or stop the Hugo preview server", {"action": str})
        async def manage_server(args: Dict[str, Any]) -> Dict[str, Any]:
            """Start or stop the Hugo preview server.

            Args:
                action: 'start' or 'stop'.
            """
            action = args["action"]
            if action == "start":
                success, message = deps.hugo_manager.start()
            elif action == "stop":
                success, message = deps.hugo_manager.stop()
            else:
                return {"success": False, "message": f"Unknown action: {action}"}

            return {
                "success": success,
                "message": message,
                "status": deps.hugo_manager.get_status(),
            }

        return [
            search_posts,
            read_post,
            write_post,
            git_status,
            deploy_blog,
            manage_server,
        ]

    async def chat(
        self,
        message: str,
        history: Optional[List[Dict[str, str]]] = None,
    ) -> AsyncGenerator:
        """Handle a chat message and stream responses.

        Args:
            message: User message.
            history: Message history (optional).

        Yields:
            Claude Agent SDK messages (AssistantMessage, StreamEvent, etc.)
        """
        if not self.enabled:
            raise RuntimeError("AI service is not configured")

        async with ClaudeSDKClient(options=self.options) as client:
            # Send the query
            await client.query(message)

            # Stream responses
            async for msg in client.receive_response():
                yield msg
