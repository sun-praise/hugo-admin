<!-- OPENSPEC:START -->
# OpenSpec Instructions

These instructions are for AI assistants working in this project.

Always open `@/openspec/AGENTS.md` when the request:
- Mentions planning or proposals (words like proposal, spec, change, plan)
- Introduces new capabilities, breaking changes, architecture shifts, or big performance/security work
- Sounds ambiguous and you need the authoritative spec before coding

Use `@/openspec/AGENTS.md` to learn:
- How to create and apply change proposals
- Spec format and conventions
- Project structure and guidelines

Keep this managed block so 'openspec update' can refresh the instructions.

<!-- OPENSPEC:END -->

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
