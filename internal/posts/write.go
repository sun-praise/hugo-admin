package posts

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/svtter/hugo-admin/internal/frontmatter"
)

// 写域：对齐 services/post_service.py 的 read_file /
// read_file_with_frontmatter / save_file / create_post。

// unmarshalYAMLOrEmpty 解析 YAML 到 map；失败或非 dict 时回退空 map
// （对齐 Python 的 isinstance 检查）。
func unmarshalYAMLOrEmpty(s string, out *map[string]any) error {
	if err := yaml.Unmarshal([]byte(s), out); err != nil || *out == nil {
		*out = map[string]any{}
		if err != nil {
			return err
		}
	}
	return nil
}

// ConflictInfo 是 save 乐观锁冲突时的负载（HTTP 409）。
type ConflictInfo struct {
	Conflict       bool    `json:"conflict"`
	CurrentContent string  `json:"current_content"`
	CurrentMTime   float64 `json:"current_mtime"`
	Message        string  `json:"message"`
}

// resolvePath 近似 Python Path.resolve()（非 strict）：解析存在的部分的
// 符号链接，不存在的尾段保留。
func resolvePath(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return filepath.Clean(p)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	parent := filepath.Dir(abs)
	if resolvedParent, err := filepath.EvalSymlinks(parent); err == nil {
		return filepath.Join(resolvedParent, filepath.Base(abs))
	}
	return filepath.Clean(abs)
}

// isSafePath 判断解析后的路径位于 content 目录下（含自身）。
func isSafePath(contentDir, p string) bool {
	if !filepath.IsAbs(p) {
		p = filepath.Join(contentDir, p)
	}
	rp, rc := resolvePath(p), resolvePath(contentDir)
	return rp == rc || strings.HasPrefix(rp, rc+string(filepath.Separator))
}

// joinContent 相对路径拼到 content 目录下，绝对路径原样返回。
func joinContent(contentDir, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(contentDir, p)
}

func fileMTime(path string) (float64, error) {
	st, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return float64(st.ModTime().UnixNano()) / 1e9, nil
}

// ReadFile 对齐 read_file：返回 (ok, 内容或错误消息, mtime)。
func ReadFile(contentDir, filePath string) (bool, string, float64) {
	abs := joinContent(contentDir, filePath)
	if !isSafePath(contentDir, abs) {
		return false, "访问被拒绝:文件不在允许的目录中", 0
	}
	data, err := os.ReadFile(abs)
	if os.IsNotExist(err) {
		return false, fmt.Sprintf("文件不存在: %s", abs), 0
	}
	if err != nil {
		return false, fmt.Sprintf("读取文件失败: %v", err), 0
	}
	mtime, err := fileMTime(abs)
	if err != nil {
		return false, fmt.Sprintf("读取文件失败: %v", err), 0
	}
	return true, string(data), mtime
}

// ReadFileWithFrontmatter 对齐 read_file_with_frontmatter：按行找
// 首对 '---' 切分，YAML 解析失败回退空 dict，正文再剥重复 frontmatter。
func ReadFileWithFrontmatter(contentDir, filePath string) (bool, string, map[string]any, float64) {
	abs := joinContent(contentDir, filePath)
	if !isSafePath(contentDir, abs) {
		return false, "访问被拒绝:文件不在允许的目录中", map[string]any{}, 0
	}
	data, err := os.ReadFile(abs)
	if os.IsNotExist(err) {
		return false, fmt.Sprintf("文件不存在: %s", abs), map[string]any{}, 0
	}
	if err != nil {
		return false, fmt.Sprintf("读取文件失败: %v", err), map[string]any{}, 0
	}
	mtime, err := fileMTime(abs)
	if err != nil {
		return false, fmt.Sprintf("读取文件失败: %v", err), map[string]any{}, 0
	}

	text := string(data)
	lines := strings.Split(text, "\n")
	metadata := map[string]any{}
	body := text
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		for i := 1; i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) == "---" {
				fmText := strings.Join(lines[1:i], "\n")
				body = strings.Join(lines[i+1:], "\n")
				_ = unmarshalYAMLOrEmpty(fmText, &metadata)
				break
			}
		}
	}
	body = StripLeadingFrontmatter(body)

	// normalized：非基础类型值转字符串（对齐 Python 实现）
	normalized := make(map[string]any, len(metadata))
	for k, v := range metadata {
		switch v.(type) {
		case string, int, float64, bool, []any, nil:
			normalized[k] = v
		default:
			normalized[k] = fmt.Sprintf("%v", v)
		}
	}
	return true, body, normalized, mtime
}

// SaveFile 对齐 save_file。expectedMtime 非 nil 时做乐观锁校验，
// 冲突返回 ConflictInfo（调用方映射为 HTTP 409）。
func SaveFile(contentDir, filePath, content string, fmData map[string]any, expectedMtime *float64) (bool, any, float64) {
	abs := joinContent(contentDir, filePath)
	if !isSafePath(contentDir, abs) {
		return false, "访问被拒绝:文件不在允许的目录中", 0
	}

	if expectedMtime != nil {
		if current, err := fileMTime(abs); err == nil {
			if diff := current - *expectedMtime; diff > 0.001 || diff < -0.001 {
				currentContent, _ := os.ReadFile(abs)
				return false, ConflictInfo{
					Conflict:       true,
					CurrentContent: string(currentContent),
					CurrentMTime:   current,
					Message:        "文件已被其他人修改",
				}, 0
			}
		}
	}

	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return false, fmt.Sprintf("保存文件失败: %v", err), 0
	}

	var fileContent string
	if len(fmData) > 0 {
		doc := &frontmatter.Document{
			Metadata: fmData,
			Content:  StripLeadingFrontmatter(content),
		}
		fileContent = string(doc.Dump())
	} else {
		fileContent = content
	}

	if err := os.WriteFile(abs, []byte(fileContent), 0o644); err != nil {
		return false, fmt.Sprintf("保存文件失败: %v", err), 0
	}
	newMtime, err := fileMTime(abs)
	if err != nil {
		return false, fmt.Sprintf("保存文件失败: %v", err), 0
	}
	return true, "文件保存成功", newMtime
}

// CreatePost 对齐 create_post：slugify 标题、当日目录、手工构造
// frontmatter（东八区时间）、返回相对路径。
func CreatePost(contentDir, title string) (bool, string) {
	slug := SlugifyTitle(title)
	postName := time.Now().Format("2006-01-02") + "-" + slug

	postDir := filepath.Join(contentDir, "post", postName)
	if err := os.MkdirAll(postDir, 0o755); err != nil {
		return false, fmt.Sprintf("创建文章失败: %v", err)
	}
	if !isSafePath(contentDir, postDir) {
		os.Remove(postDir)
		return false, "创建文章失败: 生成的路径超出内容目录"
	}

	cst := time.FixedZone("CST", 8*3600)
	dateStr := time.Now().In(cst).Format("2006-01-02T15:04:05+08:00")

	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "title: %s\n", title)
	fmt.Fprintf(&b, "date: %s\n", dateStr)
	b.WriteString("draft: true\n")
	b.WriteString("categories: []\n")
	b.WriteString("tags: []\n")
	b.WriteString("---\n\n")
	b.WriteString("在这里编写你的文章内容...\n")

	postFile := filepath.Join(postDir, "index.md")
	if err := os.WriteFile(postFile, []byte(b.String()), 0o644); err != nil {
		return false, fmt.Sprintf("创建文章失败: %v", err)
	}
	rel, err := filepath.Rel(contentDir, postFile)
	if err != nil {
		return false, fmt.Sprintf("创建文章失败: %v", err)
	}
	return true, filepath.ToSlash(rel)
}

var (
	reUnsafeSlug = regexp.MustCompile(`[^\p{L}\p{N}_\x{4e00}-\x{9fa5}-]`)
	reDashRuns   = regexp.MustCompile(`-+`)
)

// SlugifyTitle 对齐 _slugify_title：保留中英文/数字/下划线/连字符，
// 其余替换为连字符并合并，空结果回退 untitled。
func SlugifyTitle(title string) string {
	slug := reUnsafeSlug.ReplaceAllString(title, "-")
	slug = reDashRuns.ReplaceAllString(slug, "-")
	slug = strings.Trim(strings.TrimSpace(strings.Trim(slug, "-")), "-")
	if slug == "" {
		return "untitled"
	}
	return slug
}

// StripLeadingFrontmatter 对齐 _strip_leading_frontmatter：反复剥除
// 开头连续的 '---' 块与正文前空行，防止保存拼接出双重 frontmatter。
func StripLeadingFrontmatter(content string) string {
	if content == "" {
		return content
	}
	for {
		trimmed := strings.TrimLeft(content, " \t\r\n")
		if !strings.HasPrefix(trimmed, "---") {
			break
		}
		lines := strings.Split(content, "\n")
		first := -1
		for i, ln := range lines {
			if strings.TrimSpace(ln) == "---" {
				first = i
				break
			}
		}
		if first < 0 {
			return content
		}
		closed := false
		for i := first + 1; i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) == "---" {
				body := lines[i+1:]
				for len(body) > 0 && strings.TrimSpace(body[0]) == "" {
					body = body[1:]
				}
				content = strings.Join(body, "\n")
				closed = true
				break
			}
		}
		if !closed {
			return content
		}
	}
	return strings.TrimLeft(content, "\r\n")
}
