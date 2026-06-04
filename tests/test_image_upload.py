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

    def _make_file_storage(
        self, filename="image.png", content=b"\x89PNG\r\n\x1a\nfake"
    ):
        """Create a werkzeug FileStorage with given filename and content."""
        return FileStorage(
            stream=io.BytesIO(content),
            filename=filename,
            content_type="image/png",
        )

    def _create_article(self, temp_content_dir, slug="my-post"):
        """Create a minimal article, return (rel_path, article_dir)."""
        article_dir = temp_content_dir / "posts" / slug
        article_dir.mkdir(parents=True)
        article_path = article_dir / "index.md"
        article_path.write_text("---\ntitle: Test\n---\ncontent\n")
        rel_path = str(article_path.relative_to(temp_content_dir))
        return rel_path, article_dir

    def test_save_image_avoids_collision(self, post_service, temp_content_dir):
        """Ctrl-V pasted images with name image.png must not overwrite each other."""
        rel_path, article_dir = self._create_article(temp_content_dir, "collision-test")

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
        rel_path, _ = self._create_article(temp_content_dir, "unique-test")

        file1 = self._make_file_storage("screenshot-2024.png", b"ss1")
        success1, url1 = post_service.save_image(rel_path, file1)
        assert success1
        assert url1 == "pics/screenshot-2024.png"

    def test_save_image_sanitizes_special_chars(self, post_service, temp_content_dir):
        """Special characters in filenames should be stripped."""
        rel_path, _ = self._create_article(temp_content_dir, "sanitizer-test")

        file1 = self._make_file_storage("my image (1).png", b"data")
        success1, url1 = post_service.save_image(rel_path, file1)
        assert success1
        assert " " not in url1
        assert "(" not in url1
        assert url1 == "pics/myimage1.png"

    def test_save_image_none_filename(self, post_service, temp_content_dir):
        """filename=None should fall back to image.png."""
        rel_path, _ = self._create_article(temp_content_dir, "none-filename")

        file1 = self._make_file_storage(None, b"data")
        success1, url1 = post_service.save_image(rel_path, file1)
        assert success1
        assert url1 == "pics/image.png"

    def test_save_image_rejects_disallowed_extension(
        self, post_service, temp_content_dir
    ):
        """Non-image extensions should be rejected."""
        rel_path, _ = self._create_article(temp_content_dir, "reject-ext")

        file1 = self._make_file_storage("malicious.php", b"<?php echo 1; ?>")
        file1.filename = "malicious.php"
        success, msg = post_service.save_image(rel_path, file1)
        assert not success
        assert "不支持的文件类型" in msg

    def test_save_image_rejects_svg(self, post_service, temp_content_dir):
        """SVG files should be rejected even though they're 'images'."""
        rel_path, _ = self._create_article(temp_content_dir, "reject-svg")

        file1 = FileStorage(
            stream=io.BytesIO(b"<svg></svg>"),
            filename="xss.svg",
            content_type="image/svg+xml",
        )
        success, msg = post_service.save_image(rel_path, file1)
        assert not success
        assert "不支持的文件类型" in msg

    def test_save_image_rejects_oversized_file(self, post_service, temp_content_dir):
        """Files exceeding the size limit should be rejected."""
        rel_path, _ = self._create_article(temp_content_dir, "reject-size")

        big_content = b"x" * (21 * 1024 * 1024)  # 21 MB
        file1 = self._make_file_storage("big.png", big_content)
        success, msg = post_service.save_image(rel_path, file1)
        assert not success
        assert "文件大小超出限制" in msg

    def test_save_image_allows_jpeg(self, post_service, temp_content_dir):
        """JPEG files with .jpeg extension should be accepted."""
        rel_path, _ = self._create_article(temp_content_dir, "jpeg-test")

        file1 = FileStorage(
            stream=io.BytesIO(b"\xff\xd8\xff\xe0fake"),
            filename="photo.jpeg",
            content_type="image/jpeg",
        )
        success, url1 = post_service.save_image(rel_path, file1)
        assert success
        assert url1 == "pics/photo.jpeg"

    def test_save_image_allows_jpg(self, post_service, temp_content_dir):
        """JPEG files with .jpg extension should be accepted."""
        rel_path, _ = self._create_article(temp_content_dir, "jpg-test")

        file1 = FileStorage(
            stream=io.BytesIO(b"\xff\xd8\xff\xe0fake"),
            filename="photo.jpg",
            content_type="image/jpeg",
        )
        success, url1 = post_service.save_image(rel_path, file1)
        assert success
        assert url1 == "pics/photo.jpg"

    def test_save_image_allows_webp(self, post_service, temp_content_dir):
        """WebP files should be accepted."""
        rel_path, _ = self._create_article(temp_content_dir, "webp-test")

        file1 = FileStorage(
            stream=io.BytesIO(b"RIFF\x00\x00\x00\x00WEBP"),
            filename="anim.webp",
            content_type="image/webp",
        )
        success, url1 = post_service.save_image(rel_path, file1)
        assert success
        assert url1 == "pics/anim.webp"

    def test_save_image_empty_filename_fallback(self, post_service, temp_content_dir):
        """Filename consisting only of special chars should fall back to image.png."""
        rel_path, _ = self._create_article(temp_content_dir, "empty-name")

        file1 = FileStorage(
            stream=io.BytesIO(b"data"),
            filename="!!!",
            content_type="image/png",
        )
        success, url1 = post_service.save_image(rel_path, file1)
        assert success
        assert url1 == "pics/image.png"

    def test_save_image_no_extension_fallback(self, post_service, temp_content_dir):
        """Filename without extension should fall back to image.png."""
        rel_path, _ = self._create_article(temp_content_dir, "no-ext")

        file1 = FileStorage(
            stream=io.BytesIO(b"data"),
            filename="readme",
            content_type="image/png",
        )
        success, url1 = post_service.save_image(rel_path, file1)
        assert success
        assert url1 == "pics/image.png"
