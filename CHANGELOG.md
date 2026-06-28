# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- 编辑器预览支持渲染 mermaid 图表：`renderMarkdown` 新增 opt-in `{ mermaid }` 选项，开启时在 marked 解析前抽出 ` ```mermaid ` 围栏块还原为 `.mermaid` 容器；Editor 预览懒加载 mermaid（dynamic import，按需加载，失败可重试）并 `api.run()` 渲染，输入变更 200ms debounce 后重渲。`securityLevel: strict`，不绕过 DOMPurify。AIChat 等未开启的调用方行为不变。

## [2.5.1] - 2026-06-28

### Fixed
- Dockerfile 漏掉前端构建：镜像内 `static/dist/` 是仓库历史残留的旧前端，导致 RequireAuth 路由守卫未生效，未登录也能看到管理界面空壳（后端 API 鉴权始终正常，未登录无法读写任何数据）。改为 multi-stage：Stage 1 `node:22-slim` 跑 `pnpm build`，Stage 2 把产物覆盖到 `/app/static/dist`。
- HEALTHCHECK 端点从 `/api/server/status`（需登录，未认证 401 导致容器永远 starting）改为 `/api/version`（免认证）。

## [2.5.0] - 2026-06-28

### Fixed
- Python 3.10 下 `PUT /api/config` 返回 400：`tomllib` 在 3.11 才进标准库，回退到 `tomli`/`tomllib` 可选导入以兼容 3.10。
- 对齐版本号：此前 `__version__.py` 停留在 2.3.0，本次追平到 2.5.0，`/api/version` 不再误报。

## [2.4.0] - 2026-06-26

### Added
- Hugo 站点配置编辑器：Settings 新增「站点配置」tab，左侧文件树列出 `config/_default/` 下所有配置文件，右侧 PrismJS TOML/YAML/JSON 语法高亮编辑器，字号可选（11–20px，localStorage 持久化），按文件单独保存。后端 `GET/PUT /api/config`（列表）与 `GET/PUT /api/config/<filename>`（按文件读写），自动检测根目录单文件 vs `config/_default/` 多文件结构，TOML/YAML/JSON 语法校验。
- Hugo 项目初始化：`/api/project/init` 一键创建 Hugo 站点，自动安装默认主题（svtter/Fried-Rice）并设为活跃项目；支持 TOML/YAML 配置格式选择。
- 持久化活跃项目路径：`ActiveProjectRegistry` 把活跃 Hugo 项目写入 `data/active_project.txt`，进程重启后仍恢复；`POST /api/project/active/reset` 清除持久化回到默认 HUGO_ROOT。
- 主题管理：`themes/` 目录扫描、子模块/复制两种安装模式、激活/预览主题；`GET/POST /api/themes*` 系列接口；主题装上后自动清理占位 `layouts/` 让主题接管渲染（含 `POST /api/project/clean-layouts` 修复旧站点）。
- Git 管理页面（`/git`）：查看最近的提交记录（含 refs 与 diffstat）与本 admin 触发的推送历史（remote/branch、commit range、成功/失败与时间）。
- 推送历史持久化到 per-repo `content/.admin/cache.db`（新增 `git_push_history` 表）；`GitService.push()` 在成功/失败时各记录一条（best-effort 捕获 from/to SHA 与 commit_count，HEAD 摘要作为 commit_message）；hugo-root 切换时为新 content 目录重建 `Database`。
- `GET /api/git/commits` 增强：单次 `git log --numstat` 调用即返回 `refs` 与 `stats`（files/insertions/deletions），`count` 钳制到 `[1, 50]`；旧字段与响应结构不变。
- 新增 `GET /api/git/pushes`（分页、倒序，参数钳制）。
- 密码登录与 API 会话守卫：`AuthService`（`data/auth.json` 持久化，werkzeug 哈希）+ `/api/auth/login|logout|me` + 全局 `install_auth_guard` 拦截未登录 `/api/*`（白名单 login/me/version）。
- 设置页改用 tab 组织（常规设置 / 主题管理 / 初始化项目 / 站点配置）。

### Fixed
- `[outputs]` 等 TOML table header 在配置编辑器里被拆成三行：Prism 的 `token table` class 与 Tailwind `.table` 工具类撞名，用 scoped `.config-editor .token { display: inline !important }` 覆盖。
- 主题装上后删除占位 `layouts/`，避免覆盖主题模板（旧版本 init 创建的「毛坯」站点可用 `/clean-layouts` 接口修复）。
- 新站点预览与文章缓存隔离，避免切换活跃项目后串数据。
- 初始化项目后立即生成 `.admin/settings.json`，新初始化站点缺少 `content/post` 时不再创建文章失败。
- 前端 eslint errors/warnings（PR #119）。
- Image paste 不再因文件名碰撞覆盖旧上传（#70）；图片上传增加 20MB 体积限制与扩展名白名单。

### Changed
- `routes/` 模块化：新增 `config_routes`、`auth_routes`、`theme_routes`、`project_init_routes`、`publish_routes` 注册函数，统一通过 `routes/__init__.py` 导出。
- `services/registry.py` 间接访问 post_service/git_service 等实例，settings 更新重建后 Blueprint 自动用新实例。
## [2.3.0] - 2026-06-16

### Added
- Inline AI edit (rewrite) in the editor: select text → floating ✨ trigger → instruction input + 4 Chinese presets (润色 / 翻译为英文 / 精简 / 详细化) → non-streaming LLM call → original-vs-revised side-by-side diff → accept replaces the selection. Backed by a new `POST /api/ai/inline-edit` endpoint and `AIService.quick_rewrite` (Claude Agent SDK with tools disabled, 10s timeout).
- Tests for the new endpoint and the `quick_rewrite` service method.

### Fixed
- Inline AI edit: cancel the pending selection-debounce timer when the popup closes, so a selection event registered while the popup was open can no longer re-arm the ✨ trigger against a stale selection after close (the "first invocation leaks into the second" residue).
- Prevent image paste from overwriting previous uploads on filename collision (#70)
- Add file size limit (20 MB) and extension whitelist for image uploads

## [2.2.0] - 2026-06-11

### Added
- Plugin system implementation (Phase 1 + Phase 2): gRPC-based extension architecture
- Plugin management UI page
- Editor image upload wired to active image plugin
- Listmonk configuration to settings page with auto-migration from `~/.config/secret.yml`
- AI assistant awareness of current page and article context
- 80 unit tests for plugin system

### Changed
- Split README into English and Chinese versions
- Add Docker installation instructions to README
- Posts pagination: wire page param and add pagination controls

### Fixed
- Extract `tool_result` text content in backend and show preview in summary
- Fix SSE chunk boundary causing `tool_result` JSON to be rendered as text
- Fix undefined `session_id` when extracting `tool_result`
- Wire `plugin_manager` into `ServiceRegistry`
- Show article title (instead of static text) on editor page
- Docker deployment: mount hugo-blog dirs, support env-var paths
- Address PR review suggestions for docker-deploy

## [2.1.1] - 2026-06-07

### Added
- Plugin system proposal (gRPC-based extension architecture) (#92)
- Frontend build check to CI workflow
- Docker image push to GHCR on main/tags
- ServiceRegistry for consistent service access across Blueprints

### Changed
- Modularize app.py routes into Flask Blueprints (#84)
- Use pnpm instead of npm in run.sh for consistency with CI

### Fixed
- Email article URL matching with normalized URL comparison (#88)
- Stale closure in Ctrl+S keyboard handler
- Stale closure in handlePaste with functional setState
- Blank-line accumulation between frontmatter and body
- Move hasChanges useMemo after frontmatter declarations

## [2.1.0] - 2026-06-04

### Added
- AI-generated cover image for articles (#54)
- AI-generated frontmatter from article content
- Collapsible sidebar with icon-only mode
- Progress bar for cover generation, image field support, and cover preview in drawer

### Changed
- Compact toolbar to icon-only buttons with tooltips
- Switch CI review to multi-review@v3 action

### Fixed
- Parse OpenRouter image from message.images field
- Raise frontmatter and backlinks drawer z-index above AI chat button
- Don't overwrite title/draft in AI frontmatter gen
- Review feedback and in-memory cache for frontmatter gen
- Address PR #65 review feedback

## [2.0.0] - 2026-06-03

### Added
- Migrate frontend from Jinja2 templates to React + Vite SPA
- Apply Warm Editorial design system
- Add SPA route tests for React migration
- Implement bidirectional article links (#32)
- Add frontmatter drawer editor with side panel UI
- Add datetime-local picker for frontmatter date fields
- Add Hugo server URL configuration in settings page
- Add post list refresh cache button
- Display local and LAN IP addresses on startup
- Add Python logging configuration with app.log

### Changed
- Switch CI review model from glm-5.1 to deepseek-v4-flash
- Remove obsolete Jinja2 templates
- Align PostsResponse type with backend API format
- Expand backlink search scope with popup search UI
- Normalize ref target_path to strip leading /
- Replace print with logger throughout codebase
- Hugo config validation now supports config/_default/ directory structure

### Fixed
- Give textarea a white background to match the preview pane
- Show 已发布 for published articles (API field mismatch: is_draft vs is_published)
- Strip frontmatter robustly on read/save to prevent duplication
- Disable publish button when article is already published
- Auto-append Hugo port when server_url omits it
- Replace deprecated bg-opacity-* with Tailwind v4 slash syntax
- Fix Hugo server baseURL hardcoding for Docker environments (#37)
- Set HUGO_SERVER_BASE_URL default to 0.0.0.0 for container accessibility
- Prevent Ctrl+V paste image duplication
- Fix frontmatter date serialization causing Hugo parse failures
- Fix frontmatter drawer z-index to sit above AI assistant
- Fix Hugo shortcode syntax conflicting with Jinja2 template parsing
- Fix cover image not displaying due to stale cache
- Fix path inconsistency in cache_service incremental updates

## [1.4.1] - 2026-04-29

### Fixed
- Persist cache across restarts (#34)
- Resolve path inconsistency and improve incremental update in cache_service
- Remove `.resolve()` from invalidate_post for path consistency
- Extract POST_SUBDIR constant, batch DB query, replace print with logger, add edge-case tests

### Known Issues
- #36 上传图片后文章列表不显示封面图

## [1.4.0] - 2026-04-15

### Added
- Cover image display in post list and dashboard recent posts (#26, #27)
- GitHub icon link in sidebar footer (#29)
- Docker image build workflow for GitHub Actions
- Auto-read version from `__version__.py` in sidebar (#31)

### Changed
- AI assistant is now read-only with current article context injected (#28)
- Docker image tag updated to `svtter/hugo-admin`

## [1.3.0] - 2026-04-07

### Added
- Added a dedicated settings page with Hugo base directory configuration.
- Added AI chat session management and history support for chat continuity.
- Added REST endpoints and UI for managing chat sessions.

### Changed
- Reworked settings interactions from popup-based UI to a dedicated settings page.
- Moved AI configuration from environment-only settings into the sidebar/settings page.
- Integrated pre-commit checks into CI and applied automated style fixes.

### Fixed
- Fixed a settings service variable scope issue that could raise `UnboundLocalError`.
- Fixed settings sidebar interactions and Docker/CI reliability issues.
- Improved config storage and validation robustness.

## [1.2.1] - 2026-01-24

### Added
- Email push notification integration (#9)
- Migrated from PydanticAI to Claude Agent SDK with DeepSeek support

### Changed
- Migrated dependencies to pyproject.toml for uv sync compatibility

## [1.2.0] - 2026-01-10

### Added
- Clipboard image paste functionality in editor
- OpenSpec support for project planning and changes
- AI chat assistant with tool calling capabilities

### Fixed
- AI tool calls streaming response handling
- SSE stream termination and loading state resets
- Standardized date format to RFC3339 with timezone
- Socket.IO port and server log display issues

## [1.1.1] - 2025-11-26

### Fixed
- Added missing 404.html and 500.html error page templates for better error handling

## [1.1.0] - 2025-11-26

### Added
- System publish feature for git commit and push functionality
- Visual status indicators for post publication state
- Bulk publishing operations for managing multiple posts
- Markdown preview functionality in the editor
- Documentation site configuration with MkDocs

### Changed
- Optimized preview functionality
- Improved Socket.IO connection port handling
- Enhanced posts.html bulk operation toolbar layout

### Fixed
- Fixed tags and categories displaying as 0 in the dashboard
- Fixed Socket.IO connection port issues

## [1.0.1] - 2025-11-26

### Added
- 新增文档站点配置 (mkdocs.yml) 支持 MkDocs 文档框架
- 在 docs 目录中创建站点首页 (index.md)
- 新增发布文章功能文档
- 新增版本跟踪文件 (__version__.py)

### Changed
- 将技术文档从根目录移动到 docs 目录进行集中管理
  - 移动了7个技术文档文件到 docs 目录
  - 保持了文档的完整性和可访问性
- 优化了文档管理结构，便于维护和导航
- 优化预览功能相关修改

## [1.0.0] - 2025-11-05

### Added
- Initial release of Hugo Blog Web Management Interface
- Dashboard with blog statistics (total posts, tags, categories)
- Post management with search, filter, and pagination
- Online Markdown editor with auto-save detection
- Hugo server control panel with real-time logs via WebSocket
- File browser and editor with path security checks
- Create new posts with interactive form
- Real-time server status monitoring
- Responsive UI with Tailwind CSS and Alpine.js
- Support for both `.md` files and `.md/index.md` directory structures

### Changed
- **[BREAKING]** Refactored frontmatter parsing to use `python-frontmatter` library
  - Replaced manual YAML parsing with professional library
  - All field types are now guaranteed (no more `None` values)
  - Reduced code complexity by 40% (~70 lines → ~30 lines)
- Improved path resolution to support both absolute and relative paths
- Enhanced date field handling (unified to string format: `YYYY-MM-DD HH:MM:SS`)
- Simplified tag/category handling (always returns list, never `None`)

### Fixed
- Fixed article count discrepancy (629 → 550 articles)
  - Issue: `rglob("*.md")` matched both files and directories
  - Solution: Added `is_dir()` check to skip `.md` directories
- Fixed navigation sidebar alignment issues
  - Issue: Tailwind CSS `@apply` directive not working in CDN version
  - Solution: Converted to standard CSS with `display: flex` and `align-items: center`
- Fixed Flask-SocketIO compatibility issue
  - Issue: New version requires explicit `allow_unsafe_werkzeug=True`
  - Solution: Added parameter to `socketio.run()`
- Fixed date field type inconsistency
  - Issue: Some posts had `datetime` objects, others had strings
  - Solution: Unified to string format in `_get_date_field()` helper
- Fixed `None` value errors in tags/categories iteration
  - Issue: Some posts had `None` instead of empty lists
  - Solution: Added `_get_list_field()` helper to guarantee list type
- Fixed relative path calculation for absolute file paths
  - Issue: `relative_to()` failed when mixing absolute and relative paths
  - Solution: Added fallback logic to extract path from string

### Dependencies
- Flask 3.0.0
- flask-socketio 5.3.5
- psutil 5.9.6
- PyYAML 6.0.1
- **python-frontmatter 1.1.0** (new)

### Technical Details

#### BlogPost Class Improvements
```python
# New helper methods for type safety
_get_string_field()   # Ensures string output
_get_date_field()     # Converts datetime to string
_get_list_field()     # Ensures list output, handles string/None
```

#### Type Guarantees
After parsing, all BlogPost instances guarantee:
- `title`: `str` (empty string if missing)
- `description`: `str` (empty string if missing)
- `date`: `str` in format `YYYY-MM-DD HH:MM:SS`
- `tags`: `list[str]` (empty list if missing, never `None`)
- `categories`: `list[str]` (empty list if missing, never `None`)
- `content`: `str` (Markdown content)
- `excerpt`: `str` (first 100 characters)

#### Statistics
- Successfully parses 550 blog posts
- Indexes 328 unique tags
- Indexes 87 unique categories
- Supports mixed content structure:
  - 419 posts as direct `.md` files
  - 131 posts as `.md/index.md` directories

### Security
- Server binds to `127.0.0.1` by default (localhost only)
- File operations restricted to `content/` directory
- Path traversal protection with `_is_safe_path()` validation
- No authentication required (designed for personal use)

### Documentation
- README.md - Project overview and features
- QUICKSTART.md - Quick start guide
- FIX_SUMMARY.md - Detailed bug fixes
- FRONTMATTER_REFACTOR.md - Refactoring details
- TEST_REPORT.md - Comprehensive test results

### Known Limitations
- Single user only (no authentication system)
- No real-time collaboration support
- No Markdown preview (planned for future release)
- No image upload management (planned for future release)

---

## Version History

### [1.0.0] - 2025-11-05
First stable release with core features:
- Article management (browse, search, edit)
- Hugo server control
- Real-time WebSocket logs
- Type-safe frontmatter parsing

---

## Migration Guide

### Upgrading from Development Version

If you were using the development version before 1.0.0:

1. **Install new dependency:**
   ```bash
   pip install python-frontmatter==1.1.0
   ```

2. **No data migration needed:**
   - Existing blog posts work without changes
   - The new parser is backward compatible

3. **API compatibility:**
   - All existing API endpoints remain unchanged
   - BlogPost class interface is backward compatible
   - Only internal implementation changed

### Breaking Changes

None for external users. The API surface remains the same.

Internal changes (if you extended the code):
- `BlogPost._parse_file()` method signature changed
- Manual YAML parsing removed in favor of `python-frontmatter`
- Added three new helper methods (see Technical Details above)

---

## Credits

- **Framework:** Flask, Flask-SocketIO
- **Frontend:** Alpine.js, Tailwind CSS
- **Parsing:** python-frontmatter
- **Process Management:** psutil

---

## Links

- [Repository](https://github.com/your-repo/hugo-blog-admin)
- [Issue Tracker](https://github.com/your-repo/hugo-blog-admin/issues)
- [Hugo Documentation](https://gohugo.io/documentation/)
- [python-frontmatter](https://github.com/eyeseast/python-frontmatter)

---

## Footnotes

[Keep a Changelog]: https://keepachangelog.com/en/1.0.0/
[Semantic Versioning]: https://semver.org/spec/v2.0.0.html
