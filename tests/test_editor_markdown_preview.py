from pathlib import Path


def test_editor_preview_uses_decimal_ordered_list_markers():
    css_path = Path(__file__).parent.parent / "frontend" / "src" / "index.css"
    css = css_path.read_text(encoding="utf-8")

    assert ".markdown-body ol {" in css
    assert "list-style-type: decimal;" in css
