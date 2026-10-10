// Package importsvc 对齐 article_import_service.py：
// 外部 .md 导入为 Hugo 草稿——标题派生、唯一目录、AI frontmatter
// 富化（仅填缺）、后台封面生成（SSE article_import.* 事件）。
package importsvc

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/svtter/hugo-admin/internal/aigen"
	"github.com/svtter/hugo-admin/internal/frontmatter"
	"github.com/svtter/hugo-admin/internal/posts"
	"github.com/svtter/hugo-admin/internal/realtime"
	"github.com/svtter/hugo-admin/internal/util"
)

var rePathSeparators = regexp.MustCompile(`[\\/]`)

// AICfg / ImageCfg 对齐同名 dict 参数。
type AICfg struct {
	APIKey  string
	BaseURL string
	Model   string
}

type ImageCfg struct {
	APIKey string
	Model  string
}

// Result 对齐 import_markdown 返回。
type Result struct {
	Path         string   `json:"path"`
	Title        string   `json:"title"`
	Warnings     []string `json:"warnings"`
	CoverPending bool     `json:"cover_pending"`
	EventScope   string   `json:"event_scope"`
}

// Import 对齐 import_markdown：核心导入流程。
// broker 非 nil 时封面走后台任务（SSE 事件），否则同步生成。
func Import(contentDir, filename string, raw []byte, title string,
	genFM, genCover bool, aiCfg AICfg, imgCfg ImageCfg,
	broker *realtime.Broker, eventScope string, now time.Time) Result {

	warnings := []string{}
	text := strings.ReplaceAll(string(raw), "\r\n", "\n") // 近似 decode(errors=replace)

	// 拆分已有 frontmatter / 正文
	var existingFM map[string]any
	body := text
	if doc, err := frontmatter.Parse([]byte(text)); err == nil {
		existingFM = doc.Metadata
		body = doc.Content
	}

	// 派生标题 + 创建唯一目录
	resolvedTitle := deriveTitle(title, existingFM, body, filename)
	articlePath := createArticleDir(contentDir, resolvedTitle, now)

	// 组装 frontmatter（保留已有富化字段，date 用已有或东八区当前）
	fm := map[string]any{
		"title": resolvedTitle,
		"draft": true,
	}
	if date, ok := existingFM["date"].(string); ok && strings.TrimSpace(date) != "" {
		fm["date"] = date
	} else {
		cst := time.FixedZone("CST", 8*3600)
		fm["date"] = now.In(cst).Format("2006-01-02T15:04:05+08:00")
	}
	for _, key := range []string{"description", "categories", "tags", "cover"} {
		if v := existingFM[key]; !isEmpty(v) {
			fm[key] = v
		}
	}

	// AI frontmatter 富化（仅填缺）
	if genFM {
		if aiCfg.APIKey == "" {
			warnings = append(warnings, "未配置 AI API Key，跳过 frontmatter 生成")
		} else {
			ok, result := aigen.GenerateFrontmatter(body, aiCfg.APIKey, aiCfg.BaseURL, aiCfg.Model, now)
			if !ok {
				warnings = append(warnings, fmt.Sprintf("frontmatter 生成失败：%v", result))
			} else if suggested, ok := result.(map[string]any); ok {
				for _, key := range []string{"description", "tags", "categories"} {
					if isEmpty(fm[key]) && !isEmpty(suggested[key]) {
						fm[key] = suggested[key]
					}
				}
			}
		}
	}
	if isEmpty(fm["categories"]) {
		fm["categories"] = []any{}
	}
	if isEmpty(fm["tags"]) {
		fm["tags"] = []any{}
	}

	// 写入文章（草稿，暂无封面）
	writeOK, _, _ := posts.SaveFile(contentDir, articlePath, body, fm, nil)
	if !writeOK {
		return Result{
			Path: "", Title: resolvedTitle,
			Warnings: []string{"写入文件失败"}, CoverPending: false, EventScope: eventScope,
		}
	}

	// 封面：broker 存在时后台生成（SSE 事件），否则同步
	coverPending := false
	if genCover {
		if imgCfg.APIKey == "" {
			warnings = append(warnings, "未配置 OPENROUTER_API_KEY，跳过封面生成")
		} else {
			description, _ := fm["description"].(string)
			if broker != nil && eventScope != "" {
				coverPending = true
				go runCoverWithEmits(contentDir, articlePath, resolvedTitle,
					description, body, imgCfg, broker, eventScope)
			} else {
				ok, result := generateAndAttachCover(contentDir, articlePath,
					resolvedTitle, description, body, imgCfg, now)
				if !ok {
					warnings = append(warnings, fmt.Sprintf("封面生成失败：%v", result))
				}
			}
		}
	}

	return Result{
		Path: articlePath, Title: resolvedTitle, Warnings: warnings,
		CoverPending: coverPending, EventScope: eventScope,
	}
}

func runCoverWithEmits(contentDir, articlePath, title, description, content string,
	imgCfg ImageCfg, broker *realtime.Broker, eventScope string) {

	emit := func(event string, payload map[string]any) {
		full := map[string]any{"scope": eventScope}
		for k, v := range payload {
			full[k] = v
		}
		broker.Broadcast(event, full)
	}

	emit("article_import.progress", map[string]any{"stage": "cover"})
	ok, result := generateAndAttachCover(contentDir, articlePath, title,
		description, content, imgCfg, time.Now())
	if ok {
		emit("article_import.cover_done", map[string]any{"url": result})
	} else {
		emit("article_import.cover_failed", map[string]any{"message": result})
	}
}

func generateAndAttachCover(contentDir, articlePath, title, description, content string,
	imgCfg ImageCfg, now time.Time) (bool, string) {
	ok, result := aigen.GenerateCoverImage(title, description, content, imgCfg.APIKey, imgCfg.Model)
	if !ok {
		return false, fmt.Sprintf("%v", result)
	}
	imageBytes, _ := result.([]byte)

	saveOK, saveResult := aigen.SaveGeneratedImage(articlePath, imageBytes, contentDir, now)
	if !saveOK {
		return false, saveResult
	}

	// 写回 cover 字段
	rOK, body, fm, _ := posts.ReadFileWithFrontmatter(contentDir, articlePath)
	if !rOK {
		return false, "写入封面字段失败"
	}
	fm["cover"] = saveResult
	wOK, _, _ := posts.SaveFile(contentDir, articlePath, body, fm, nil)
	if !wOK {
		return false, "写入封面字段失败"
	}
	return true, saveResult
}

// ---- 工具（对齐私有函数） ----

func isEmpty(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return t == ""
	case []any:
		return len(t) == 0
	case map[string]any:
		return len(t) == 0
	}
	return false
}

func deriveTitle(explicit string, existingFM map[string]any, body, filename string) string {
	if trimmed := strings.TrimSpace(explicit); trimmed != "" {
		return trimmed
	}
	if title, ok := existingFM["title"].(string); ok && strings.TrimSpace(title) != "" {
		return strings.TrimSpace(title)
	}
	for _, line := range strings.Split(body, "\n") {
		stripped := strings.TrimSpace(line)
		if strings.HasPrefix(stripped, "# ") {
			if heading := strings.TrimSpace(stripped[2:]); heading != "" {
				return heading
			}
		}
	}
	stem := strings.TrimSpace(strings.TrimSuffix(filename, filepath.Ext(filename)))
	if stem == "" {
		return "untitled"
	}
	return stem
}

func importSlug(title string) string {
	slug := strings.ReplaceAll(strings.TrimSpace(title), " ", "-")
	slug = rePathSeparators.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, ".")
	if slug == "" {
		return "untitled"
	}
	return slug
}

func createArticleDir(contentDir, title string, now time.Time) string {
	datePrefix := now.Format("2006-01-02")
	slug := importSlug(title)
	name := datePrefix + "-" + slug
	target := filepath.Join(contentDir, "post", name)
	for {
		if _, err := os.Stat(target); os.IsNotExist(err) {
			break
		}
		name = datePrefix + "-" + slug + "-" + util.NewOperationID()[:6]
		target = filepath.Join(contentDir, "post", name)
	}
	_ = os.MkdirAll(target, 0o755)
	return "post/" + name + "/index.md"
}
