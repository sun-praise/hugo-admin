package posts

// 图片域：对齐 post_service 的 save_image / list_images。
// Python 侧的上传优先走 gRPC 插件（image_upload 能力，如 R2 CDN），
// 插件系统批次迁移前 Go 侧直接本地保存（行为差异：图片落在文章
// 旁 pics/ 而非 CDN）。

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

const (
	MaxImageSize = 20 * 1024 * 1024 // 20 MB
)

var allowedImageExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true,
}

// SaveImage 对齐 save_image：存到文章旁 pics/，同名追加短 UUID，
// 原子写入（tmp + rename），返回相对 URL "pics/<name>"。
func SaveImage(contentDir, articlePath, filename string, data []byte) (bool, string) {
	articleFile := joinContent(contentDir, articlePath)
	picsDir := filepath.Join(filepath.Dir(articleFile), "pics")
	if err := os.MkdirAll(picsDir, 0o755); err != nil {
		return false, fmt.Sprintf("保存图片失败: %v", err)
	}

	if len(data) > MaxImageSize {
		return false, fmt.Sprintf("文件大小超出限制 (最大 %dMB)", MaxImageSize/(1024*1024))
	}

	// 安全文件名：仅保留 Unicode 字母数字与 .-_
	// （Python str.isalnum() 对中文等返回 True，文件名保留中文）
	safe := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) ||
			r == '.' || r == '-' || r == '_' {
			return r
		}
		return -1
	}, filename)
	if safe == "" || filepath.Ext(safe) == "" {
		safe = "image.png"
	}
	ext := strings.ToLower(filepath.Ext(safe))
	if !allowedImageExtensions[ext] {
		return false, fmt.Sprintf("不支持的文件类型: %s", ext)
	}

	// 同名文件已存在则追加短 UUID（最多重试 10 次）
	stem := strings.TrimSuffix(safe, filepath.Ext(safe))
	target := filepath.Join(picsDir, safe)
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(target); os.IsNotExist(err) {
			break
		}
		suffix := randomHex(4)
		safe = stem + "_" + suffix + ext
		target = filepath.Join(picsDir, safe)
		if i == 9 {
			if _, err := os.Stat(target); err == nil {
				return false, "无法生成唯一文件名，请重试"
			}
		}
	}

	tmp, err := os.CreateTemp(picsDir, "*.tmp")
	if err != nil {
		return false, fmt.Sprintf("保存图片失败: %v", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return false, fmt.Sprintf("保存图片失败: %v", err)
	}
	if err := tmp.Close(); err != nil {
		return false, fmt.Sprintf("保存图片失败: %v", err)
	}
	if err := os.Rename(tmpName, target); err != nil {
		return false, fmt.Sprintf("保存图片失败: %v", err)
	}
	if err := os.Chmod(target, 0o644); err != nil {
		return false, fmt.Sprintf("保存图片失败: %v", err)
	}
	return true, "pics/" + safe
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ImageEntry 对齐 list_images 的条目。
type ImageEntry struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

var (
	reMarkdownImage = regexp.MustCompile(`!\[[^\]]*\]\(([^)\s]+)(?:\s+"[^"]*")?\)`)
	reFmImage       = regexp.MustCompile(`(?m)^(?:image|cover|featured_image):\s*(.+)$`)
)

var listableExtensions = []string{
	".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp", ".jfif",
}

// ListImages 对齐 list_images：图片已迁 R2/CDN，从 Markdown 正文与
// frontmatter 解析图片引用（不扫本地目录）。文章不存在返回空列表。
func ListImages(contentDir, articlePath string) (bool, any) {
	articleFile := joinContent(contentDir, articlePath)
	data, err := os.ReadFile(articleFile)
	if os.IsNotExist(err) {
		return true, []ImageEntry{}
	}
	if err != nil {
		return false, fmt.Sprintf("获取图片列表失败: %v", err)
	}
	text := string(data)

	seen := map[string]bool{}
	images := []ImageEntry{}
	add := func(url string) {
		if url == "" || seen[url] {
			return
		}
		seen[url] = true
		name := url
		if idx := strings.LastIndex(url, "/"); idx >= 0 {
			name = url[idx+1:]
		}
		images = append(images, ImageEntry{Name: name, URL: url})
	}

	for _, m := range reMarkdownImage.FindAllStringSubmatch(text, -1) {
		add(m[1])
	}
	for _, m := range reFmImage.FindAllStringSubmatch(text, -1) {
		url := strings.TrimSpace(m[1])
		url = strings.Trim(url, `"'`)
		url = strings.TrimSpace(url)
		isURL := strings.HasPrefix(url, "http")
		isFile := false
		lower := strings.ToLower(url)
		for _, ext := range listableExtensions {
			if strings.HasSuffix(lower, ext) {
				isFile = true
				break
			}
		}
		if isURL || isFile {
			add(url)
		}
	}
	return true, images
}
