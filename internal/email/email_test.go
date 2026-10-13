package email

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testFeed = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel>
<title>Blog</title><link>https://blog.example.com</link><description>d</description>
<item>
  <title>First</title>
  <link>https://blog.example.com/posts/first/</link>
  <pubDate>Mon, 02 Jan 2006 15:04:05 -0700</pubDate>
  <description>hello &lt;b&gt;world&lt;/b&gt;</description>
  <category>go</category><category>hugo</category><category>blog</category><category>extra1</category><category>extra2</category>
</item>
<item>
  <title>Second</title>
  <link>https://blog.example.com/posts/second/index.html</link>
  <description>d2</description>
</item>
<item>
  <title>DupOfFirst</title>
  <link>https://blog.example.com/posts/first/</link>
  <description>d3</description>
</item>
</channel></rss>`

// newService 构造指向本地假 RSS 的服务。
func newService(t *testing.T, feed string) *Service {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, feed)
	}))
	t.Cleanup(srv.Close)
	s := New(map[string]any{
		"api_url": "http://listmonk.invalid", "api_user": "u", "api_key": "k",
	}, false, t.TempDir())
	s.rssURL = srv.URL
	return s
}

func TestNewParsesListmonkConfig(t *testing.T) {
	s := New(map[string]any{
		"api_url": "https://lm.example.com", "api_user": "user",
		"api_key": "key", "blog_list_id": 7,
	}, false, t.TempDir())
	if s.apiURL != "https://lm.example.com" || s.apiUser != "user" || s.apiKey != "key" || s.listID != 7 {
		t.Fatalf("service = %+v", s)
	}
	// JSON 反序列化场景下 list_id 是 float64
	s = New(map[string]any{"blog_list_id": float64(9)}, false, t.TempDir())
	if s.listID != 9 {
		t.Fatalf("listID = %d", s.listID)
	}
	// 缺省 listID
	s = New(nil, true, t.TempDir())
	if s.listID != 1 || !s.debugMode {
		t.Fatalf("listID = %d, debug = %v", s.listID, s.debugMode)
	}
	if !strings.HasSuffix(s.sentFile, filepath.Join(".config", "latest_post_sent.json")) {
		t.Fatalf("sentFile = %q", s.sentFile)
	}
}

func TestGetLatestPost(t *testing.T) {
	s := newService(t, testFeed)
	p, err := s.GetLatestPost()
	if err != nil || p == nil {
		t.Fatalf("post = %v, err = %v", p, err)
	}
	if p.Title != "First" || p.Link != "https://blog.example.com/posts/first/" {
		t.Fatalf("post = %+v", p)
	}
	if len(p.Tags) != 5 || p.Tags[0] != "go" {
		t.Fatalf("tags = %#v", p.Tags)
	}
	if p.Published != "Mon, 02 Jan 2006 22:04:05 +0000" {
		t.Fatalf("published = %q（gofeed 会把 -0700 时区归一化为 UTC）", p.Published)
	}
	if p.Summary != "hello <b>world</b>" {
		t.Fatalf("summary = %q", p.Summary)
	}

	// 空 feed → nil
	s2 := newService(t, `<?xml version="1.0"?><rss version="2.0"><channel><title>t</title></channel></rss>`)
	if p, err := s2.GetLatestPost(); p != nil || err != nil {
		t.Fatalf("空 feed 应返回 nil: %v, %v", p, err)
	}

	// 非 XML → 错误
	s3 := newService(t, "not-xml")
	if _, err := s3.GetLatestPost(); err == nil {
		t.Fatal("坏 feed 应报错")
	}
}

func TestGetPostByURL(t *testing.T) {
	s := newService(t, testFeed)

	p, err := s.GetPostByURL("https://blog.example.com/posts/second/index.html")
	if err != nil || p == nil || p.Title != "Second" {
		t.Fatalf("index.html 后缀应匹配: %v, %v", p, err)
	}
	p, err = s.GetPostByURL("https://www.blog.example.com/posts/second/")
	if err != nil || p == nil || p.Title != "Second" {
		t.Fatalf("www 前缀与尾部斜杠应匹配: %v, %v", p, err)
	}
	// 同一 path 出现两次 → 视为未找到
	if p, err := s.GetPostByURL("https://blog.example.com/posts/first/"); err != nil || p != nil {
		t.Fatalf("多匹配应视为未找到: %v, %v", p, err)
	}
	// 不存在的文章
	if p, err := s.GetPostByURL("https://blog.example.com/posts/none/"); err != nil || p != nil {
		t.Fatalf("未找到: %v, %v", p, err)
	}
	// 裸 slug 拒绝
	if p, err := s.GetPostByURL("just-a-slug"); err != nil || p != nil {
		t.Fatalf("裸 slug 应拒绝: %v, %v", p, err)
	}
}

func TestNormalizeURLForMatch(t *testing.T) {
	cases := []struct {
		in     string
		netloc string
		path   string
		ok     bool
	}{
		{"https://www.Example.COM/a/index.html", "example.com", "/a", true},
		{"http://Example.com/a/b.htm", "example.com", "/a/b", true},
		{"/path/", "", "/path", true},
		{"/path/index.html", "", "/path", true},
		{"slug", "", "", false},
		{"", "", "", false},
		{"example.com/x", "example.com", "/x", true},
	}
	for _, tc := range cases {
		netloc, path, ok := normalizeURLForMatch(tc.in)
		if ok != tc.ok || netloc != tc.netloc || path != tc.path {
			t.Fatalf("normalizeURLForMatch(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tc.in, netloc, path, ok, tc.netloc, tc.path, tc.ok)
		}
	}
}

func TestStripHTMLSuffix(t *testing.T) {
	cases := map[string]string{
		"/a/index.html": "/a",
		"/a/index.htm":  "/a",
		"/a/b.html":     "/a/b",
		"/a/b.htm":      "/a/b",
		"/a/":           "/a/",
	}
	for in, want := range cases {
		if got := stripHTMLSuffix(in); got != want {
			t.Fatalf("stripHTMLSuffix(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCreateEmailContent(t *testing.T) {
	s := New(nil, false, t.TempDir())
	post := &Post{Title: "标题", Published: "Mon, 02 Jan 2006 15:04:05 -0700",
		Summary: strings.Repeat("字", 500), Link: "https://blog.example.com/p/1/",
		Tags: []string{"a", "b", "c", "d"}}

	subject, body := s.CreateEmailContent(post)
	if subject != "📖 新文章发布：标题" {
		t.Fatalf("subject = %q", subject)
	}
	// 超长摘要截断到 400 rune + 省略号；HTML 标签剔除
	if strings.Contains(body, strings.Repeat("字", 401)) {
		t.Fatal("摘要应截断到 400 字")
	}
	if !strings.Contains(body, strings.Repeat("字", 400)) {
		t.Fatal("截断后应保留前 400 字")
	}
	if !strings.Contains(body, "...</div>") && !strings.Contains(body, "...") {
		t.Fatal("截断应带省略号")
	}
	// 标签最多渲染 3 个（#e3f2fd 是标签 chip 独有的背景色）
	if !strings.Contains(body, "#e3f2fd") {
		t.Fatal("应渲染标签 chip")
	}
	if strings.Contains(body, "#d</span>") {
		t.Fatal("第 4 个标签不应渲染")
	}
	if !strings.Contains(body, post.Link) {
		t.Fatal("正文应包含文章链接")
	}

	// Summary 为空时回退 Description
	_, body = s.CreateEmailContent(&Post{Title: "T", Description: "fallback-desc", Link: "l"})
	if !strings.Contains(body, "fallback-desc") {
		t.Fatal("Summary 空时应使用 Description")
	}
	// 无标签 → 无标签栏
	_, body = s.CreateEmailContent(&Post{Title: "T", Link: "l"})
	if strings.Contains(body, "#e3f2fd") {
		t.Fatal("无标签时不应渲染标签栏")
	}
}

func TestNormalizeListmonkTemplateVars(t *testing.T) {
	in := "{{.UnsubscribeURL}} {{ .MessageURL }} {{ .TrackView }} {{ .TrackLink \"x\" }}"
	out := NormalizeListmonkTemplateVars(in)
	for _, want := range []string{"{{ UnsubscribeURL }}", "{{ MessageURL }}", "{{ TrackView }}", `{{ TrackLink "x" }}`} {
		if !strings.Contains(out, want) {
			t.Fatalf("out = %q, 缺 %q", out, want)
		}
	}
	if NormalizeListmonkTemplateVars("") != "" {
		t.Fatal("空串应原样返回")
	}
}

func TestPreviewEmailSentRecord(t *testing.T) {
	s := newService(t, testFeed)
	post, _ := s.GetLatestPost()

	preview := s.PreviewEmail(post)
	if preview["is_already_sent"] != false {
		t.Fatalf("preview = %#v", preview)
	}
	// 记录已发送后 preview 应标记
	s.saveSentRecord(sentRecord{SentPosts: []string{post.Link}})
	preview = s.PreviewEmail(post)
	if preview["is_already_sent"] != true {
		t.Fatalf("is_already_sent = %#v", preview["is_already_sent"])
	}
}

func TestSendCampaign(t *testing.T) {
	var createBody []byte
	postAuth, putAuth := "", ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/api/campaigns":
			postAuth = r.Header.Get("Authorization")
			var data map[string]any
			json.NewDecoder(r.Body).Decode(&data)
			createBody, _ = json.Marshal(data)
			fmt.Fprint(w, `{"data":{"id":42}}`)
		case r.Method == "PUT" && r.URL.Path == "/api/campaigns/42/status":
			putAuth = r.Header.Get("Authorization")
			fmt.Fprint(w, `{"data":{"id":42}}`)
		default:
			w.WriteHeader(404)
			fmt.Fprint(w, `{"message":"not found"}`)
		}
	}))
	t.Cleanup(srv.Close)

	s := newService(t, testFeed)
	s.apiURL = srv.URL
	post, _ := s.GetLatestPost()

	res := s.SendCampaign(post, false)
	if res["success"] != true {
		t.Fatalf("res = %#v", res)
	}
	if !strings.Contains(string(createBody), `"lists":[1]`) {
		t.Fatalf("campaign 请求体应携带 list id: %s", createBody)
	}
	// 创建与启动两个请求都必须携带 listmonk 凭据
	if postAuth != "token u:k" {
		t.Fatalf("POST Authorization = %q", postAuth)
	}
	if putAuth != "token u:k" {
		t.Fatalf("PUT Authorization = %q", putAuth)
	}

	// 已发送记录落盘，非 force 再次发送被拒
	if !s.isAlreadySent(post) {
		t.Fatal("发送后应记录")
	}
	res = s.SendCampaign(post, false)
	if res["success"] != false {
		t.Fatalf("重复发送应被拒: %#v", res)
	}
	// force 覆盖
	res = s.SendCampaign(post, true)
	if res["success"] != true {
		t.Fatalf("force 应放行: %#v", res)
	}

	// listmonk 返回错误状态 → campaignError 带 error_detail
	s2 := newService(t, testFeed)
	s2.apiURL = srv.URL + "/fail" // 404 分支
	post2, _ := s2.GetPostByURL("https://blog.example.com/posts/second/index.html")
	res = s2.SendCampaign(post2, true)
	if res["success"] != false || res["error_detail"] == "" {
		t.Fatalf("错误响应应带 error_detail: %#v", res)
	}

	// 连接失败 → campaignError(err, nil)
	s3 := newService(t, testFeed)
	s3.apiURL = "http://127.0.0.1:0" // 不可达端口
	post3, _ := s3.GetPostByURL("https://blog.example.com/posts/second/index.html")
	res = s3.SendCampaign(post3, true)
	if res["success"] != false {
		t.Fatalf("连接失败应返回失败: %#v", res)
	}
}

func TestPushAndPreviewLatest(t *testing.T) {
	s := newService(t, testFeed)

	res := s.PreviewLatest()
	if res["success"] != true {
		t.Fatalf("res = %#v", res)
	}

	// debug 模式走 SendDebugEmail（本机无 MailHog 时为失败结果，只验证结构）
	s.debugMode = true
	res = s.PushLatest(true)
	if _, ok := res["success"]; !ok {
		t.Fatalf("res = %#v", res)
	}
	if res["post"] == nil {
		t.Fatal("结果应携带 post")
	}

	// feed 不可达
	bad := newService(t, "not-xml")
	if res := bad.PreviewLatest(); res["success"] != false {
		t.Fatalf("res = %#v", res)
	}
	if res := bad.PushLatest(true); res["success"] != false {
		t.Fatalf("res = %#v", res)
	}
}

func TestPushAndPreviewArticle(t *testing.T) {
	s := newService(t, testFeed)

	res := s.PreviewArticle("https://blog.example.com/posts/second/")
	if res["success"] != true {
		t.Fatalf("res = %#v", res)
	}

	// 未找到文章
	res = s.PushArticle("https://blog.example.com/posts/none/", false)
	if res["success"] != false || !strings.Contains(fmt.Sprint(res["message"]), "未找到文章") {
		t.Fatalf("res = %#v", res)
	}

	// 裸 slug
	res = s.PushArticle("bare-slug", false)
	if res["success"] != false {
		t.Fatalf("res = %#v", res)
	}
}

func TestSendDebugEmailStructure(t *testing.T) {
	s := New(nil, true, t.TempDir())
	post := &Post{Title: "T", Link: "https://blog.example.com/p/", Summary: "s"}
	res := s.SendDebugEmail(post)
	if _, ok := res["success"]; !ok {
		t.Fatalf("res = %#v", res)
	}
	// 本机未运行 MailHog（localhost:1025）时应给出提示
	if res["success"] == false {
		if res["hint"] == nil {
			t.Fatalf("失败时应带 hint: %#v", res)
		}
	}
}

func TestSentRecordPersistence(t *testing.T) {
	home := t.TempDir()
	s := New(nil, false, home)
	r := sentRecord{SentPosts: []string{"a", "b"}, LastSent: map[string]any{"title": "t"}}
	s.saveSentRecord(r)

	// 文件确实写到 ~/.config/latest_post_sent.json
	path := filepath.Join(home, ".config", sentRecordFile)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("sent record 未落盘: %v", err)
	}
	loaded := s.loadSentRecord()
	if len(loaded.SentPosts) != 2 || loaded.SentPosts[1] != "b" {
		t.Fatalf("loaded = %#v", loaded)
	}
	if !s.isAlreadySent(&Post{Link: "a"}) || s.isAlreadySent(&Post{Link: "c"}) {
		t.Fatal("isAlreadySent 判断错误")
	}
}
