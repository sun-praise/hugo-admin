from dataclasses import dataclass
from typing import List, Optional, Dict, Any
import os

from pydantic_ai import Agent, RunContext
from pydantic_ai.models.openai import OpenAIChatModel
from pydantic_ai.providers.deepseek import DeepSeekProvider

from services.post_service import PostService
from services.git_service import GitService
from services.hugo_service import HugoServerManager


@dataclass
class Deps:
    post_service: PostService
    git_service: GitService
    hugo_manager: HugoServerManager


class AIService:
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
        self.model = None
        self.agent = None

        if self.enabled:
            # 创建 DeepSeek provider 并传递 API key
            deepseek_provider = DeepSeekProvider(api_key=api_key)

            # 使用 DeepSeek provider 初始化模型
            self.model = OpenAIChatModel(
                model_name=model_name,
                provider=deepseek_provider,
            )

            self.agent = Agent(
                self.model,
                deps_type=Deps,
                system_prompt=(
                    "You are a helpful AI assistant for managing a Hugo blog."
                    "You can search, read, and write blog posts, check git status, and deploy the blog."
                    "Always explain what you are doing before calling tools that modify content or deploy changes."
                ),
            )

            # Register tools
            self._register_tools()
        else:
            print("⚠ AI service disabled: DEEPSEEK_API_KEY not configured")

    def _register_tools(self):
        @self.agent.tool
        async def search_posts(
            ctx: RunContext[Deps], query: str = ""
        ) -> Dict[str, Any]:
            """Search for blog posts.

            Args:
                query: Search query string.
            """
            return ctx.deps.post_service.get_posts(query=query)

        @self.agent.tool
        async def read_post(ctx: RunContext[Deps], file_path: str) -> Dict[str, Any]:
            """Read the content of a blog post.

            Args:
                file_path: Relative path to the post file.
            """
            success, content = ctx.deps.post_service.read_file(file_path)
            return {"success": success, "content": content, "path": file_path}

        @self.agent.tool
        async def write_post(
            ctx: RunContext[Deps], file_path: str, content: str
        ) -> Dict[str, Any]:
            """Create or update a blog post.

            Args:
                file_path: Relative path to the post file.
                content: Markdown content of the post.
            """
            success, message = ctx.deps.post_service.save_file(file_path, content)
            return {"success": success, "message": message}

        @self.agent.tool
        async def git_status(ctx: RunContext[Deps]) -> Dict[str, Any]:
            """Check the current git status of the blog repository."""
            return ctx.deps.git_service.get_status()

        @self.agent.tool
        async def deploy_blog(
            ctx: RunContext[Deps], commit_message: str = "Update via AI Assistant"
        ) -> Dict[str, Any]:
            """Deploy the blog by committing and pushing changes.

            Args:
                commit_message: Message for the git commit.
            """
            return ctx.deps.git_service.publish_system(commit_message)

        @self.agent.tool
        async def manage_server(ctx: RunContext[Deps], action: str) -> Dict[str, Any]:
            """Start or stop the Hugo preview server.

            Args:
                action: 'start' or 'stop'.
            """
            if action == "start":
                success, message = ctx.deps.hugo_manager.start()
            elif action == "stop":
                success, message = ctx.deps.hugo_manager.stop()
            else:
                return {"success": False, "message": f"Unknown action: {action}"}

            return {
                "success": success,
                "message": message,
                "status": ctx.deps.hugo_manager.get_status(),
            }

    async def chat(self, message: str, history: Optional[List[Dict[str, str]]] = None):
        """Handle a chat message."""
        # PydanticAI 会自动管理消息历史
        # 对于简单实现,我们不传递历史记录,让 agent 处理单次对话
        # 如果需要完整历史支持,需要转换为 ModelMessage 格式
        return self.agent.run_stream(message, deps=self.deps)
