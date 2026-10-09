package posts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeArticle(t *testing.T, contentDir, rel, content string) string {
	t.Helper()
	path := filepath.Join(contentDir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSaveImage(t *testing.T) {
	dir := t.TempDir()
	writeArticle(t, dir, "post/a/index.md", "# A\n")

	png := make([]byte, 100)
	ok, url := SaveImage(dir, "post/a/index.md", "截图.png", png)
	if !ok || url != "pics/截图.png" {
		t.Fatalf("save = %v %q", ok, url)
	}
	data, err := os.ReadFile(filepath.Join(dir, "post", "a", "pics", "截图.png"))
	if err != nil || len(data) != 100 {
		t.Fatalf("文件内容: %v %d", err, len(data))
	}

	// 同名 → 短 UUID 后缀
	ok, url2 := SaveImage(dir, "post/a/index.md", "截图.png", png)
	if !ok || url2 == url || !strings.HasPrefix(url2, "pics/截图_") {
		t.Fatalf("重名 = %v %q", ok, url2)
	}

	// 文件名清洗：非法字符剔除（Python 语义：'/' 与空格、'!' 剔除，
	// 前导的点保留形成 "..evilname.png"）
	ok, url3 := SaveImage(dir, "post/a/index.md", "../evil name!.png", png)
	if !ok || url3 != "pics/..evilname.png" {
		t.Fatalf("清洗 = %v %q", ok, url3)
	}
	ok, url4 := SaveImage(dir, "post/a/index.md", "///", png)
	if !ok || url4 != "pics/image.png" {
		t.Fatalf("回退 = %v %q", ok, url4)
	}

	// 白名单（服务层不含 svg）
	ok, msg := SaveImage(dir, "post/a/index.md", "x.svg", png)
	if ok || msg != "不支持的文件类型: .svg" {
		t.Fatalf("svg = %v %q", ok, msg)
	}

	// 超限
	ok, msg = SaveImage(dir, "post/a/index.md", "big.png", make([]byte, MaxImageSize+1))
	if ok || !strings.Contains(msg, "文件大小超出限制") {
		t.Fatalf("超限 = %v %q", ok, msg)
	}
}

func TestListImages(t *testing.T) {
	dir := t.TempDir()
	writeArticle(t, dir, "post/b/index.md", `---
title: B
cover: https://cdn.example.com/cover.webp
image: 'pics/本地图.png'
featured_image: "not-an-image"
---

正文 ![第一张](https://cdn.example.com/a.png "标题")
再看 ![重复](https://cdn.example.com/a.png)
本地 ![本地](pics/本地图.png)
非图 ![链接](https://example.com/page)
`)

	ok, result := ListImages(dir, "post/b/index.md")
	if !ok {
		t.Fatalf("list = %v", result)
	}
	images := result.([]ImageEntry)
	// Python 语义：markdown 分支不筛扩展名（page 也收）；frontmatter
	// 分支要求 http 或图片后缀；body 先于 frontmatter
	want := []ImageEntry{
		{Name: "a.png", URL: "https://cdn.example.com/a.png"},
		{Name: "本地图.png", URL: "pics/本地图.png"},
		{Name: "page", URL: "https://example.com/page"},
		{Name: "cover.webp", URL: "https://cdn.example.com/cover.webp"},
	}
	if len(images) != len(want) {
		t.Fatalf("images = %#v", images)
	}
	for i := range want {
		if images[i] != want[i] {
			t.Errorf("images[%d] = %#v, want %#v", i, images[i], want[i])
		}
	}

	// 不存在 → 空列表
	ok, result = ListImages(dir, "post/none/index.md")
	if !ok || len(result.([]ImageEntry)) != 0 {
		t.Fatalf("不存在 = %v %#v", ok, result)
	}
}
