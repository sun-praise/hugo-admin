package httpapi

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"os"
	"path/filepath"
	"testing"
)

func TestImageUploadAndList(t *testing.T) {
	contentDir := t.TempDir()
	ts := newTestServerWithContent(t, contentDir)
	client := ts.Client()
	login(t, client, ts.URL)

	// 造一篇文章
	if err := os.MkdirAll(filepath.Join(contentDir, "post", "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contentDir, "post", "a", "index.md"), []byte("# A\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// multipart 上传
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "粘贴截图.png")
	_, _ = fw.Write(bytes.Repeat([]byte{0x89, 0x50}, 50))
	_ = mw.WriteField("article_path", "post/a/index.md")
	mw.Close()

	resp, err := client.Post(ts.URL+"/api/image/upload", mw.FormDataContentType(), &buf)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body["url"] != "pics/粘贴截图.png" {
		t.Fatalf("url = %#v", body)
	}

	// 文件真实落盘
	if _, err := os.Stat(filepath.Join(contentDir, "post", "a", "pics", "粘贴截图.png")); err != nil {
		t.Fatalf("文件未落盘: %v", err)
	}

	// 缺 article_path → 400
	buf.Reset()
	mw = multipart.NewWriter(&buf)
	fw, _ = mw.CreateFormFile("file", "x.png")
	_, _ = fw.Write([]byte("x"))
	mw.Close()
	resp2, err2 := client.Post(ts.URL+"/api/image/upload", mw.FormDataContentType(), &buf)
	if err2 != nil {
		t.Fatal(err2)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != 400 {
		t.Fatalf("缺路径 status = %d", resp2.StatusCode)
	}

	// 不支持的类型（路由层白名单）
	buf.Reset()
	mw = multipart.NewWriter(&buf)
	fw, _ = mw.CreateFormFile("file", "x.bmp")
	_, _ = fw.Write([]byte("x"))
	_ = mw.WriteField("article_path", "post/a/index.md")
	mw.Close()
	resp3, err3 := client.Post(ts.URL+"/api/image/upload", mw.FormDataContentType(), &buf)
	if err3 != nil {
		t.Fatal(err3)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != 400 {
		t.Fatalf("bmp status = %d", resp3.StatusCode)
	}

	// list：文章引用了刚上传的图
	if err := os.WriteFile(filepath.Join(contentDir, "post", "a", "index.md"),
		[]byte("# A\n\n![粘贴](pics/粘贴截图.png)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	status, listBody := doJSON(t, client, "POST", ts.URL+"/api/image/list",
		map[string]string{"article_path": "post/a/index.md"})
	if status != 200 {
		t.Fatalf("list status = %d", status)
	}
	images := listBody["images"].([]any)
	if len(images) != 1 || images[0].(map[string]any)["url"] != "pics/粘贴截图.png" {
		t.Fatalf("images = %#v", images)
	}
}
