from pathlib import Path


def test_editor_preview_uses_decimal_ordered_list_markers():
    template_path = Path(__file__).parent.parent / "templates" / "editor.html"
    html = template_path.read_text(encoding="utf-8")

    assert ".markdown-body ol {" in html
    assert "list-style-type: decimal;" in html
