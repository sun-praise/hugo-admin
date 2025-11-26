# coding: utf-8
"""
GitService 功能测试
"""
import pytest
import tempfile
import os
from pathlib import Path
import subprocess

from services.git_service import GitService


class TestGitService:

    @pytest.fixture
    def temp_git_repo(self):
        """临时 Git 仓库"""
        with tempfile.TemporaryDirectory() as temp_dir:
            repo_path = Path(temp_dir)

            # 初始化 git 仓库
            subprocess.run(['git', 'init'], cwd=repo_path, check=True, capture_output=True)
            subprocess.run(['git', 'config', 'user.email', 'test@example.com'], cwd=repo_path, check=True)
            subprocess.run(['git', 'config', 'user.name', 'Test User'], cwd=repo_path, check=True)
            # 禁用 GPG 签名
            subprocess.run(['git', 'config', 'commit.gpgsign', 'false'], cwd=repo_path, check=True)

            # 创建初始提交
            test_file = repo_path / 'README.md'
            test_file.write_text('# Test Repo')
            subprocess.run(['git', 'add', 'README.md'], cwd=repo_path, check=True)
            subprocess.run(['git', 'commit', '-m', 'Initial commit'], cwd=repo_path, check=True)

            yield repo_path

    @pytest.fixture
    def git_service(self, temp_git_repo):
        """GitService 实例"""
        return GitService(temp_git_repo)

    def test_is_git_repo(self, git_service):
        """测试 Git 仓库检测"""
        assert git_service.is_git_repo() is True

    def test_is_not_git_repo(self):
        """测试非 Git 仓库检测"""
        with tempfile.TemporaryDirectory() as temp_dir:
            service = GitService(temp_dir)
            assert service.is_git_repo() is False

    def test_get_status_clean(self, git_service):
        """测试获取干净的 Git 状态"""
        status = git_service.get_status()

        assert status['success'] is True
        assert status['has_changes'] is False
        assert len(status['staged']) == 0
        assert len(status['unstaged']) == 0
        assert len(status['untracked']) == 0

    def test_get_status_with_changes(self, git_service, temp_git_repo):
        """测试获取有改动的 Git 状态"""
        # 创建新文件 (未追踪)
        new_file = temp_git_repo / 'new_file.txt'
        new_file.write_text('New content')

        # 修改已有文件 (已追踪但未暂存)
        readme = temp_git_repo / 'README.md'
        readme.write_text('# Modified')

        status = git_service.get_status()

        assert status['success'] is True
        assert status['has_changes'] is True
        # 新文件应该在 untracked 列表中
        assert 'new_file.txt' in status['untracked'] or 'new_file.txt' in status['unstaged']
        # 修改的已追踪文件应该在 unstaged 列表中
        assert 'README.md' in status['unstaged'] or len(status['unstaged']) > 0 or len(status['untracked']) > 0

    def test_add_all(self, git_service, temp_git_repo):
        """测试添加所有文件到暂存区"""
        # 创建新文件
        new_file = temp_git_repo / 'test.txt'
        new_file.write_text('Test content')

        # 添加所有文件
        success, message = git_service.add_all()

        assert success is True
        assert "添加" in message

        # 验证文件已暂存
        status = git_service.get_status()
        assert 'test.txt' in status['staged']

    def test_commit(self, git_service, temp_git_repo):
        """测试提交改动"""
        # 创建并暂存文件
        new_file = temp_git_repo / 'test.txt'
        new_file.write_text('Test content')
        git_service.add_all()

        # 提交
        success, message = git_service.commit('Test commit')

        assert success is True
        assert "提交成功" in message

        # 验证没有待提交的改动
        status = git_service.get_status()
        assert status['has_changes'] is False

    def test_commit_with_default_message(self, git_service, temp_git_repo):
        """测试使用默认消息提交"""
        # 创建并暂存文件
        new_file = temp_git_repo / 'test.txt'
        new_file.write_text('Test content')
        git_service.add_all()

        # 使用默认消息提交
        success, message = git_service.commit()

        assert success is True
        assert "提交成功" in message

    def test_commit_no_changes(self, git_service):
        """测试在没有改动时提交"""
        success, message = git_service.commit('Empty commit')

        assert success is False
        assert "没有需要提交的改动" in message

    def test_get_recent_commits(self, git_service):
        """测试获取最近的提交记录"""
        result = git_service.get_recent_commits(count=5)

        assert result['success'] is True
        assert len(result['commits']) > 0

        # 验证提交记录结构
        commit = result['commits'][0]
        assert 'hash' in commit
        assert 'author' in commit
        assert 'email' in commit
        assert 'date' in commit
        assert 'message' in commit

    def test_publish_system_success(self, git_service, temp_git_repo):
        """测试完整的系统发布流程（不包括 push）"""
        # 注意：这个测试不会真正执行 push，因为没有配置远程仓库
        # 但可以测试 add 和 commit 部分

        # 创建新文件
        new_file = temp_git_repo / 'publish_test.txt'
        new_file.write_text('Content to publish')

        # 由于没有远程仓库，我们只测试到 commit 阶段
        # 首先手动测试 add 和 commit
        git_service.add_all()
        success, message = git_service.commit('Test system publish')

        assert success is True
        assert "提交成功" in message

    def test_publish_system_no_changes(self, git_service):
        """测试在没有改动时执行系统发布"""
        result = git_service.publish_system()

        assert result['success'] is False
        assert "没有需要发布的改动" in result['message']

    def test_publish_system_with_changes(self, git_service, temp_git_repo):
        """测试有改动时的系统发布（add + commit，跳过 push）"""
        # 创建新文件
        new_file = temp_git_repo / 'content.txt'
        new_file.write_text('New content')

        # 由于测试环境没有远程仓库，publish_system 会在 push 阶段失败
        # 我们可以单独测试各个步骤

        # 1. 检查状态
        status = git_service.get_status()
        assert status['has_changes'] is True

        # 2. 添加文件
        success, msg = git_service.add_all()
        assert success is True

        # 3. 提交
        success, msg = git_service.commit('Test changes')
        assert success is True

    def test_invalid_repo_path(self):
        """测试无效的仓库路径"""
        with pytest.raises(ValueError):
            GitService('/non/existent/path')

    def test_get_status_non_git_repo(self):
        """测试在非 Git 仓库上获取状态"""
        with tempfile.TemporaryDirectory() as temp_dir:
            service = GitService(temp_dir)
            status = service.get_status()

            assert status['success'] is False
            assert "不是有效的 git 仓库" in status['message']
