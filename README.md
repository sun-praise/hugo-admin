# Hugo Admin

[![Tests](https://github.com/Svtter/hugo-admin/workflows/Tests/badge.svg)](https://github.com/Svtter/hugo-admin/actions)
[![License](https://img.shields.io/github/license/Svtter/hugo-admin)](https://github.com/Svtter/hugo-admin/blob/main/LICENSE)
[![Python](https://img.shields.io/badge/python-3.9%2B-blue)](https://www.python.org/downloads/)
[![Hugo](https://img.shields.io/badge/hugo-compatible-ff4088)](https://gohugo.io/)
[![Code style: black](https://img.shields.io/badge/code%20style-black-000000.svg)](https://github.com/psf/black)

[中文](README.zh-CN.md) | English

## Screenshot

![Hugo Admin Dashboard](screenshot.png)

## Features

- **📊 Dashboard**: Overview of blog statistics and quick actions
- **📝 Post Management**: Browse, search, and filter posts by category and tags
- **✏️ Markdown Editor**: Online editing with auto-save and keyboard shortcuts
- **🚀 Hugo Server Control**: Start/stop Hugo dev server with real-time logs
- **🔍 Advanced Search**: Full-text search with category and tag filtering
- **⚡ Real-time Updates**: WebSocket-based live log streaming
- **💾 Cache System**: SQLite-based caching for fast post retrieval

## Tech Stack

- **Backend**: Flask + Flask-SocketIO
- **Frontend**: Tailwind CSS + Alpine.js
- **Real-time Communication**: WebSocket (Socket.IO)
- **Process Management**: psutil
- **Database**: SQLite (for caching)

## Installation
### Docker (Recommended)
Pull the image from GHCR and run with Docker Compose:
```bash
# Clone the repository
git clone https://github.com/Svtter/hugo-admin.git
cd hugo-admin
# Start the service
docker compose up -d
```
Open your browser and navigate to `http://127.0.0.1:5050`.
See [docker-compose.yml](docker-compose.yml) for volume mounts and environment variables. Adjust the volume paths to match your Hugo site layout.
### Manual Setup
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
pip install .
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

## Configuration

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

## Usage

### Dashboard
- View blog statistics (post count, tags, categories)
- Check Hugo server status
- Quick access to common operations
- Recent posts overview

### Post Management
- Browse all posts with pagination
- Search posts by title, content, tags, or categories
- Filter by specific category or tag
- Click any post to edit

### Editor
- Edit Markdown files with syntax highlighting
- Auto-save on changes
- Keyboard shortcut: `Ctrl+S` / `Cmd+S` to save
- Real-time save status indicator

### Server Control
- Start Hugo server (with or without drafts)
- Stop running server
- View server status (PID, uptime, CPU, memory)
- Real-time log streaming

## Development

### Running Tests

```bash
# Install dev dependencies
pip install -r requirements-dev.txt

# Run all tests
pytest

# Run with coverage
pytest --cov=. --cov-report=html
```

### Project Structure

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

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request. For major changes, please open an issue first to discuss what you would like to change.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/AmazingFeature`)
3. Commit your changes (`git commit -m 'Add some AmazingFeature'`)
4. Push to the branch (`git push origin feature/AmazingFeature`)
5. Open a Pull Request

## Security

- Application binds to `127.0.0.1` by default (localhost only)
- File operations are restricted to the `content` directory
- Path traversal protection included
- Not recommended for production use on public networks without additional security measures

## Roadmap

- [x] Basic framework
- [x] Hugo server control
- [x] Post browsing and search
- [x] Markdown editor
- [x] Markdown preview
- [x] SQLite caching system
- [x] Test suite with CI/CD
- [x] Image upload and management
- [x] Docker support
- [ ] Git operations interface
- [ ] Batch operations
- [ ] Multi-user support

## License

Apache License 2.0 - see [LICENSE](LICENSE) file for details

## Acknowledgments

Built with ❤️ for the Hugo community.
