# coding: utf-8
"""
Git 操作服务
负责 Hugo 博客的 git 提交和推送操作
"""

import logging
import subprocess
from datetime import datetime
from pathlib import Path

logger = logging.getLogger(__name__)


class GitService:
    """Git 操作服务"""

    def __init__(self, repo_path, database=None):
        """
        初始化 Git 服务

        Args:
            repo_path: Git 仓库路径（Hugo 项目根目录）
            database: 可选的 Database 实例，用于持久化推送历史；
                为 None 时 push() 的记录行为是 no-op（便于测试）。
        """
        self.repo_path = Path(repo_path)
        if not self.repo_path.exists():
            raise ValueError(f"仓库路径不存在: {repo_path}")
        self.database = database

    def _run_git_command(self, command, check=True):
        """
        执行 git 命令

        Args:
            command: git 命令列表，例如 ['status']
            check: 是否检查返回码

        Returns:
            tuple: (success, stdout, stderr)
        """
        try:
            full_command = ["git"] + command
            result = subprocess.run(
                full_command,
                cwd=self.repo_path,
                capture_output=True,
                text=True,
                check=check,
            )
            return True, result.stdout, result.stderr
        except subprocess.CalledProcessError as e:
            logger.error(f"Git 命令执行失败: {' '.join(command)}\n{e.stderr}")
            return False, e.stdout, e.stderr
        except Exception as e:
            logger.error(f"执行 git 命令时发生错误: {e}")
            return False, "", str(e)

    def is_git_repo(self):
        """
        检查是否为有效的 git 仓库

        Returns:
            bool: 是否为 git 仓库
        """
        git_dir = self.repo_path / ".git"
        return git_dir.exists() and git_dir.is_dir()

    def get_status(self):
        """
        获取 git 状态

        Returns:
            dict: 包含状态信息的字典
                - success: 是否成功
                - has_changes: 是否有改动
                - staged: 暂存的文件列表
                - unstaged: 未暂存的文件列表
                - untracked: 未跟踪的文件列表
                - message: 状态消息
        """
        if not self.is_git_repo():
            return {
                "success": False,
                "has_changes": False,
                "message": "当前目录不是有效的 git 仓库",
            }

        # 获取状态
        success, stdout, stderr = self._run_git_command(["status", "--porcelain"])
        if not success:
            return {
                "success": False,
                "has_changes": False,
                "message": f"获取 git 状态失败: {stderr}",
            }

        # 解析状态输出
        staged = []
        unstaged = []
        untracked = []

        for line in stdout.strip().split("\n"):
            if not line:
                continue

            status = line[:2]
            filepath = line[3:].strip()

            # 第一个字符表示暂存区状态，第二个字符表示工作区状态
            if status[0] != " " and status[0] != "?":
                staged.append(filepath)
            if status[1] != " ":
                unstaged.append(filepath)
            if status == "??":
                untracked.append(filepath)

        has_changes = bool(staged or unstaged or untracked)

        return {
            "success": True,
            "has_changes": has_changes,
            "staged": staged,
            "unstaged": unstaged,
            "untracked": untracked,
            "message": "获取状态成功",
        }

    def add_all(self):
        """
        添加所有改动到暂存区

        Returns:
            tuple: (success, message)
        """
        success, stdout, stderr = self._run_git_command(["add", "-A"])
        if success:
            return True, "已添加所有改动到暂存区"
        else:
            return False, f"添加文件失败: {stderr}"

    def commit(self, message=None):
        """
        提交暂存的改动

        Args:
            message: 提交消息，如果为 None 则使用默认消息

        Returns:
            tuple: (success, message)
        """
        if message is None:
            message = (
                f"Update blog content - {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}"
            )

        success, stdout, stderr = self._run_git_command(["commit", "-m", message])
        if success:
            return True, f"提交成功: {message}"
        else:
            # 检查是否是"没有改动"的错误
            if "nothing to commit" in stderr or "nothing to commit" in stdout:
                return False, "没有需要提交的改动"
            return False, f"提交失败: {stderr}"

    def _remote_head(self, remote, branch):
        """尽力获取 remote-tracking 分支的 HEAD SHA；失败或不存在时返回空串。"""
        success, stdout, _ = self._run_git_command(
            ["rev-parse", f"{remote}/{branch}"], check=False
        )
        if not success:
            return ""
        return stdout.strip()

    def _head_subject(self):
        """尽力获取当前 HEAD 提交的 subject（push 不改变 HEAD，失败时也有效）。"""
        success, stdout, _ = self._run_git_command(
            ["log", "-1", "--pretty=format:%s"], check=False
        )
        if not success:
            return ""
        return stdout.strip()

    def _count_commits(self, from_sha, to_sha):
        """尽力计算 from..to 之间的提交数；from_sha 为空（首次推送）或失败时返回 0。

        首次推送时 from_sha 缺失，无法界定本次推送范围；此前回退到
        `rev-list --count <to_sha>` 会返回 to_sha 可达的全部历史提交数，
        对用户有误导，故改为返回 0（UI 会在 commit_count 为 0 时隐藏该字段）。
        """
        if to_sha and from_sha:
            success, stdout, _ = self._run_git_command(
                ["rev-list", "--count", f"{from_sha}..{to_sha}"], check=False
            )
            if success and stdout.strip().isdigit():
                return int(stdout.strip())
        return 0

    def _record_push(self, **fields):
        """安全地记录一次推送；任何异常都被吞掉并记录日志，绝不影响推送结果。"""
        if self.database is None:
            return
        try:
            self.database.record_push(**fields)
        except Exception as e:  # noqa: BLE001 - 记录失败绝不能影响发布流程
            logger.error(f"记录推送历史失败: {e}")

    def push(self, remote="origin", branch=None, set_upstream=False):
        """
        推送到远程仓库，并在成功/失败时记录一条推送历史（当 database 已注入）。

        Args:
            remote: 远程仓库名称，默认为 origin
            branch: 分支名称，如果为 None 则使用当前分支
            set_upstream: 是否设置上游分支

        Returns:
            tuple: (success, message)
        """
        # 获取当前分支
        if branch is None:
            success, stdout, stderr = self._run_git_command(
                ["branch", "--show-current"]
            )
            if not success:
                return False, f"获取当前分支失败: {stderr}"
            branch = stdout.strip()
        # 推送前尽力捕获 remote-tracking HEAD（首次推送可能不存在）
        from_sha = self._remote_head(remote, branch)
        # HEAD 的提交摘要（push 不改变 HEAD，失败时也有效）
        commit_message = self._head_subject()

        # 构建 push 命令
        push_command = ["push"]
        if set_upstream:
            push_command.extend(["-u", remote, branch])
        else:
            push_command.extend([remote, branch])

        # 执行 push
        success, stdout, stderr = self._run_git_command(push_command)

        to_sha = self._remote_head(remote, branch) if success else ""
        commit_count = self._count_commits(from_sha, to_sha) if success else 0

        if success:
            message = f"推送成功到 {remote}/{branch}"
            self._record_push(
                remote=remote,
                branch=branch,
                from_sha=from_sha,
                to_sha=to_sha,
                commit_count=commit_count,
                commit_message=commit_message,
                success=True,
                message=message,
            )
            return True, message
        else:
            message = f"推送失败: {stderr}"
            self._record_push(
                remote=remote,
                branch=branch,
                from_sha=from_sha,
                to_sha=to_sha,
                commit_count=0,
                commit_message=commit_message,
                success=False,
                message=message,
            )
            return False, message

    def publish_system(self, commit_message=None):
        """
        完整的系统发布流程：add all -> commit -> push

        Args:
            commit_message: 提交消息

        Returns:
            dict: 包含发布结果的字典
                - success: 是否成功
                - steps: 各步骤的执行结果
                - message: 总体消息
        """
        if not self.is_git_repo():
            return {
                "success": False,
                "steps": {},
                "message": "当前目录不是有效的 git 仓库",
            }

        steps = {}

        # 1. 检查状态
        status = self.get_status()
        steps["check_status"] = status
        if not status["success"]:
            return {"success": False, "steps": steps, "message": status["message"]}

        if not status["has_changes"]:
            return {"success": False, "steps": steps, "message": "没有需要发布的改动"}

        # 2. 添加所有改动
        add_success, add_message = self.add_all()
        steps["add"] = {"success": add_success, "message": add_message}
        if not add_success:
            return {"success": False, "steps": steps, "message": add_message}

        # 3. 提交
        commit_success, commit_msg = self.commit(commit_message)
        steps["commit"] = {"success": commit_success, "message": commit_msg}
        if not commit_success:
            return {"success": False, "steps": steps, "message": commit_msg}

        # 4. 推送
        push_success, push_message = self.push()
        steps["push"] = {"success": push_success, "message": push_message}
        if not push_success:
            return {
                "success": False,
                "steps": steps,
                "message": f"推送失败: {push_message}",
            }

        return {
            "success": True,
            "steps": steps,
            "message": "系统发布成功，GitHub Actions 将自动构建站点",
        }

    def get_recent_commits(self, count=10):
        """
        获取最近的提交记录（含 refs 与 diffstat，单次 git log 调用）。

        每条提交在原有字段基础上新增：
        - refs: 指向该提交的分支/标签（%d，已去除首尾括号）
        - stats: {files, insertions, deletions}（来自 --numstat）

        Args:
            count: 获取的提交数量，会被钳制到 [1, 50]
        """
        count = max(1, min(int(count or 10), 50))

        if not self.is_git_repo():
            return {
                "success": False,
                "commits": [],
                "message": "当前目录不是有效的 git 仓库",
            }

        # 单次 git log 同时输出提交信息与每文件增删（--numstat），
        # 提交行与 numstat 行之间用空行分隔。
        success, stdout, stderr = self._run_git_command(
            [
                "log",
                f"-{count}",
                "--pretty=format:%H|%an|%ae|%ad|%d|%s",
                "--date=iso",
                "--numstat",
            ]
        )

        if not success:
            return {
                "success": False,
                "commits": [],
                "message": f"获取提交记录失败: {stderr}",
            }

        commits = []
        current = None

        for line in stdout.split("\n"):
            stripped = line.strip()

            # numstat 行格式: <insertions>\t<deletions>\t<path>，二进制文件为 -\t-\t<path>
            if current is not None and "\t" in stripped and not stripped.endswith("|"):
                parts = stripped.split("\t")
                if len(parts) >= 3:
                    ins_raw, del_raw = parts[0], parts[1]
                    stats = current["stats"]
                    stats["files"] += 1
                    if ins_raw.lstrip("-").isdigit():
                        stats["insertions"] += int(ins_raw)
                    if del_raw.lstrip("-").isdigit():
                        stats["deletions"] += int(del_raw)
                    continue

            # 提交行: %H|%an|%ae|%ad|%d|%s （message 可能包含 |，故只在前 5 个分隔）
            if "|" in stripped and not stripped.startswith("\t"):
                fields = stripped.split("|", 5)
                if len(fields) >= 6:
                    # 收尾上一条提交
                    if current is not None:
                        commits.append(current)
                    refs_raw = fields[4].strip()
                    # %d 形如 " (HEAD -> main, tag: v1.0)" -> 去掉首尾括号与空格
                    refs = refs_raw.strip().strip("()")
                    current = {
                        "hash": fields[0],
                        "author": fields[1],
                        "email": fields[2],
                        "date": fields[3],
                        "refs": refs,
                        "message": fields[5],
                        "stats": {
                            "files": 0,
                            "insertions": 0,
                            "deletions": 0,
                        },
                    }
                    continue

        if current is not None:
            commits.append(current)

        return {
            "success": True,
            "commits": commits,
            "message": f"成功获取 {len(commits)} 条提交记录",
        }
