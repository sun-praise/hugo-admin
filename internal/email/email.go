// Package email 对齐 services/email_service.py：
// RSS 取最新文章 → 渲染邮件 → listmonk campaign（或 debug 模式 MailHog SMTP）。
package email

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"
)

const (
	rssURL         = "https://svtter.cn/index.xml"
	sentRecordFile = "latest_post_sent.json"
	debugSMTPHost  = "localhost"
	debugSMTPPort  = "1025"
	debugTestEmail = "test@example.com"
)

// Post 是 RSS 条目的规范化形式（对齐 get_latest_post 返回）。
type Post struct {
	Title       string   `json:"title"`
	Link        string   `json:"link"`
	Published   string   `json:"published"`
	Description string   `json:"description"`
	Summary     string   `json:"summary"`
	Tags        []string `json:"tags"`
}

type Service struct {
	rssURL    string
	sentFile  string
	debugMode bool

	apiURL  string
	apiUser string
	apiKey  string
	listID  int
}

// New 从 settings 的 listmonk 配置构造。
func New(listmonkCfg map[string]any, debugMode bool, homeDir string) *Service {
	s := &Service{
		rssURL:    rssURL,
		sentFile:  filepath.Join(homeDir, ".config", sentRecordFile),
		debugMode: debugMode,
		listID:    1,
	}
	// 对齐 configure_from_settings
	if v, ok := listmonkCfg["api_url"].(string); ok {
		s.apiURL = v
	}
	if v, ok := listmonkCfg["api_user"].(string); ok {
		s.apiUser = v
	}
	if v, ok := listmonkCfg["api_key"].(string); ok {
		s.apiKey = v
	}
	switch v := listmonkCfg["blog_list_id"].(type) {
	case int:
		s.listID = v
	case float64:
		s.listID = int(v)
	}
	return s
}

// GetLatestPost 对齐 get_latest_post：RSS 第一条。
func (s *Service) GetLatestPost() (*Post, error) {
	feed, err := gofeed.NewParser().ParseURL(s.rssURL)
	if err != nil {
		return nil, fmt.Errorf("获取 RSS 失败: %v", err)
	}
	if len(feed.Items) == 0 {
		return nil, nil
	}
	return itemToPost(feed.Items[0]), nil
}

// GetPostByURL 对齐 get_post_by_url：URL/路径归一化匹配，多匹配视为未找到。
func (s *Service) GetPostByURL(rawURL string) (*Post, error) {
	netloc, path, ok := normalizeURLForMatch(rawURL)
	if !ok {
		return nil, nil
	}
	feed, err := gofeed.NewParser().ParseURL(s.rssURL)
	if err != nil {
		return nil, fmt.Errorf("获取文章失败: %v", err)
	}
	matches := 0
	var matched *gofeed.Item
	for _, item := range feed.Items {
		entryURL, err := url.Parse(item.Link)
		if err != nil {
			continue
		}
		entryNetloc := strings.ToLower(entryURL.Host)
		entryNetloc = strings.TrimPrefix(entryNetloc, "www.")
		entryPath := stripHTMLSuffix(unquote(entryURL.EscapedPath()))
		entryPath = strings.TrimRight(entryPath, "/")
		netlocMatch := netloc == "" || netloc == entryNetloc
		if netlocMatch && path == entryPath {
			matches++
			if matches > 1 {
				return nil, nil
			}
			matched = item
		}
	}
	if matches == 1 {
		return itemToPost(matched), nil
	}
	return nil, nil
}

func itemToPost(item *gofeed.Item) *Post {
	p := &Post{
		Title:   item.Title,
		Link:    item.Link,
		Summary: item.Description,
		Tags:    []string{},
	}
	if item.PublishedParsed != nil {
		p.Published = item.PublishedParsed.Format("Mon, 02 Jan 2006 15:04:05 -0700")
	} else if item.Published != "" {
		p.Published = item.Published
	}
	if item.Description != "" {
		p.Summary = item.Description
	} else if item.Content != "" {
		p.Summary = item.Content
	}
	for _, tag := range item.Categories {
		p.Tags = append(p.Tags, tag)
	}
	return p
}

// ---- URL 归一化（对齐 _normalize_url_for_match / _strip_html_suffix） ----

func stripHTMLSuffix(path string) string {
	switch {
	case strings.HasSuffix(path, "/index.html"):
		return path[:len(path)-len("/index.html")]
	case strings.HasSuffix(path, "/index.htm"):
		return path[:len(path)-len("/index.htm")]
	case strings.HasSuffix(path, ".html"):
		return path[:len(path)-len(".html")]
	case strings.HasSuffix(path, ".htm"):
		return path[:len(path)-len(".htm")]
	}
	return path
}

func unquote(s string) string {
	if out, err := url.QueryUnescape(s); err == nil {
		return out
	}
	return s
}

// normalizeURLForMatch 返回 (netloc, path, ok)。裸 slug 拒绝。
func normalizeURLForMatch(rawURL string) (string, string, bool) {
	u := strings.TrimSpace(rawURL)
	if u == "" {
		return "", "", false
	}
	// 裸 slug（无 / 无 .）→ 拒绝
	if !strings.Contains(u, "/") && !strings.Contains(u, ".") {
		return "", "", false
	}
	if strings.HasPrefix(u, "/") {
		return "", stripHTMLSuffix(unquote(strings.TrimRight(u, "/"))), true
	}
	if !strings.Contains(u, "://") {
		u = "https://" + u
	}
	parsed, err := url.Parse(u)
	if err != nil {
		return "", "", false
	}
	netloc := strings.ToLower(parsed.Host)
	netloc = strings.TrimPrefix(netloc, "www.")
	return netloc, stripHTMLSuffix(unquote(strings.TrimRight(parsed.EscapedPath(), "/"))), true
}

// ---- 已发送记录 ----

type sentRecord struct {
	SentPosts []string       `json:"sent_posts"`
	LastSent  map[string]any `json:"last_sent"`
}

func (s *Service) loadSentRecord() sentRecord {
	var r sentRecord
	if raw, err := os.ReadFile(s.sentFile); err == nil {
		_ = json.Unmarshal(raw, &r)
	}
	return r
}

func (s *Service) saveSentRecord(r sentRecord) {
	_ = os.MkdirAll(filepath.Dir(s.sentFile), 0o755)
	data, _ := json.MarshalIndent(r, "", "  ")
	_ = os.WriteFile(s.sentFile, data, 0o644)
}

func (s *Service) isAlreadySent(post *Post) bool {
	r := s.loadSentRecord()
	for _, link := range r.SentPosts {
		if link == post.Link {
			return true
		}
	}
	return false
}

// ---- 邮件内容 ----

var reHTMLTag = regexp.MustCompile(`<[^>]+>`)

// CreateEmailContent 对齐 create_email_content（同款 HTML 模板）。
func (s *Service) CreateEmailContent(post *Post) (string, string) {
	subject := fmt.Sprintf("📖 新文章发布：%s", post.Title)

	summary := post.Summary
	if summary == "" {
		summary = post.Description
	}
	cleanSummary := reHTMLTag.ReplaceAllString(summary, "")
	runes := []rune(cleanSummary)
	if len(runes) > 400 {
		cleanSummary = string(runes[:400]) + "..."
	}

	categoryHTML := ""
	if len(post.Tags) > 0 {
		var tags strings.Builder
		limit := 3
		if len(post.Tags) < limit {
			limit = len(post.Tags)
		}
		for _, tag := range post.Tags[:limit] {
			fmt.Fprintf(&tags, `<span style="background: #e3f2fd; color: #1976d2; padding: 2px 8px; border-radius: 12px; font-size: 12px; margin-right: 5px;">#%s</span>`, tag)
		}
		categoryHTML = fmt.Sprintf(`<div style="margin-bottom: 15px;">%s</div>`, tags.String())
	}

	body := fmt.Sprintf(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>%s</title>
</head>
<body style="margin: 0; padding: 0; background-color: #f5f5f5; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;">
    <div style="max-width: 600px; margin: 20px auto; background: white; border-radius: 12px; box-shadow: 0 4px 12px rgba(0,0,0,0.1); overflow: hidden;">
        <div style="background: linear-gradient(135deg, #007cba 0%%, #0056b3 100%%); padding: 30px; text-align: center;">
            <h1 style="color: white; margin: 0; font-size: 24px; font-weight: 600;">📖 Svtter's Blog</h1>
            <p style="color: rgba(255,255,255,0.9); margin: 8px 0 0 0; font-size: 14px;">有新文章发布啦！</p>
        </div>
        <div style="padding: 30px;">
            <div style="border-left: 4px solid #007cba; padding-left: 20px; margin-bottom: 25px;">
                <h2 style="color: #333; margin: 0 0 10px 0; font-size: 22px; line-height: 1.4;">%s</h2>
                <p style="color: #666; font-size: 14px; margin: 0;">
                    📅 %s &nbsp;&nbsp;
                    👁️ <a href="https://svtter.cn" style="color: #007cba; text-decoration: none;">svtter.cn</a>
                </p>
            </div>
            %s
            <div style="background: #f8f9fa; padding: 25px; border-radius: 8px; border: 1px solid #e9ecef; margin: 20px 0;">
                <div style="color: #333; line-height: 1.7; font-size: 15px;">
                    %s
                </div>
            </div>
            <div style="text-align: center; margin: 35px 0;">
                <a href="%s" style="display: inline-block; background: linear-gradient(135deg, #007cba 0%%, #0056b3 100%%); color: white; padding: 15px 35px; text-decoration: none; border-radius: 25px; font-weight: 600; font-size: 16px; box-shadow: 0 3px 10px rgba(0, 124, 186, 0.3); transition: all 0.3s ease;">
                    📖 立即阅读全文
                </a>
            </div>
            <div style="background: #e8f4fd; padding: 20px; border-radius: 8px; margin-top: 30px;">
                <p style="color: #1565c0; font-size: 14px; margin: 0; text-align: center;">
                    💡 喜欢这篇文章？别忘了分享给朋友们！
                </p>
            </div>
        </div>
        <div style="background: #f8f9fa; padding: 25px; border-top: 1px solid #e9ecef; text-align: center;">
            <p style="color: #666; font-size: 13px; margin: 0 0 10px 0;">
                您收到此邮件是因为您订阅了 <a href="https://svtter.cn" style="color: #007cba; text-decoration: none;">Svtter's Blog</a>
            </p>
            <p style="color: #999; font-size: 12px; margin: 0;">
                <a href="%s/subscription/form" style="color: #999; text-decoration: none;">管理订阅</a> |
                <a href="https://svtter.cn" style="color: #999; text-decoration: none;">访问博客</a>
            </p>
        </div>
    </div>
    <div style="height: 20px;"></div>
</body>
</html>`, subject, post.Title, post.Published, categoryHTML, cleanSummary, post.Link, s.apiURL)

	return subject, body
}

// NormalizeListmonkTemplateVars 对齐同名方法。
func NormalizeListmonkTemplateVars(content string) string {
	if content == "" {
		return content
	}
	replacements := [][2]string{
		{`{{\s*\.UnsubscribeURL\s*}}`, "{{ UnsubscribeURL }}"},
		{`{{\s*\.MessageURL\s*}}`, "{{ MessageURL }}"},
		{`{{\s*\.TrackView\s*}}`, "{{ TrackView }}"},
	}
	for _, r := range replacements {
		content = regexp.MustCompile(r[0]).ReplaceAllString(content, r[1])
	}
	content = regexp.MustCompile(`{{\s*\.TrackLink(\s+[^}]*)}}`).ReplaceAllString(content, "{{ TrackLink$1}}")
	return content
}

// PreviewEmail 对齐 preview_email。
func (s *Service) PreviewEmail(post *Post) map[string]any {
	subject, body := s.CreateEmailContent(post)
	return map[string]any{
		"subject": subject, "body_html": body,
		"post": post, "is_already_sent": s.isAlreadySent(post),
		"debug_mode": s.debugMode, "target_list_id": s.listID,
	}
}

// SendDebugEmail 对齐 send_debug_email（MailHog SMTP）。
func (s *Service) SendDebugEmail(post *Post) map[string]any {
	subject, body := s.CreateEmailContent(post)
	body = NormalizeListmonkTemplateVars(body)

	msg := fmt.Sprintf("From: debug@svtter.cn\r\nTo: %s\r\nSubject: [DEBUG] %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n%s",
		debugTestEmail, subject, body)
	err := smtp.SendMail(debugSMTPHost+":"+debugSMTPPort, nil, "debug@svtter.cn", []string{debugTestEmail}, []byte(msg))
	if err != nil {
		return map[string]any{
			"success": false, "message": fmt.Sprintf("调试邮件发送失败: %v", err),
			"hint": "请确保 MailHog 容器正在运行：docker-compose up -d",
		}
	}
	return map[string]any{
		"success": true, "message": "调试邮件发送成功",
		"details": map[string]any{
			"subject": subject, "title": post.Title, "to": debugTestEmail,
			"mailhog_url": "http://localhost:8025",
		},
	}
}

// SendCampaign 对齐 send_campaign：listmonk 创建活动并启动。
func (s *Service) SendCampaign(post *Post, force bool) map[string]any {
	if !force && s.isAlreadySent(post) {
		return map[string]any{
			"success": false,
			"message": fmt.Sprintf("文章已发送过: %s", post.Title),
			"hint":    "如需强制发送，请设置 force=true",
		}
	}

	subject, body := s.CreateEmailContent(post)
	campaignName := fmt.Sprintf("📖 %s - %s", post.Title, time.Now().Format("20060102_1504"))
	campaignData := map[string]any{
		"name": campaignName, "subject": subject,
		"lists": []int{s.listID}, "type": "regular",
		"content_type": "html", "body": body,
		"tags": []string{"blog", "latest", "auto-send"},
	}

	client := &http.Client{Timeout: 30 * time.Second}
	auth := fmt.Sprintf("token %s:%s", s.apiUser, s.apiKey)

	createJSON, _ := json.Marshal(campaignData)
	createResp, err := client.Post(s.apiURL+"/api/campaigns", "application/json",
		bytes.NewReader(createJSON))
	if err != nil {
		return s.campaignError(err, createResp)
	}
	if createResp.StatusCode >= 300 {
		return s.campaignError(fmt.Errorf("listmonk 返回 %d", createResp.StatusCode), createResp)
	}
	var created struct {
		Data struct {
			ID int `json:"id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(createResp.Body).Decode(&created)
	createResp.Body.Close()
	campaignID := created.Data.ID

	// 启动活动
	sendJSON, _ := json.Marshal(map[string]any{"status": "running"})
	sendReq, _ := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/api/campaigns/%d/status", s.apiURL, campaignID),
		bytes.NewReader(sendJSON))
	sendReq.Header.Set("Authorization", auth)
	sendReq.Header.Set("Content-Type", "application/json")
	sendResp, err := client.Do(sendReq)
	if err != nil {
		return s.campaignError(err, sendResp)
	}
	if sendResp.StatusCode >= 300 {
		return s.campaignError(fmt.Errorf("listmonk 返回 %d", sendResp.StatusCode), sendResp)
	}
	sendResp.Body.Close()

	// 记录已发送
	record := s.loadSentRecord()
	record.SentPosts = append(record.SentPosts, post.Link)
	record.LastSent = map[string]any{
		"title": post.Title, "link": post.Link,
		"sent_time": time.Now().Format(time.RFC3339), "campaign_id": campaignID,
	}
	s.saveSentRecord(record)

	return map[string]any{
		"success": true, "message": "邮件发送成功",
		"details": map[string]any{
			"campaign_name": campaignName, "subject": subject,
			"campaign_id": campaignID, "list_id": s.listID,
		},
	}
}

func (s *Service) campaignError(err error, resp *http.Response) map[string]any {
	detail := ""
	if resp != nil {
		buf := make([]byte, 512)
		n, _ := resp.Body.Read(buf)
		resp.Body.Close()
		detail = string(buf[:n])
	}
	return map[string]any{
		"success": false, "message": fmt.Sprintf("发送失败: %v", err),
		"error_detail": detail,
	}
}

// PushLatest / PushArticle / PreviewLatest / PreviewArticle 对齐同名方法。
func (s *Service) PushLatest(force bool) map[string]any {
	post, err := s.GetLatestPost()
	if err != nil {
		return map[string]any{"success": false, "message": err.Error()}
	}
	if post == nil {
		return map[string]any{"success": false, "message": "无法获取最新文章"}
	}
	var result map[string]any
	if s.debugMode {
		result = s.SendDebugEmail(post)
	} else {
		result = s.SendCampaign(post, force)
	}
	result["post"] = post
	return result
}

func (s *Service) PushArticle(rawURL string, force bool) map[string]any {
	post, err := s.GetPostByURL(rawURL)
	if err != nil {
		return map[string]any{"success": false, "message": err.Error()}
	}
	if post == nil {
		return map[string]any{"success": false, "message": fmt.Sprintf("未找到文章: %s", rawURL)}
	}
	var result map[string]any
	if s.debugMode {
		result = s.SendDebugEmail(post)
	} else {
		result = s.SendCampaign(post, force)
	}
	result["post"] = post
	return result
}

func (s *Service) PreviewLatest() map[string]any {
	post, err := s.GetLatestPost()
	if err != nil {
		return map[string]any{"success": false, "message": err.Error()}
	}
	if post == nil {
		return map[string]any{"success": false, "message": "无法获取最新文章"}
	}
	return map[string]any{
		"success": true, "message": "预览生成成功", "data": s.PreviewEmail(post),
	}
}

func (s *Service) PreviewArticle(rawURL string) map[string]any {
	post, err := s.GetPostByURL(rawURL)
	if err != nil {
		return map[string]any{"success": false, "message": err.Error()}
	}
	if post == nil {
		return map[string]any{"success": false, "message": fmt.Sprintf("未找到文章: %s", rawURL)}
	}
	return map[string]any{
		"success": true, "message": "预览生成成功", "data": s.PreviewEmail(post),
	}
}
