// Package aigen 提供 AI 生成类服务：frontmatter 建议与封面图。
// 对齐 frontmatter_gen_service.py 与 image_gen_service.py
// （均直连 OpenAI/OpenRouter 兼容的 chat/completions 端点）。
package aigen

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	maxTags         = 5
	maxCategories   = 2
	contentSnipLen  = 2000
	fmCacheTTL      = 30 * time.Minute
	openRouterURL   = "https://openrouter.ai/api/v1/chat/completions"
	defaultImgModel = "google/gemini-3.1-flash-image-preview"
)

// ---- frontmatter 生成 ----

var (
	fmCacheMu sync.Mutex
	fmCache   = map[string]fmCacheEntry{}
)

type fmCacheEntry struct {
	at time.Time
	fm map[string]any
}

// GenerateFrontmatter 对齐 generate_frontmatter：AI 生成 description/tags/
// categories 建议（截断/数量约束/30 分钟缓存）。
func GenerateFrontmatter(content, apiKey, baseURL, model string, now time.Time) (bool, any) {
	if apiKey == "" {
		return false, "AI API Key 未配置"
	}
	runes := []rune(strings.TrimSpace(content))
	if len(runes) > contentSnipLen {
		runes = runes[:contentSnipLen]
	}
	snippet := string(runes)
	if snippet == "" {
		return false, "文章内容为空"
	}

	cacheKey := fmt.Sprintf("%x", hashString(snippet))
	fmCacheMu.Lock()
	if cached, ok := fmCache[cacheKey]; ok && now.Sub(cached.at) < fmCacheTTL {
		fm := cached.fm
		fmCacheMu.Unlock()
		return true, fm
	}
	fmCacheMu.Unlock()

	prompt := "根据以下文章内容，生成合适的 Hugo frontmatter 字段。\n" +
		"只返回一个 JSON 对象，包含以下键（均为可选）：\n" +
		"- \"description\": 文章摘要，中文，1-2 句话 (string)\n" +
		"- \"tags\": 相关标签数组 (array of strings, 最多 5 个)\n" +
		"- \"categories\": 分类数组 (array of strings, 最多 2 个)\n\n" +
		"文章内容:\n---\n" + snippet + "\n---\n\n" +
		"只返回 JSON 对象，不要 markdown 代码块，不要解释。"

	text, ok := chatCompletion(apiKey, baseURL, model, prompt, 0.3, 60*time.Second)
	if !ok {
		return false, "生成失败，请稍后重试"
	}
	// 剥可能的 markdown 代码块包裹
	if strings.HasPrefix(text, "```") {
		lines := strings.Split(text, "\n")
		inner := strings.Join(lines[1:], "\n")
		if idx := strings.LastIndex(inner, "```"); idx >= 0 {
			inner = inner[:idx]
		}
		text = strings.TrimSpace(inner)
	}

	var raw map[string]any
	if err := json.Unmarshal([]byte(text), &raw); err != nil || raw == nil {
		return false, "AI 返回了无效的 JSON"
	}
	result := SanitizeFrontmatter(raw)
	fmCacheMu.Lock()
	fmCache[cacheKey] = fmCacheEntry{at: now, fm: result}
	fmCacheMu.Unlock()
	return true, result
}

// SanitizeFrontmatter 对齐 _sanitize_frontmatter。
func SanitizeFrontmatter(fm map[string]any) map[string]any {
	result := map[string]any{}
	if desc, ok := fm["description"].(string); ok {
		if trimmed := strings.TrimSpace(desc); trimmed != "" {
			runes := []rune(trimmed)
			if len(runes) > 500 {
				trimmed = string(runes[:500])
			}
			result["description"] = trimmed
		}
	}
	if tags, ok := fm["tags"].([]any); ok {
		out := []string{}
		for _, t := range tags {
			if s, ok := t.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
			if len(out) >= maxTags {
				break
			}
		}
		if len(out) > 0 {
			result["tags"] = out
		}
	}
	if cats, ok := fm["categories"].([]any); ok {
		out := []string{}
		for _, c := range cats {
			if s, ok := c.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
			if len(out) >= maxCategories {
				break
			}
		}
		if len(out) > 0 {
			result["categories"] = out
		}
	}
	return result
}

func hashString(s string) uint64 {
	var h uint64 = 14695981039346656037
	for _, b := range []byte(s) {
		h ^= uint64(b)
		h *= 1099511628211
	}
	return h
}

// ---- OpenAI 兼容 chat/completions ----

func chatCompletion(apiKey, baseURL, model, prompt string, temperature float64, timeout time.Duration) (string, bool) {
	url := strings.TrimRight(baseURL, "/") + "/chat/completions"
	payload, _ := json.Marshal(map[string]any{
		"model":       model,
		"messages":    []map[string]any{{"role": "user", "content": prompt}},
		"temperature": temperature,
	})
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", false
	}
	var data struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil || len(data.Choices) == 0 {
		return "", false
	}
	return strings.TrimSpace(data.Choices[0].Message.Content), true
}

// ---- 封面图生成 ----

// GenerateCoverImage 对齐 generate_cover_image：OpenRouter 图像模型，
// 从 images 数组 / content parts / data URL 文本三种形态提取图片字节。
func GenerateCoverImage(title, description, content, apiKey, model string) (bool, any) {
	if apiKey == "" {
		apiKey = os.Getenv("OPENROUTER_API_KEY")
		if apiKey == "" {
			return false, "OPENROUTER_API_KEY 未配置"
		}
	}
	if model == "" {
		model = defaultImgModel
	}

	promptParts := []string{
		"Generate a clean, visually appealing cover image for a blog post.",
		"Title: " + title,
	}
	if description != "" {
		promptParts = append(promptParts, "Description: "+description)
	}
	if content != "" {
		runes := []rune(strings.TrimSpace(content))
		if len(runes) > 500 {
			runes = runes[:500]
		}
		promptParts = append(promptParts, "Content summary: "+string(runes))
	}
	promptParts = append(promptParts,
		"Style: modern, minimalist, elegant. No text overlay. "+
			"Use warm editorial tones. Aspect ratio 16:9.")

	payload, _ := json.Marshal(map[string]any{
		"model":    model,
		"messages": []map[string]any{{"role": "user", "content": strings.Join(promptParts, "\n")}},
	})
	req, _ := http.NewRequest(http.MethodPost, openRouterURL, bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 180 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		if os.IsTimeout(err) {
			return false, "Image generation timed out (180s)"
		}
		return false, fmt.Sprintf("Image generation failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if resp.StatusCode >= 300 {
		return false, fmt.Sprintf("API error %d: %s", resp.StatusCode, string(body[:min(len(body), 200)]))
	}

	var data struct {
		Error   any `json:"error"`
		Choices []struct {
			Message struct {
				Images []struct {
					Type     string `json:"type"`
					ImageURL struct {
						URL string `json:"url"`
					} `json:"image_url"`
				} `json:"images"`
				Content any `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return false, "Image generation failed: invalid response"
	}
	if data.Error != nil {
		return false, fmt.Sprintf("API error: %v", data.Error)
	}
	if len(data.Choices) == 0 {
		return false, "API returned no choices"
	}
	msg := data.Choices[0].Message

	// 1. images 数组
	for _, img := range msg.Images {
		if img.Type == "image_url" {
			if b, ok := extractImageFromURL(img.ImageURL.URL); ok {
				return true, b
			}
		}
	}
	// 2. content parts
	if parts, ok := msg.Content.([]any); ok {
		for _, p := range parts {
			part, ok := p.(map[string]any)
			if !ok || part["type"] != "image_url" {
				continue
			}
			imgURL, _ := part["image_url"].(map[string]any)
			url, _ := imgURL["url"].(string)
			if b, ok := extractImageFromURL(url); ok {
				return true, b
			}
		}
	}
	// 3. content 文本中的 data URL
	if contentStr, ok := msg.Content.(string); ok && strings.Contains(contentStr, "data:image") {
		re := regexp.MustCompile(`data:image/[^;]+;base64,([A-Za-z0-9+/=]+)`)
		if m := re.FindStringSubmatch(contentStr); m != nil {
			if b, err := base64.StdEncoding.DecodeString(m[1]); err == nil {
				return true, b
			}
		}
	}
	return false, "No image found in API response"
}

func extractImageFromURL(url string) ([]byte, bool) {
	if strings.HasPrefix(url, "data:") {
		if idx := strings.Index(url, ","); idx > 0 {
			if b, err := base64.StdEncoding.DecodeString(url[idx+1:]); err == nil {
				return b, true
			}
		}
		return nil, false
	}
	if strings.HasPrefix(url, "http") {
		client := &http.Client{Timeout: 60 * time.Second}
		resp, err := client.Get(url)
		if err != nil {
			return nil, false
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			return nil, false
		}
		b, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
		if err != nil {
			return nil, false
		}
		return b, true
	}
	return nil, false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// SaveGeneratedImage 对齐 save_generated_image：存到文章旁 pics/cover_<ts>.png。
func SaveGeneratedImage(articlePath string, imageBytes []byte, contentDir string, now time.Time) (bool, string) {
	articleFile := articlePath
	if !filepath.IsAbs(articleFile) {
		articleFile = filepath.Join(contentDir, articleFile)
	}
	picsDir := filepath.Join(filepath.Dir(articleFile), "pics")
	if err := os.MkdirAll(picsDir, 0o755); err != nil {
		return false, fmt.Sprintf("Failed to save image: %v", err)
	}
	filename := fmt.Sprintf("cover_%d.png", now.Unix())
	if err := os.WriteFile(filepath.Join(picsDir, filename), imageBytes, 0o644); err != nil {
		return false, fmt.Sprintf("Failed to save image: %v", err)
	}
	return true, "pics/" + filename
}

// ExtractImageFromURL 暴露内部工具供测试（data:/http: 图片提取）。
func ExtractImageFromURL(url string) ([]byte, bool) { return extractImageFromURL(url) }
