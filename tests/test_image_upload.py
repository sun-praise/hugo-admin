# coding: utf-8
"""
图片上传功能测试
"""

import io
import tempfile
from pathlib import Path

import pytest
from werkzeug.datastructures import FileStorage

from services.post_service import PostService


class TestImageUpload:
    @pytest.fixture
    def temp_content_dir(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            yield Path(temp_dir)

    @pytest.fixture
    def post_service(self, temp_content_dir):
        return PostService(temp_content_dir, use_cache=False)

    def _make_file_storage(self, filename="image.png", content=b"\x89PNG\r\n\x1a\nfake"):
        """Create a werkzeug FileStorage with given filename and content."""
        return FileStorage(
            stream=io.BytesIO(content),
            filename=filename,
            content_type="image/png",
        )

    def test_save_image_avoids_collision(self, post_service, temp_content_dir):
        """Ctrl-V pasted images with name image.png must not overwrite each other."""
        # Create a dummy article so the pics dir is under an article-specific path
        article_dir = temp_content_dir / "posts" / "my-post"
        article_dir.mkdir(parents=True)
        article_path = article_dir / "index.md"
        article_path.write_text("---\ntitle: Test\n---\ncontent\n")

        rel_path = str(article_path.relative_to(temp_content_dir))

        # Upload first image
        file1 = self._make_file_storage("image.png", b"image1")
        success1, url1 = post_service.save_image(rel_path, file1)
        assert success1
        assert url1 == "pics/image.png"

        # Upload second image with same filename — must not overwrite
        file2 = self._make_file_storage("image.png", b"image2")
        success2, url2 = post_service.save_image(rel_path, file2)
        assert success2
        assert url2 != url1, f"second upload should get a unique name, got {url2}"
        assert url2.startswith("pics/image_")
        assert url2.endswith(".png")

        # Verify both files exist on disk with correct content
        pics_dir = article_dir / "pics"
        files = sorted(pics_dir.iterdir())
        assert len(files) == 2, f"expected 2 files, got {files}"

        content1 = (pics_dir / "image.png").read_bytes()
        content2 = (pics_dir / url2.replace("pics/", "")).read_bytes()
        assert content1 == b"image1"
        assert content2 == b"image2"

    def test_save_image_unique_filename(self, post_service, temp_content_dir):
        """Files with unique names should keep their original names."""
        article_dir = temp_content_dir / "posts" / "another-post"
        article_dir.mkdir(parents=True)
        article_path = article_dir / "index.md"
        article_path.write_text("---\ntitle: Another\n---\ncontent\n")
        rel_path = str(article_path.relative_to(temp_content_dir))

        file1 = self._make_file_storage("screenshot-2024.png", b"ss1")
        success1, url1 = post_service.save_image(rel_path, file1)
        assert success1
        assert url1 == "pics/screenshot-2024.png"

    def test_save_image_sanitizes_special_chars(self, post_service, temp_content_dir):
        """Special characters in filenames should be stripped."""
        article_dir = temp_content_dir / "posts" / "test"
        article_dir.mkdir(parents=True)
        article_path = article_dir / "index.md"
        article_path.write_text("---\ntitle: Test\n---\ncontent\n")
        rel_path = str(article_path.relative_to(temp_content_dir))

        file1 = self._make_file_storage("my image (1).png", b"data")
        success1, url1 = post_service.save_image(rel_path, file1)
        assert success1
        assert " " not in url1
        assert "(" not in url1
