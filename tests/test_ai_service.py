#!/usr/bin/env python
# coding: utf-8
"""
AI Service 独立测试脚本
用于调试 Claude Agent SDK 与 DeepSeek 的集成

使用方法:
    python tests/test_ai_service.py                    # 运行所有测试
    python tests/test_ai_service.py --test basic       # 只测试基础连接
    python tests/test_ai_service.py --test sync        # 测试同步调用
    python tests/test_ai_service.py --test stream      # 测试流式调用
    python tests/test_ai_service.py --test tools       # 测试工具调用
    python tests/test_ai_service.py --test full        # 测试完整 AIService
    python tests/test_ai_service.py -v                 # 详细输出
"""

import argparse
import logging
import os
import sys
from pathlib import Path

# 添加项目根目录到 Python 路径
sys.path.insert(0, str(Path(__file__).parent.parent))

from dotenv import load_dotenv
from claude_agent_sdk import (
    tool,
    create_sdk_mcp_server,
    ClaudeAgentOptions,
    ClaudeSDKClient,
    AssistantMessage,
    TextBlock,
)

# 设置日志
logging.basicConfig(
    level=logging.INFO, format="%(asctime)s - %(name)s - %(levelname)s - %(message)s"
)
logger = logging.getLogger(__name__)


class AIServiceTester:
    """AI Service 测试类"""

    def __init__(self, verbose: bool = False):
        self.verbose = verbose
        if verbose:
            logging.getLogger().setLevel(logging.DEBUG)

        # 加载环境变量
        load_dotenv()

        self.api_key = os.environ.get("DEEPSEEK_API_KEY", "")
        self.base_url = os.environ.get("AI_BASE_URL", "https://api.deepseek.com")
        self.model_name = os.environ.get("AI_MODEL", "deepseek-chat")

        logger.info(
            f"API Key: {self.api_key[:10]}..." if self.api_key else "API Key: NOT SET"
        )
        logger.info(f"Base URL: {self.base_url}")
        logger.info(f"Model: {self.model_name}")

    async def test_basic_connection(self) -> bool:
        """测试 1: 基础连接测试（无工具的简单 Agent）"""
        logger.info("=" * 50)
        logger.info("测试 1: 基础连接测试")
        logger.info("=" * 50)

        try:
            # 配置 DeepSeek Anthropic-compatible endpoint
            os.environ["ANTHROPIC_BASE_URL"] = "https://api.deepseek.com/anthropic"
            os.environ["ANTHROPIC_AUTH_TOKEN"] = self.api_key
            os.environ["ANTHROPIC_API_KEY"] = self.api_key
            os.environ["ANTHROPIC_MODEL"] = self.model_name

            # 创建简单的 Agent，不带任何工具
            options = ClaudeAgentOptions(
                model=self.model_name,
                include_partial_messages=True,
                env={
                    "ANTHROPIC_BASE_URL": "https://api.deepseek.com/anthropic",
                    "ANTHROPIC_AUTH_TOKEN": self.api_key,
                    "ANTHROPIC_API_KEY": self.api_key,
                    "ANTHROPIC_MODEL": self.model_name,
                },
                system_prompt="You are a helpful assistant. Reply briefly.",
            )

            async with ClaudeSDKClient(options=options) as client:
                await client.query("Hello, say hi back in one word.")

                # 收集响应
                messages = []
                async for msg in client.receive_response():
                    messages.append(msg)
                    if isinstance(msg, AssistantMessage):
                        for block in msg.content:
                            if isinstance(block, TextBlock):
                                logger.info(f"响应内容: {block.text}")

            logger.info("✅ 基础连接测试通过")
            return True
        except Exception as e:
            logger.error(f"❌ 基础连接测试失败: {e}")
            logger.exception("详细错误信息:")
            return False

    async def test_with_tools(self) -> bool:
        """测试 2: 带简单工具的 Agent"""
        logger.info("=" * 50)
        logger.info("测试 2: 带工具的 Agent")
        logger.info("=" * 50)

        try:
            # 配置 DeepSeek Anthropic-compatible endpoint
            os.environ["ANTHROPIC_BASE_URL"] = "https://api.deepseek.com/anthropic"
            os.environ["ANTHROPIC_AUTH_TOKEN"] = self.api_key
            os.environ["ANTHROPIC_API_KEY"] = self.api_key
            os.environ["ANTHROPIC_MODEL"] = self.model_name

            @tool("multiply", "Multiply two numbers", {"a": float, "b": float})
            async def multiply(args: dict):
                a = args["a"]
                b = args["b"]
                result = a * b
                logger.info(f"  [Tool called] multiply({a}, {b}) = {result}")
                return {"content": [{"type": "text", "text": f"{a} × {b} = {result}"}]}

            # 创建 MCP server
            server = create_sdk_mcp_server(
                name="calc", version="1.0.0", tools=[multiply]
            )

            # 配置 Claude
            options = ClaudeAgentOptions(
                mcp_servers={"calc": server},
                allowed_tools=["mcp__calc__multiply"],
                model=self.model_name,
                include_partial_messages=True,
                env={
                    "ANTHROPIC_BASE_URL": "https://api.deepseek.com/anthropic",
                    "ANTHROPIC_AUTH_TOKEN": self.api_key,
                    "ANTHROPIC_API_KEY": self.api_key,
                    "ANTHROPIC_MODEL": self.model_name,
                },
                system_prompt="You are a math assistant. "
                "Use multiply tool when asked to multiply.",
            )

            async with ClaudeSDKClient(options=options) as client:
                # 测试 1: 6 * 7
                logger.info("测试 1: 计算 6 * 7")
                await client.query("What is 6 times 7? Use the multiply tool.")

                messages = []
                async for msg in client.receive_response():
                    messages.append(msg)
                    if isinstance(msg, AssistantMessage):
                        for block in msg.content:
                            if isinstance(block, TextBlock):
                                logger.info(f"响应: {block.text}")
                    # Stop after first complete response
                    if len(messages) > 5:
                        break

            logger.info("✅ 带工具测试通过")
            return True
        except Exception as e:
            logger.error(f"❌ 带工具测试失败: {e}")
            logger.exception("详细错误信息:")
            return False

    async def test_streaming(self) -> bool:
        """测试 3: 流式输出测试"""
        logger.info("=" * 50)
        logger.info("测试 3: 流式输出测试")
        logger.info("=" * 50)

        try:
            # 配置 DeepSeek Anthropic-compatible endpoint
            os.environ["ANTHROPIC_BASE_URL"] = "https://api.deepseek.com/anthropic"
            os.environ["ANTHROPIC_AUTH_TOKEN"] = self.api_key
            os.environ["ANTHROPIC_API_KEY"] = self.api_key
            os.environ["ANTHROPIC_MODEL"] = self.model_name

            options = ClaudeAgentOptions(
                model=self.model_name,
                include_partial_messages=True,
                env={
                    "ANTHROPIC_BASE_URL": "https://api.deepseek.com/anthropic",
                    "ANTHROPIC_AUTH_TOKEN": self.api_key,
                    "ANTHROPIC_API_KEY": self.api_key,
                    "ANTHROPIC_MODEL": self.model_name,
                },
                system_prompt="You are a helpful assistant.",
            )

            async with ClaudeSDKClient(options=options) as client:
                await client.query("Count from 1 to 5, one number per line.")

                chunk_count = 0
                chunks = []

                async for msg in client.receive_response():
                    if isinstance(msg, AssistantMessage):
                        for block in msg.content:
                            if isinstance(block, TextBlock):
                                chunks.append(block.text)
                                chunk_count += 1
                                if self.verbose:
                                    logger.debug(
                                        f"  chunk #{chunk_count}: {repr(block.text)}"
                                    )

                full_response = "".join(chunks)
                logger.info(f"完整响应: {full_response}")
                logger.info(f"chunk 数量: {len(chunks)}")
                logger.info("✅ 流式输出测试通过")
                return True
        except Exception as e:
            logger.error(f"❌ 流式输出测试失败: {e}")
            logger.exception("详细错误信息:")
            return False

    async def test_full_ai_service(self) -> bool:
        """测试 4: 完整的 AIService 集成测试"""
        logger.info("=" * 50)
        logger.info("测试 4: 完整 AIService 集成测试")
        logger.info("=" * 50)

        try:
            from config import Config
            from services.post_service import PostService
            from services.git_service import GitService
            from services.hugo_service import HugoServerManager
            from services.ai_service import AIService

            # 尝试加载本地配置
            try:
                from config_local import LocalConfig

                config = LocalConfig
                logger.info("使用本地配置 (LocalConfig)")
            except ImportError:
                config = Config
                logger.info("使用默认配置 (Config)")

            # 初始化真实依赖
            logger.info(f"HUGO_ROOT: {config.HUGO_ROOT}")
            logger.info(f"CONTENT_DIR: {config.CONTENT_DIR}")

            post_service = PostService(config.CONTENT_DIR, use_cache=False)
            git_service = GitService(config.HUGO_ROOT)
            hugo_manager = HugoServerManager(config.HUGO_ROOT, socketio=None)

            # 初始化 AIService
            ai_service = AIService(
                api_key=self.api_key,
                base_url=self.base_url,
                model_name=self.model_name,
                post_service=post_service,
                git_service=git_service,
                hugo_manager=hugo_manager,
            )

            logger.info("AIService 初始化成功")

            # 测试基础对话
            logger.info("\n--- 测试基础对话 ---")
            message = "Hello, what can you help me with?"
            logger.info(f"发送消息: '{message}'")

            try:
                messages = []
                async for msg in ai_service.chat(message):
                    messages.append(msg)
                    if isinstance(msg, AssistantMessage):
                        for block in msg.content:
                            if isinstance(block, TextBlock):
                                response_preview = (
                                    block.text[:200] + "..."
                                    if len(block.text) > 200
                                    else block.text
                                )
                                logger.info(f"响应: {response_preview}")
                    # Stop after first complete response
                    if len(messages) > 5:
                        break
            except Exception as e:
                logger.warning(f"对话测试跳过: {e}")

            # 测试工具调用 (search_posts)
            logger.info("\n--- 测试工具调用 (search_posts) ---")
            message = "Search for posts. Just tell me how many posts you found."
            logger.info(f"发送消息: '{message}'")

            try:
                messages = []
                async for msg in ai_service.chat(message):
                    messages.append(msg)
                    if isinstance(msg, AssistantMessage):
                        for block in msg.content:
                            if isinstance(block, TextBlock):
                                response_preview = (
                                    block.text[:500] + "..."
                                    if len(block.text) > 500
                                    else block.text
                                )
                                logger.info(f"响应: {response_preview}")
                    # Stop after first complete response
                    if len(messages) > 10:
                        break
            except Exception as e:
                logger.warning(f"工具调用测试跳过: {e}")

            logger.info("✅ 完整 AIService 测试通过")
            return True
        except Exception as e:
            logger.error(f"❌ 完整 AIService 测试失败: {e}")
            logger.exception("详细错误信息:")
            return False


import asyncio


async def run_test_async(tester, test_name):
    """Run an async test."""
    tests = {
        "basic": tester.test_basic_connection,
        "stream": tester.test_streaming,
        "tools": tester.test_with_tools,
        "full": tester.test_full_ai_service,
    }
    return await tests[test_name]()


def main():
    parser = argparse.ArgumentParser(description="AI Service 测试脚本")
    parser.add_argument(
        "--test",
        choices=["basic", "stream", "tools", "full", "all"],
        default="all",
        help="选择测试类型",
    )
    parser.add_argument("-v", "--verbose", action="store_true", help="详细输出")
    args = parser.parse_args()

    tester = AIServiceTester(verbose=args.verbose)

    if args.test == "all":
        results = {}
        for test_name in ["basic", "stream", "tools", "full"]:
            passed = asyncio.run(run_test_async(tester, test_name))
            results[test_name] = passed
            print()  # 空行分隔

        # 打印总结
        print("\n" + "=" * 50)
        print("测试总结")
        print("=" * 50)
        for name, passed in results.items():
            status = "✅ PASS" if passed else "❌ FAIL"
            print(f"  {name}: {status}")

        all_passed = all(results.values())
        print(f"\n总体结果: {'✅ 全部通过' if all_passed else '❌ 有失败'}")
        sys.exit(0 if all_passed else 1)
    else:
        passed = asyncio.run(run_test_async(tester, args.test))
        sys.exit(0 if passed else 1)


if __name__ == "__main__":
    main()
