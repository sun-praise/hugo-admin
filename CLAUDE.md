IMPORTANT: 不要在当前目录和 main 分支修改代码，在 .worktrees 目录下创建 git worktree 来修改。

ops 仓库: ~/ops 记录了部署内容和部署记录。

# hugo-admin Development Guidelines

Auto-generated from all feature plans. Last updated: 2025-11-14

## Active Technologies
- Python 3.11+ + Flask==3.0.0, flask-socketio==5.3.5, PyYAML==6.0.1, python-frontmatter==1.1.0 (001-article-publish)
- File-based (Hugo markdown files with YAML frontmatter) (001-article-publish)

- Python 3.11+ (existing codebase) + Flask==3.0.0, flask-socketio==5.3.5, PyYAML==6.0.1, python-frontmatter==1.1.0 (001-article-publish)

## Project Structure

```text
src/
tests/
```

## Commands

cd src [ONLY COMMANDS FOR ACTIVE TECHNOLOGIES][ONLY COMMANDS FOR ACTIVE TECHNOLOGIES] pytest [ONLY COMMANDS FOR ACTIVE TECHNOLOGIES][ONLY COMMANDS FOR ACTIVE TECHNOLOGIES] ruff check .

## Code Style

Python 3.11+ (existing codebase): Follow standard conventions

## Recent Changes
- 001-article-publish: Added Python 3.11+ + Flask==3.0.0, flask-socketio==5.3.5, PyYAML==6.0.1, python-frontmatter==1.1.0

- 001-article-publish: Added Python 3.11+ (existing codebase) + Flask==3.0.0, flask-socketio==5.3.5, PyYAML==6.0.1, python-frontmatter==1.1.0

<!-- MANUAL ADDITIONS START -->
<!-- MANUAL ADDITIONS END -->
