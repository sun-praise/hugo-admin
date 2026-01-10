#!/usr/bin/env python
# coding: utf-8
"""
AI Service 独立测试脚本
用于调试 pydantic-ai 与 DeepSeek 的集成

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
from pydantic_ai import Agent, RunContext
from pydantic_ai.models.openai import OpenAIChatModel
from pydantic_ai.providers.deepseek import DeepSeekProvider

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

    def test_basic_connection(self) -> bool:
        """测试 1: 基础连接测试（无工具的简单 Agent）"""
        logger.info("=" * 50)
        logger.info("测试 1: 基础连接测试")
        logger.info("=" * 50)

        try:
            # 创建最简单的 Agent，不带任何工具
            provider = DeepSeekProvider(api_key=self.api_key)
            model = OpenAIChatModel(model_name=self.model_name, provider=provider)
            agent = Agent(
                model, system_prompt="You are a helpful assistant. Reply briefly."
            )

            # 使用 run_sync 进行简单测试
            logger.info("发送测试消息: 'Hello, say hi back in one word.'")
            result = agent.run_sync("Hello, say hi back in one word.")

            logger.info(f"响应类型: {type(result)}")
            logger.info(f"响应内容: {result.output}")
            logger.info(f"消息数量: {len(result.all_messages())}")
            logger.info("✅ 基础连接测试通过")
            return True
        except Exception as e:
            logger.error(f"❌ 基础连接测试失败: {e}")
            logger.exception("详细错误信息:")
            return False

    def test_run_sync(self) -> bool:
        """测试 2: 同步非流式调用 run_sync()"""
        logger.info("=" * 50)
        logger.info("测试 2: 同步非流式调用 (run_sync)")
        logger.info("=" * 50)

        try:
            provider = DeepSeekProvider(api_key=self.api_key)
            model = OpenAIChatModel(model_name=self.model_name, provider=provider)
            agent = Agent(model, system_prompt="You are a helpful assistant.")

            message = "What is 2 + 2? Reply with just the number."
            logger.info(f"发送消息: '{message}'")

            result = agent.run_sync(message)

            logger.info(f"响应类型: {type(result)}")
            logger.info(f"响应内容: {result.output}")
            if self.verbose:
                logger.debug(f"所有消息: {result.all_messages()}")
            logger.info("✅ run_sync 测试通过")
            return True
        except Exception as e:
            logger.error(f"❌ run_sync 测试失败: {e}")
            logger.exception("详细错误信息:")
            return False

    def test_run_stream_sync(self) -> bool:
        """测试 3: 同步流式调用 run_stream_sync()"""
        logger.info("=" * 50)
        logger.info("测试 3: 同步流式调用 (run_stream_sync)")
        logger.info("=" * 50)

        try:
            provider = DeepSeekProvider(api_key=self.api_key)
            model = OpenAIChatModel(model_name=self.model_name, provider=provider)
            agent = Agent(model, system_prompt="You are a helpful assistant.")

            message = "Count from 1 to 5, one number per line."
            logger.info(f"发送消息: '{message}'")

            # 使用 run_stream_sync
            result = agent.run_stream_sync(message)

            logger.info("开始接收流式响应:")
            chunks = []
            chunk_count = 0
            for chunk in result.stream_text(delta=True):
                chunks.append(chunk)
                chunk_count += 1
                if self.verbose:
                    logger.debug(f"  chunk #{chunk_count}: {repr(chunk)}")

            full_response = "".join(chunks)
            logger.info(f"完整响应: {full_response}")
            logger.info(f"chunk 数量: {len(chunks)}")
            logger.info("✅ run_stream_sync 测试通过")
            return True
        except Exception as e:
            logger.error(f"❌ run_stream_sync 测试失败: {e}")
            logger.exception("详细错误信息:")
            return False

    def test_with_tools(self) -> bool:
        """测试 4: 带简单工具的 Agent"""
        logger.info("=" * 50)
        logger.info("测试 4: 带工具的 Agent")
        logger.info("=" * 50)

        try:
            from dataclasses import dataclass

            @dataclass
            class TestDeps:
                multiplier: int = 2

            provider = DeepSeekProvider(api_key=self.api_key)
            model = OpenAIChatModel(model_name=self.model_name, provider=provider)
            agent = Agent(
                model,
                deps_type=TestDeps,
                system_prompt="You are a math assistant. Use the multiply tool when asked to multiply.",
            )

            @agent.tool
            async def multiply(ctx: RunContext[TestDeps], a: int, b: int) -> int:
                """Multiply two numbers together.

                Args:
                    a: First number
                    b: Second number
                """
                result = a * b * ctx.deps.multiplier
                logger.info(
                    f"  [Tool called] multiply({a}, {b}) * {ctx.deps.multiplier} = {result}"
                )
                return result

            deps = TestDeps(multiplier=1)
            message = "What is 6 times 7? Use the multiply tool."
            logger.info(f"发送消息: '{message}'")

            # 测试 run_sync 带工具
            logger.info("测试 run_sync 带工具:")
            result = agent.run_sync(message, deps=deps)
            logger.info(f"响应: {result.output}")

            # 测试 run_stream_sync 带工具
            logger.info("测试 run_stream_sync 带工具:")
            result_stream = agent.run_stream_sync(
                "What is 8 times 9? Use the multiply tool.", deps=deps
            )
            chunks = []
            for chunk in result_stream.stream_text(delta=True):
                chunks.append(chunk)
            logger.info(f"流式响应: {''.join(chunks)}")

            logger.info("✅ 带工具测试通过")
            return True
        except Exception as e:
            logger.error(f"❌ 带工具测试失败: {e}")
            logger.exception("详细错误信息:")
            return False

    def test_full_ai_service(self) -> bool:
        """测试 5: 完整的 AIService 集成测试"""
        logger.info("=" * 50)
        logger.info("测试 5: 完整 AIService 集成测试")
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

            # 测试 run_sync
            logger.info("\n--- 测试 run_sync ---")
            message = "Hello, what can you help me with?"
            logger.info(f"发送消息: '{message}'")
            result = ai_service.agent.run_sync(message, deps=ai_service.deps)
            response_preview = (
                result.output[:200] + "..."
                if len(result.output) > 200
                else result.output
            )
            logger.info(f"响应: {response_preview}")

            # 测试 run_stream_sync
            logger.info("\n--- 测试 run_stream_sync ---")
            message = "List 3 things you can do for me."
            logger.info(f"发送消息: '{message}'")
            result_stream = ai_service.agent.run_stream_sync(
                message, deps=ai_service.deps
            )
            chunks = []
            for chunk in result_stream.stream_text(delta=True):
                chunks.append(chunk)
                if self.verbose:
                    logger.debug(f"  chunk: {repr(chunk)}")
            full_response = "".join(chunks)
            response_preview = (
                full_response[:200] + "..."
                if len(full_response) > 200
                else full_response
            )
            logger.info(f"流式响应: {response_preview}")
            logger.info(f"chunk 数量: {len(chunks)}")

            # 测试工具调用 (search_posts)
            logger.info("\n--- 测试工具调用 (search_posts) ---")
            message = "Search for posts. Just tell me how many posts you found."
            logger.info(f"发送消息: '{message}'")
            result = ai_service.agent.run_sync(message, deps=ai_service.deps)
            response_preview = (
                result.output[:500] + "..."
                if len(result.output) > 500
                else result.output
            )
            logger.info(f"响应: {response_preview}")

            logger.info("✅ 完整 AIService 测试通过")
            return True
        except Exception as e:
            logger.error(f"❌ 完整 AIService 测试失败: {e}")
            logger.exception("详细错误信息:")
            return False


def main():
    parser = argparse.ArgumentParser(description="AI Service 测试脚本")
    parser.add_argument(
        "--test",
        choices=["basic", "sync", "stream", "tools", "full", "all"],
        default="all",
        help="选择测试类型",
    )
    parser.add_argument("-v", "--verbose", action="store_true", help="详细输出")
    args = parser.parse_args()

    tester = AIServiceTester(verbose=args.verbose)

    tests = {
        "basic": tester.test_basic_connection,
        "sync": tester.test_run_sync,
        "stream": tester.test_run_stream_sync,
        "tools": tester.test_with_tools,
        "full": tester.test_full_ai_service,
    }

    if args.test == "all":
        results = {}
        for name, test_func in tests.items():
            results[name] = test_func()
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
        passed = tests[args.test]()
        sys.exit(0 if passed else 1)


if __name__ == "__main__":
    main()
