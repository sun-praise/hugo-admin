# Hugo Admin

[![Tests](https://github.com/Svtter/hugo-admin/workflows/Tests/badge.svg)](https://github.com/Svtter/hugo-admin/actions)
[![License](https://img.shields.io/github/license/Svtter/hugo-admin)](https://github.com/Svtter/hugo-admin/blob/main/LICENSE)
[![Python](https://img.shields.io/badge/python-3.9%2B-blue)](https://www.python.org/downloads/)
[![Hugo](https://img.shields.io/badge/hugo-compatible-ff4088)](https://gohugo.io/)
[![Code style: black](https://img.shields.io/badge/code%20style-black-000000.svg)](https://github.com/psf/black)

[English](#english) | [中文](#chinese)

<a name="english"></a>
## English

A lightweight web-based admin interface for managing Hugo static sites. Provides an intuitive GUI for browsing, searching, editing posts, and controlling the Hugo development server.

### Features

- **📊 Dashboard**: Overview of blog statistics and quick actions
- **📝 Post Management**: Browse, search, and filter posts by category and tags
- **✏️ Markdown Editor**: Online editing with auto-save and keyboard shortcuts
- **🚀 Hugo Server Control**: Start/stop Hugo dev server with real-time logs
- **🔍 Advanced Search**: Full-text search with category and tag filtering
- **⚡ Real-time Updates**: WebSocket-based live log streaming
- **💾 Cache System**: SQLite-based caching for fast post retrieval

### Tech Stack

- **Backend**: Flask + Flask-SocketIO
- **Frontend**: Tailwind CSS + Alpine.js
- **Real-time Communication**: WebSocket (Socket.IO)
- **Process Management**: psutil
- **Database**: SQLite (for caching)

### Installation

#### Requirements

- Python 3.9+
- Hugo (installed and in PATH)

#### Steps

1. Clone the repository:
```bash
git clone https://github.com/Svtter/hugo-admin.git
cd hugo-admin
```

2. Install dependencies:
```bash
pip install -r requirements.txt
```

3. Configure the application:
```bash
cp config.py config_local.py
# Edit config_local.py to set your Hugo root directory
```

4. Run the application:
```bash
python app.py
```

5. Open your browser and navigate to `http://127.0.0.1:5000`

### Configuration

Edit `config.py` or create `config_local.py` to customize:

```python
# Hugo root directory (parent of content/)
HUGO_ROOT = '/path/to/your/hugo/site'

# Content directory
CONTENT_DIR = HUGO_ROOT + '/content'

# Hugo server settings
HUGO_SERVER_PORT = 1313
HUGO_SERVER_HOST = '127.0.0.1'
```

### Usage

#### Dashboard
- View blog statistics (post count, tags, categories)
- Check Hugo server status
- Quick access to common operations
- Recent posts overview

#### Post Management
- Browse all posts with pagination
- Search posts by title, content, tags, or categories
- Filter by specific category or tag
- Click any post to edit

#### Editor
- Edit Markdown files with syntax highlighting
- Auto-save on changes
- Keyboard shortcut: `Ctrl+S` / `Cmd+S` to save
- Real-time save status indicator

#### Server Control
- Start Hugo server (with or without drafts)
- Stop running server
- View server status (PID, uptime, CPU, memory)
- Real-time log streaming

### Development

#### Running Tests

```bash
# Install dev dependencies
pip install -r requirements-dev.txt

# Run all tests
pytest

# Run with coverage
pytest --cov=. --cov-report=html
```

#### Project Structure

```
hugo-admin/
├── app.py                 # Flask application
├── config.py              # Configuration
├── requirements.txt       # Dependencies
├── requirements-dev.txt   # Dev dependencies
├── pytest.ini             # Pytest configuration
├── services/              # Business logic
│   ├── hugo_service.py    # Hugo server management
│   ├── post_service.py    # Post operations
│   └── cache_service.py   # Caching layer
├── models/                # Database models
│   └── database.py        # SQLite operations
├── templates/             # Jinja2 templates
├── static/                # CSS, JS, images
└── tests/                 # Test suite
```

### Contributing

Contributions are welcome! Please feel free to submit a Pull Request. For major changes, please open an issue first to discuss what you would like to change.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/AmazingFeature`)
3. Commit your changes (`git commit -m 'Add some AmazingFeature'`)
4. Push to the branch (`git push origin feature/AmazingFeature`)
5. Open a Pull Request

### Security

- Application binds to `127.0.0.1` by default (localhost only)
- File operations are restricted to the `content` directory
- Path traversal protection included
- Not recommended for production use on public networks without additional security measures

### Roadmap

- [x] Basic framework
- [x] Hugo server control
- [x] Post browsing and search
- [x] Markdown editor
- [x] Markdown preview
- [x] SQLite caching system
- [x] Test suite with CI/CD
- [x] Image upload and management
- [ ] Git operations interface
- [ ] Batch operations
- [ ] Multi-user support
- [ ] Docker support

### License

MIT License - see [LICENSE](LICENSE) file for details

### Acknowledgments

Built with ❤️ for the Hugo community.

---

<a name="chinese"></a>
## 中文

一个轻量级的 Hugo 静态网站管理界面。提供直观的 GUI 用于浏览、搜索、编辑文章和控制 Hugo 开发服务器。

### 功能特性

- **📊 仪表板**: 博客统计信息和快速操作概览
- **📝 文章管理**: 按分类和标签浏览、搜索和筛选文章
- **✏️ Markdown 编辑器**: 在线编辑，支持自动保存和快捷键
- **🚀 Hugo 服务器控制**: 启动/停止 Hugo 开发服务器，实时日志
- **🔍 高级搜索**: 全文搜索，支持分类和标签过滤
- **⚡ 实时更新**: 基于 WebSocket 的实时日志流
- **💾 缓存系统**: 基于 SQLite 的缓存，快速检索文章

### 技术栈

- **后端**: Flask + Flask-SocketIO
- **前端**: Tailwind CSS + Alpine.js
- **实时通信**: WebSocket (Socket.IO)
- **进程管理**: psutil
- **数据库**: SQLite (用于缓存)

### 安装

#### 环境要求

- Python 3.9+
- Hugo (已安装并在 PATH 中)

#### 安装步骤

1. 克隆仓库:
```bash
git clone https://github.com/Svtter/hugo-admin.git
cd hugo-admin
```

2. 安装依赖:
```bash
pip install -r requirements.txt
```

3. 配置应用:
```bash
cp config.py config_local.py
# 编辑 config_local.py 设置你的 Hugo 根目录
```

4. 运行应用:
```bash
python app.py
```

5. 在浏览器中打开 `http://127.0.0.1:5000`

### 配置

编辑 `config.py` 或创建 `config_local.py` 进行自定义:

```python
# Hugo 根目录 (content/ 的父目录)
HUGO_ROOT = '/path/to/your/hugo/site'

# 内容目录
CONTENT_DIR = HUGO_ROOT + '/content'

# Hugo 服务器设置
HUGO_SERVER_PORT = 1313
HUGO_SERVER_HOST = '127.0.0.1'
```

### 使用说明

#### 仪表板
- 查看博客统计信息（文章数、标签数、分类数）
- 检查 Hugo 服务器状态
- 快速访问常用操作
- 最近文章概览

#### 文章管理
- 分页浏览所有文章
- 按标题、内容、标签或分类搜索文章
- 按特定分类或标签筛选
- 点击任意文章进入编辑

#### 编辑器
- 编辑 Markdown 文件，支持语法高亮
- 自动保存更改
- 键盘快捷键：`Ctrl+S` / `Cmd+S` 保存
- 实时保存状态指示器

#### 服务器控制
- 启动 Hugo 服务器（支持草稿模式）
- 停止运行中的服务器
- 查看服务器状态（PID、运行时间、CPU、内存）
- 实时日志流

### 开发

#### 运行测试

```bash
# 安装开发依赖
pip install -r requirements-dev.txt

# 运行所有测试
pytest

# 运行测试并生成覆盖率报告
pytest --cov=. --cov-report=html
```

#### 项目结构

```
hugo-admin/
├── app.py                 # Flask 应用
├── config.py              # 配置文件
├── requirements.txt       # 依赖
├── requirements-dev.txt   # 开发依赖
├── pytest.ini             # Pytest 配置
├── services/              # 业务逻辑层
│   ├── hugo_service.py    # Hugo 服务器管理
│   ├── post_service.py    # 文章操作
│   └── cache_service.py   # 缓存层
├── models/                # 数据库模型
│   └── database.py        # SQLite 操作
├── templates/             # Jinja2 模板
├── static/                # CSS、JS、图片
└── tests/                 # 测试套件
```

### 贡献

欢迎贡献！请随时提交 Pull Request。对于重大更改，请先开 issue 讨论您想要更改的内容。

1. Fork 本仓库
2. 创建特性分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 开启 Pull Request

### 安全性

- 应用默认绑定到 `127.0.0.1`（仅本地访问）
- 文件操作限制在 `content` 目录内
- 包含路径遍历保护
- 不建议在公共网络上生产使用，除非采取额外的安全措施

### 开发路线图

- [x] 基础框架
- [x] Hugo 服务器控制
- [x] 文章浏览和搜索
- [x] Markdown 编辑器
- [x] Markdown 预览
- [x] SQLite 缓存系统
- [x] 测试套件与 CI/CD
- [x] 图片上传和管理
- [ ] Git 操作界面
- [ ] 批量操作
- [ ] 多用户支持
- [ ] Docker 支持

### 许可证

MIT License - 详见 [LICENSE](LICENSE) 文件

### 致谢

为 Hugo 社区用 ❤️ 构建。
