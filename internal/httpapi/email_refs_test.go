package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/svtter/hugo-admin/internal/db"
	"github.com/svtter/hugo-admin/internal/refs"
)

// refsTestContent：a 引用 b（ref shortcode + 相对路径），c 引用 b（裸文件名）。
const refsTestContent = `---
title: 文章 A
date: 2026-03-01
draft: false
tags: [go]
categories: [tech]
---

正文引用 {{< ref "post/b.md" >}} 这里。
`

func newRefsTestServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	contentDir := t.TempDir()
	os.MkdirAll(filepath.Join(contentDir, "post"), 0o755)
	os.WriteFile(filepath.Join(contentDir, "post", "a.md"), []byte(refsTestContent), 0o644)
	os.WriteFile(filepath.Join(contentDir, "post", "b.md"),
		[]byte("---\ntitle: 文章 B\ndate: 2026-02-01\n---\n被引用\n"), 0o644)
	os.WriteFile(filepath.Join(contentDir, "post", "c.md"),
		[]byte("---\ntitle: 文章 C\ndate: 2026-04-01\n---\n也引用 {{< ref \"b.md\" >}}\n"), 0o644)

	database, err := db.Open(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	refsSvc := refs.New(contentDir, database)
	refsSvc.ScanAll()
	ts := newTestServerOpts(t, contentDir, Options{Database: database, Refs: refsSvc})
	return ts, contentDir
}

func TestReferencesScanAndBacklinks(t *testing.T) {
	srv, _ := newRefsTestServer(t)
	client := srv.Client()
	login(t, client, srv.URL)

	// scan
	status, body := doJSON(t, client, "POST", srv.URL+"/api/references/scan", nil)
	if status != 200 || body["success"] != true {
		t.Fatalf("scan = %d %#v", status, body)
	}
	references := body["references"].(map[string]any)
	if len(references) < 2 {
		t.Fatalf("references = %#v", references)
	}

	// backlinks：b.md 被 a 和 c 引用（c 更新，date 排前）
	status, body = doJSON(t, client, "GET",
		srv.URL+"/api/references/backlinks?path=post/b.md", nil)
	if status != 200 {
		t.Fatalf("backlinks = %d", status)
	}
	bl := body["backlinks"].([]any)
	if len(bl) != 2 {
		t.Fatalf("backlinks = %#v", bl)
	}
	first := bl[0].(map[string]any)
	if first["title"] != "文章 C" && first["title"] != "文章 A" {
		t.Fatalf("first = %#v", first)
	}

	// search：按标题
	status, body = doJSON(t, client, "GET", srv.URL+"/api/posts/search?q=文章", nil)
	if status != 200 {
		t.Fatalf("search = %d", status)
	}
	postsFound := body["posts"].([]any)
	if len(postsFound) < 3 {
		t.Fatalf("search = %#v", postsFound)
	}
	// 空 query → 空列表
	status, body = doJSON(t, client, "GET", srv.URL+"/api/posts/search", nil)
	if status != 200 || len(body["posts"].([]any)) != 0 {
		t.Fatalf("empty search = %d %#v", status, body)
	}
	// 缺 path → 400
	status, _ = doJSON(t, client, "GET", srv.URL+"/api/references/backlinks", nil)
	if status != 400 {
		t.Fatalf("missing path = %d", status)
	}
}

// newEmailTestServer 构造 listmonk mock + 固定 RSS 的测试服务。
func newEmailTestServer(t *testing.T) (*httptest.Server, *httptest.Server) {
	t.Helper()
	// listmonk mock：记录 campaign 创建
	var createdCampaigns []map[string]any
	listmonk := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/api/campaigns"):
			var payload map[string]any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			createdCampaigns = append(createdCampaigns, payload)
			fmt.Fprintf(w, `{"data":{"id":42}}`)
		case r.Method == "PUT" && strings.Contains(r.URL.Path, "/status"):
			fmt.Fprint(w, `{"data":true}`)
		default:
			http.NotFound(w, r)
		}
	}))

	// RSS mock
	rss := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>Blog</title>
<item><title>Go 并发模式</title><link>https://svtter.cn/post/go-concurrency/</link>
<pubDate>Mon, 05 Oct 2026 10:00:00 +0800</pubDate>
<description>&lt;p&gt;并发是 Go 的核心&lt;/p&gt;</description>
<category>go</category></item>
<item><title>旧文章</title><link>https://svtter.cn/post/old-post/</link></item>
</channel></rss>`)
	}))
	t.Cleanup(listmonk.Close)
	t.Cleanup(rss.Close)

	// 构造服务：设置 listmonk 指向 mock
	contentDir := t.TempDir()
	database, _ := db.Open(filepath.Join(t.TempDir(), "cache.db"))
	ts := newTestServerOpts(t, contentDir, Options{
		Database: database,
		Refs:     refs.New(contentDir, database),
	})
	return ts, listmonk
}

func TestEmailEndpoints(t *testing.T) {
	srv, _ := newEmailTestServer(t)
	client := srv.Client()
	login(t, client, srv.URL)

	// push-article 缺 URL → 400
	status, body := doJSON(t, client, "POST", srv.URL+"/api/email/push-article", map[string]any{})
	if status != 400 || body["message"] != "缺少文章 URL 参数" {
		t.Fatalf("missing url = %d %#v", status, body)
	}

	// preview-article 缺 URL → 400
	status, _ = doJSON(t, client, "GET", srv.URL+"/api/email/preview-article", nil)
	if status != 400 {
		t.Fatalf("preview missing = %d", status)
	}
}
