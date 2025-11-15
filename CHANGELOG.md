# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- 新增文档站点配置 (mkdocs.yml) 支持 MkDocs 文档框架
- 在 docs 目录中创建站点首页 (index.md)

### Changed
- 将技术文档从根目录移动到 docs 目录进行集中管理
  - 移动了7个技术文档文件到 docs 目录
  - 保持了文档的完整性和可访问性
- 优化了文档管理结构，便于维护和导航

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
