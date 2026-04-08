"""AI Service using Claude Agent SDK with DeepSeek provider."""

import os
from dataclasses import dataclass
from typing import Any, AsyncGenerator, Dict, List, Optional

from claude_agent_sdk import (
    ClaudeAgentOptions,
    ClaudeSDKClient,
    create_sdk_mcp_server,
    tool,
)

from services.git_service import GitService
from services.hugo_service import HugoServerManager
from services.post_service import PostService


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
                    "mcp__hugo-tools__git_status",
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
                    "You are a helpful read-only AI assistant for a Hugo blog. "
                    "You can search and read blog posts. You cannot modify, "
                    "create, deploy, or manage the blog in any way. "
                    "Always explain what you are doing before calling tools."
                ),
            )
        else:
            print("⚠ AI service disabled: DEEPSEEK_API_KEY not configured")
            self.mcp_server = None
            self.options = None

    def _mcp_text(self, text: str) -> Dict[str, Any]:
        """Wrap text in MCP content format."""
        return {"content": [{"type": "text", "text": text}]}

    def _create_tools(self):
        """Create all tools for the Hugo blog assistant."""

        deps = self.deps
        mcp_text = self._mcp_text

        @tool("search_posts", "Search for blog posts", {"query": str})
        async def search_posts(args: Dict[str, Any]) -> Dict[str, Any]:
            query = args.get("query", "")
            result = deps.post_service.get_posts(query=query, per_page=10)

            if not result["posts"]:
                return mcp_text(f"未找到匹配 '{query}' 的文章")

            lines = [f"找到 {result['total']} 篇文章：\n"]
            for post in result["posts"]:
                lines.append(f"- **{post['title']}**")
                lines.append(f"  路径: `{post['path']}`")
                lines.append(f"  日期: {post['date']}\n")
            return mcp_text("\n".join(lines))

        @tool("read_post", "Read the content of a blog post", {"file_path": str})
        async def read_post(args: Dict[str, Any]) -> Dict[str, Any]:
            file_path = args["file_path"]
            success, content = deps.post_service.read_file(file_path)
            if success:
                return mcp_text(f"文件 `{file_path}` 内容：\n\n{content}")
            return mcp_text(f"读取失败: {content}")

        @tool(
            "write_post",
            "Create or update a blog post",
            {"file_path": str, "content": str},
        )
        async def write_post(args: Dict[str, Any]) -> Dict[str, Any]:
            file_path = args["file_path"]
            content = args["content"]
            success, message = deps.post_service.save_file(file_path, content)
            if success:
                return mcp_text(f"✅ 文件 `{file_path}` 保存成功")
            return mcp_text(f"❌ 保存失败: {message}")

        @tool("git_status", "Check the current git status of the blog repository", {})
        async def git_status(args: Dict[str, Any]) -> Dict[str, Any]:
            status = deps.git_service.get_status()
            lines = ["Git 仓库状态：\n"]
            lines.append(f"- 分支: `{status.get('branch', 'unknown')}`")
            lines.append(f"- 是否干净: {'是' if status.get('clean') else '否'}")
            if status.get("changes"):
                lines.append(f"- 变更文件数: {len(status['changes'])}")
                for change in status["changes"][:10]:
                    lines.append(f"  - {change}")
            return mcp_text("\n".join(lines))

        @tool(
            "deploy_blog",
            "Deploy the blog by committing and pushing changes",
            {"commit_message": str},
        )
        async def deploy_blog(args: Dict[str, Any]) -> Dict[str, Any]:
            commit_message = args.get("commit_message", "Update via AI Assistant")
            result = deps.git_service.publish_system(commit_message)
            if result.get("success"):
                return mcp_text(f"✅ 部署成功！提交信息: {commit_message}")
            return mcp_text(f"❌ 部署失败: {result.get('message', '未知错误')}")

        @tool("manage_server", "Start or stop the Hugo preview server", {"action": str})
        async def manage_server(args: Dict[str, Any]) -> Dict[str, Any]:
            action = args["action"]
            if action == "start":
                success, message = deps.hugo_manager.start()
            elif action == "stop":
                success, message = deps.hugo_manager.stop()
            else:
                return mcp_text(f"❌ 未知操作: {action}，请使用 'start' 或 'stop'")

            status = deps.hugo_manager.get_status()
            icon = "✅" if success else "❌"
            return mcp_text(
                f"{icon} {message}\n服务器状态: {'运行中' if status.get('running') else '已停止'}"
            )

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
