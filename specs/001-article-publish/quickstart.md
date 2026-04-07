# Implementation Quickstart

**Feature**: Article Publishing
**Purpose**: Step-by-step implementation guide for developers

## Overview

This guide provides a practical roadmap for implementing the article publishing functionality. The implementation is divided into manageable phases that can be developed and tested independently.

## Prerequisites

- Python 3.11+ environment
- Hugo blog with content directory structure
- Existing Flask application with post service
- Dependencies: Flask, PyYAML, python-frontmatter already installed

## Implementation Phases

### Phase 1: Backend Service Layer (1-2 days)

#### 1.1 Extend PostService with Publishing Methods

**File**: `services/post_service.py`

Add these methods to the existing `PostService` class:

```python
import frontmatter
import fcntl
import uuid
from datetime import datetime
from pathlib import Path

def publish_article(self, file_path: str) -> Tuple[bool, str, str]:
    """
    Publish an article by changing draft: true to draft: false

    Args:
        file_path: Absolute path to article file

    Returns:
        Tuple of (success, message, operation_id)
    """
    operation_id = str(uuid.uuid4())

    try:
        # Validate file path and existence
        if not self._validate_file_path(file_path):
            return False, "Invalid file path", operation_id

        # Use file locking for concurrency
        def publish_operation(file_handle):
            post = frontmatter.load(file_path)

            # Check if already published
            if not post.get('draft', False):
                return False, "Article already published"

            # Update draft status
            post['draft'] = False

            # Save with updated frontmatter
            frontmatter.dump(post, file_path.name)
            return True, "Article published successfully"

        result, message = self._safe_file_operation(file_path, publish_operation)

        # Invalidate cache for this article
        if self.cache_service:
            self.cache_service.delete(f"article:publish_status:{file_path}")

        return result, message, operation_id

    except Exception as e:
        return False, f"Publish failed: {str(e)}", operation_id

def bulk_publish_articles(self, file_paths: List[str]) -> Dict:
    """
    Publish multiple articles in bulk

    Args:
        file_paths: List of absolute file paths

    Returns:
        Dictionary with bulk operation results
    """
    operation_id = str(uuid.uuid4())
    results = []
    published_count = 0
    failed_count = 0

    for file_path in file_paths:
        success, message, _ = self.publish_article(file_path)
        result = {
            'file_path': file_path,
            'success': success,
            'error': message if not success else None
        }
        results.append(result)

        if success:
            published_count += 1
        else:
            failed_count += 1

    return {
        'success': failed_count == 0,
        'total_count': len(file_paths),
        'published_count': published_count,
        'failed_count': failed_count,
        'operation_id': operation_id,
        'results': results
    }

def get_publish_status(self, file_path: str) -> Dict:
    """
    Get current publish status of an article

    Args:
        file_path: Absolute path to article file

    Returns:
        Dictionary with publish status information
    """
    try:
        if not Path(file_path).exists():
            return {'error': 'File not found'}

        post = frontmatter.load(file_path)
        is_draft = post.get('draft', False)

        return {
            'file_path': file_path,
            'is_draft': is_draft,
            'is_publishable': is_draft,  # Can add more complex logic here
            'last_published': post.get('last_published'),
            'publish_errors': [] if not is_draft else []
        }

    except Exception as e:
        return {'error': f'Status check failed: {str(e)}'}

# Helper methods to add
def _validate_file_path(self, file_path: str) -> bool:
    """Validate file is within allowed content directory"""
    try:
        path = Path(file_path)
        content_dir = Path(self.content_dir)
        return path.exists() and str(path).startswith(str(content_dir))
    except:
        return False

def _safe_file_operation(self, file_path: str, operation, timeout=5):
    """Perform file operation with locking"""
    import time

    start_time = time.time()
    while time.time() - start_time < timeout:
        try:
            with open(file_path, 'r+') as f:
                fcntl.flock(f.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
                return operation(f)
        except (IOError, OSError):
            time.sleep(0.1)

    return False, "Could not acquire file lock"
```

#### 1.2 Add API Endpoints

**File**: `app.py`

Add these new routes after the existing API endpoints:

```python
# Article Publishing Endpoints

@app.route('/api/article/publish', methods=['POST'])
def publish_article():
    """Publish a single article"""
    data = request.get_json()

    if not data or 'file_path' not in data:
        return jsonify({
            'success': False,
            'error': 'Missing file_path parameter',
            'error_code': 'MISSING_PARAMETER'
        }), 400

    file_path = data['file_path']
    success, message, operation_id = post_service.publish_article(file_path)

    if success:
        return jsonify({
            'success': True,
            'message': message,
            'operation_id': operation_id,
            'article_path': file_path,
            'draft_status_changed': True,
            'published_at': datetime.utcnow().isoformat() + 'Z'
        })
    else:
        status_code = 404 if "not found" in message.lower() else 409 if "already" in message.lower() else 400
        return jsonify({
            'success': False,
            'error': message,
            'error_code': 'PUBLISH_FAILED'
        }), status_code

@app.route('/api/article/publish/bulk', methods=['POST'])
def bulk_publish_articles():
    """Publish multiple articles"""
    data = request.get_json()

    if not data or 'file_paths' not in data:
        return jsonify({
            'success': False,
            'error': 'Missing file_paths parameter',
            'error_code': 'MISSING_PARAMETER'
        }), 400

    file_paths = data['file_paths']
    if not isinstance(file_paths, list) or len(file_paths) == 0:
        return jsonify({
            'success': False,
            'error': 'file_paths must be a non-empty array',
            'error_code': 'INVALID_PARAMETER'
        }), 400

    result = post_service.bulk_publish_articles(file_paths)
    return jsonify(result)

@app.route('/api/article/status')
def get_article_status():
    """Get publish status of an article"""
    file_path = request.args.get('file_path')

    if not file_path:
        return jsonify({
            'success': False,
            'error': 'Missing file_path parameter',
            'error_code': 'MISSING_PARAMETER'
        }), 400

    status = post_service.get_publish_status(file_path)

    if 'error' in status:
        status_code = 404 if "not found" in status['error'].lower() else 400
        return jsonify({
            'success': False,
            'error': status['error'],
            'error_code': 'STATUS_CHECK_FAILED'
        }), status_code

    return jsonify({
        'success': True,
        'status': status
    })

@app.route('/api/article/status/bulk')
def get_bulk_article_status():
    """Get publish status of multiple articles"""
    file_paths_str = request.args.get('file_paths', '')

    if not file_paths_str:
        return jsonify({
            'success': False,
            'error': 'Missing file_paths parameter',
            'error_code': 'MISSING_PARAMETER'
        }), 400

    file_paths = [path.strip() for path in file_paths_str.split(',') if path.strip()]
    statuses = []

    for file_path in file_paths:
        status = post_service.get_publish_status(file_path)
        if 'error' not in status:
            statuses.append(status)
        else:
            statuses.append({
                'file_path': file_path,
                'error': status['error']
            })

    return jsonify({
        'success': True,
        'statuses': statuses
    })
```

### Phase 2: Frontend UI Components (1-2 days)

#### 2.1 Update Article Editor Template

**File**: `templates/editor.html`

Add publish button near existing save buttons:

```html
<!-- Find the save button section and add publish button -->
<div class="flex gap-2 mb-4">
    <button
        hx-post="/api/file/save"
        hx-include="[name='content']"
        hx-target="#save-status"
        class="px-4 py-2 bg-blue-500 text-white rounded hover:bg-blue-600">
        Save
    </button>

    <!-- NEW: Publish Button -->
    <button
        id="publish-btn"
        hx-post="/api/article/publish"
        hx-include='[name="current_file_path"]'
        hx-target="#publish-status"
        hx-indicator="#publish-loading"
        class="px-4 py-2 bg-green-500 text-white rounded hover:bg-green-600"
        onclick="confirmPublish(event)">
        Publish
    </button>

    <div id="publish-loading" class="htmx-indicator hidden">
        Publishing...
    </div>
    <div id="publish-status" class="text-sm mt-1"></div>
</div>

<!-- Add hidden input for current file path if not already present -->
<input type="hidden" name="current_file_path" id="current_file_path" value="{{ current_file_path or '' }}">

<script>
function confirmPublish(event) {
    const isConfirmed = confirm('Are you sure you want to publish this article? This will make it visible to readers.');
    if (!isConfirmed) {
        event.preventDefault();
        return false;
    }
}

// Update publish button state based on article status
document.addEventListener('DOMContentLoaded', function() {
    const filePath = document.getElementById('current_file_path').value;
    if (filePath) {
        fetch(`/api/article/status?file_path=${encodeURIComponent(filePath)}`)
            .then(response => response.json())
            .then(data => {
                if (data.success && !data.status.is_draft) {
                    const publishBtn = document.getElementById('publish-btn');
                    publishBtn.textContent = 'Already Published';
                    publishBtn.disabled = true;
                    publishBtn.classList.remove('bg-green-500', 'hover:bg-green-600');
                    publishBtn.classList.add('bg-gray-400');
                }
            })
            .catch(error => console.error('Error checking publish status:', error));
    }
});
</script>
```

#### 2.2 Update Article List Template

**File**: `templates/posts.html`

Add status indicators to article list:

```html
<!-- Find the article listing section and add status badges -->
<div id="posts-list" class="space-y-4">
    {% for post in posts %}
    <div class="border rounded p-4 hover:bg-gray-50">
        <div class="flex justify-between items-start">
            <div class="flex-1">
                <h3 class="text-lg font-semibold">
                    <a href="/editor/{{ post.path }}" class="text-blue-600 hover:underline">
                        {{ post.title or post.path }}
                    </a>
                </h3>
                <p class="text-sm text-gray-600">{{ post.path }}</p>
                <p class="text-xs text-gray-500">
                    {{ post.date or 'No date' }} |
                    {% if post.categories %}
                        Categories: {{ post.categories|join(', ') }}
                    {% endif %}
                </p>
            </div>

            <!-- NEW: Status Badge -->
            <div class="ml-4">
                <span id="status-{{ loop.index }}" class="px-2 py-1 text-xs rounded-full">
                    <!-- Status will be loaded via JavaScript -->
                    <span class="text-gray-500">Loading...</span>
                </span>
            </div>
        </div>
    </div>
    {% endfor %}
</div>

<!-- Add bulk publish controls -->
<div class="mt-6 border-t pt-4">
    <div class="flex justify-between items-center">
        <div>
            <input type="checkbox" id="select-all" onchange="toggleAllArticles()">
            <label for="select-all" class="ml-2">Select All</label>
        </div>
        <button
            id="bulk-publish-btn"
            onclick="bulkPublishSelected()"
            class="px-4 py-2 bg-green-500 text-white rounded hover:bg-green-600 disabled:bg-gray-400"
            disabled>
            Publish Selected (0)
        </button>
    </div>
</div>

<script>
// Load status indicators
document.addEventListener('DOMContentLoaded', function() {
    const articles = [
        {% for post in posts %}
        { path: '{{ post.path }}', index: {{ loop.index }} }{% if not loop.last %},{% endif %}
        {% endfor %}
    ];

    articles.forEach(article => {
        fetch(`/api/article/status?file_path=${encodeURIComponent(article.path)}`)
            .then(response => response.json())
            .then(data => {
                if (data.success) {
                    const statusEl = document.getElementById(`status-${article.index}`);
                    if (data.status.is_draft) {
                        statusEl.innerHTML = '<span class="bg-yellow-100 text-yellow-800">Draft</span>';
                    } else {
                        statusEl.innerHTML = '<span class="bg-green-100 text-green-800">Published</span>';
                    }
                }
            })
            .catch(error => {
                console.error('Error loading status:', error);
                const statusEl = document.getElementById(`status-${article.index}`);
                statusEl.innerHTML = '<span class="bg-red-100 text-red-800">Error</span>';
            });
    });
});

// Bulk publish functionality
function toggleAllArticles() {
    const selectAll = document.getElementById('select-all');
    const checkboxes = document.querySelectorAll('input[name="article-select"]');
    checkboxes.forEach(cb => cb.checked = selectAll.checked);
    updateBulkPublishButton();
}

function updateBulkPublishButton() {
    const selectedCount = document.querySelectorAll('input[name="article-select"]:checked').length;
    const bulkBtn = document.getElementById('bulk-publish-btn');
    bulkBtn.textContent = `Publish Selected (${selectedCount})`;
    bulkBtn.disabled = selectedCount === 0;
}

function bulkPublishSelected() {
    const selected = Array.from(document.querySelectorAll('input[name="article-select"]:checked'))
        .map(cb => cb.value);

    if (selected.length === 0) return;

    if (!confirm(`Publish ${selected.length} selected articles?`)) return;

    fetch('/api/article/publish/bulk', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ file_paths: selected })
    })
    .then(response => response.json())
    .then(data => {
        if (data.success) {
            alert(`Successfully published ${data.published_count} articles.`);
            // Reload page to update status indicators
            location.reload();
        } else {
            alert(`Published ${data.published_count} of ${data.total_count} articles. Some failed.`);
        }
    })
    .catch(error => {
        console.error('Bulk publish error:', error);
        alert('Error publishing articles');
    });
}
</script>
```

### Phase 3: Testing (1 day)

#### 3.1 Add Unit Tests

**File**: `tests/test_publish_api.py`

```python
import pytest
import tempfile
import os
from pathlib import Path
import frontmatter
from app import app
from services.post_service import PostService

class TestPublishAPI:

    @pytest.fixture
    def client(self):
        app.config['TESTING'] = True
        with app.test_client() as client:
            yield client

    @pytest.fixture
    def temp_article(self):
        """Create a temporary article file for testing"""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.md', delete=False) as f:
            article_content = """---
title: Test Article
draft: true
date: 2025-11-14
---

This is test content.
"""
            f.write(article_content)
            temp_path = f.name

        yield temp_path

        # Cleanup
        if os.path.exists(temp_path):
            os.unlink(temp_path)

    def test_publish_article_success(self, client, temp_article):
        """Test successful article publishing"""
        response = client.post('/api/article/publish',
                             json={'file_path': temp_article})

        assert response.status_code == 200
        data = response.get_json()
        assert data['success'] is True
        assert 'operation_id' in data

        # Verify file was actually changed
        post = frontmatter.load(temp_article)
        assert post.get('draft') is False

    def test_publish_already_published(self, client, temp_article):
        """Test publishing already published article"""
        # First publish
        client.post('/api/article/publish', json={'file_path': temp_article})

        # Try to publish again
        response = client.post('/api/article/publish',
                             json={'file_path': temp_article})

        assert response.status_code == 409
        data = response.get_json()
        assert data['success'] is False
        assert 'already published' in data['error'].lower()

    def test_get_article_status(self, client, temp_article):
        """Test getting article status"""
        response = client.get(f'/api/article/status?file_path={temp_article}')

        assert response.status_code == 200
        data = response.get_json()
        assert data['success'] is True
        assert data['status']['is_draft'] is True
        assert data['status']['is_publishable'] is True

    def test_bulk_publish(self, client, temp_article):
        """Test bulk publishing"""
        # Create second temporary article
        with tempfile.NamedTemporaryFile(mode='w', suffix='.md', delete=False) as f:
            article_content = """---
title: Test Article 2
draft: true
---

This is test content 2.
"""
            f.write(article_content)
            temp_path2 = f.name

        try:
            response = client.post('/api/article/publish/bulk',
                                 json={'file_paths': [temp_article, temp_path2]})

            assert response.status_code == 200
            data = response.get_json()
            assert data['success'] is True
            assert data['published_count'] == 2
            assert len(data['results']) == 2

        finally:
            if os.path.exists(temp_path2):
                os.unlink(temp_path2)

class TestPostService:

    @pytest.fixture
    def temp_content_dir(self):
        """Create temporary content directory"""
        with tempfile.TemporaryDirectory() as temp_dir:
            yield temp_dir

    def test_publish_article_service(self, temp_content_dir):
        """Test PostService publish method"""
        # Create test article
        article_path = Path(temp_content_dir) / "test.md"
        article_content = """---
title: Test
draft: true
---

Content here
"""
        article_path.write_text(article_content)

        service = PostService(temp_content_dir, use_cache=False)
        success, message, operation_id = service.publish_article(str(article_path))

        assert success is True
        assert "published successfully" in message.lower()
        assert operation_id is not None

        # Verify file was changed
        post = frontmatter.load(str(article_path))
        assert post.get('draft') is False
```

#### 3.2 Add Integration Tests

**File**: `tests/test_publish_integration.py`

```python
import pytest
import tempfile
import os
from app import app

class TestPublishIntegration:

    @pytest.fixture
    def client(self):
        app.config['TESTING'] = True
        with app.test_client() as client:
            yield client

    def test_full_publish_workflow(self, client):
        """Test complete publish workflow from status check to publish"""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.md', delete=False) as f:
            article_content = """---
title: Workflow Test
draft: true
---

This is a workflow test.
"""
            f.write(article_content)
            temp_path = f.name

        try:
            # Step 1: Check initial status
            response = client.get(f'/api/article/status?file_path={temp_path}')
            assert response.status_code == 200
            data = response.get_json()
            assert data['status']['is_draft'] is True

            # Step 2: Publish article
            response = client.post('/api/article/publish',
                                 json={'file_path': temp_path})
            assert response.status_code == 200

            # Step 3: Verify updated status
            response = client.get(f'/api/article/status?file_path={temp_path}')
            assert response.status_code == 200
            data = response.get_json()
            assert data['status']['is_draft'] is False

        finally:
            if os.path.exists(temp_path):
                os.unlink(temp_path)
```

### Phase 4: Integration and Deployment (0.5 day)

#### 4.1 Update Requirements

Ensure `python-frontmatter` is in `requirements.txt` (should already be there).

#### 4.2 Test Deployment

1. Run the application: `python app.py`
2. Navigate to article editor: `http://localhost:5000/editor`
3. Create or edit an article with `draft: true`
4. Click publish button
5. Verify status changes and UI updates
6. Test bulk publishing from article list

#### 4.3 Performance Testing

Test with various scenarios:
- Publishing single article
- Bulk publishing 10+ articles
- Concurrent publish requests
- Large articles with extensive frontmatter

## Success Criteria

- [ ] Publish button appears in editor for draft articles
- [ ] Clicking publish changes draft status from true to false
- [ ] Status indicators show correct draft/published state
- [ ] Bulk publish works for multiple articles
- [ ] Error handling works for file permissions, locks, etc.
- [ ] UI provides clear feedback for all operations
- [ ] All tests pass

## Troubleshooting

### Common Issues

1. **Permission Denied**: Ensure web application has write permissions to content directory
2. **File Lock Timeout**: Increase timeout in `_safe_file_operation` if needed
3. **Invalid YAML**: Check frontmatter format in article files
4. **Cache Issues**: Clear cache if status indicators don't update: `POST /api/cache/refresh`

### Debug Mode

Enable debug logging by setting:
```python
import logging
logging.basicConfig(level=logging.DEBUG)
```

### Testing with Real Hugo Content

1. Copy some real Hugo articles to test directory
2. Test publishing workflow
3. Verify Hugo can still build published content correctly

## Next Steps

After implementation:
1. User acceptance testing with actual blog content
2. Performance monitoring in production
3. Consider adding scheduled publishing (future enhancement)
4. Add publishing history and audit logs
5. Integration with CI/CD pipeline for automated deployments
