package posts

import (
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/svtter/hugo-admin/internal/frontmatter"
	"github.com/svtter/hugo-admin/internal/util"
)

// 发布域：对齐 post_service 的 publish_article /
// bulk_publish_articles / get_publish_status。

var fileLocks sync.Map // path -> *sync.Mutex（进程内并发控制 + flock 跨进程）

// withFileLock 进程内互斥 + 跨进程 flock（对齐 _safe_file_operation）。
func withFileLock(path string, fn func() (bool, string)) (bool, string) {
	muAny, _ := fileLocks.LoadOrStore(path, &sync.Mutex{})
	mu := muAny.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()

	lockFile, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o644)
	if err == nil {
		if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err == nil {
			defer syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
			defer lockFile.Close()
			return fn()
		}
		lockFile.Close()
	}
	// flock 不可用时退化为进程内互斥
	return fn()
}

// PublishArticle 对齐 publish_article：draft true → false，无 publishDate
// 则补东八区时间；已发布返回"文章已经发布"。
func PublishArticle(contentDir, filePath string) (bool, string, string) {
	operationID := util.NewOperationID()
	abs := joinContent(contentDir, filePath)
	if !isSafePath(contentDir, abs) {
		return false, "访问被拒绝:文件不在允许的目录中", operationID
	}
	if _, err := os.Stat(abs); os.IsNotExist(err) {
		return false, fmt.Sprintf("文件不存在: %s", abs), operationID
	}

	ok, message := withFileLock(abs, func() (bool, string) {
		data, err := os.ReadFile(abs)
		if err != nil {
			return false, fmt.Sprintf("发布操作失败: %v", err)
		}
		doc, err := frontmatter.Parse(data)
		if err != nil {
			return false, fmt.Sprintf("发布操作失败: %v", err)
		}
		if doc.Degraded {
			// 形似 +++ 但无合法 frontmatter：明确报错而非误导性的"已发布"
			return false, "发布失败: 文件没有合法的 frontmatter"
		}
		draft, _ := doc.Metadata["draft"].(bool)
		if !draft {
			return false, "文章已经发布"
		}
		doc.Metadata["draft"] = false
		if _, has := doc.Metadata["publishDate"]; !has {
			cst := time.FixedZone("CST", 8*3600)
			doc.Metadata["publishDate"] = time.Now().In(cst).Format("2006-01-02T15:04:05+08:00")
		}
		if err := os.WriteFile(abs, doc.Dump(), 0o644); err != nil {
			return false, fmt.Sprintf("发布操作失败: %v", err)
		}
		return true, "文章发布成功"
	})
	return ok, message, operationID
}

// BulkPublishArticles 对齐 bulk_publish_articles。
func BulkPublishArticles(contentDir string, filePaths []string) map[string]any {
	results := make([]map[string]any, 0, len(filePaths))
	published, failed := 0, 0
	cst := time.FixedZone("CST", 8*3600)
	for _, filePath := range filePaths {
		ok, message, _ := PublishArticle(contentDir, filePath)
		entry := map[string]any{
			"file_path":    filePath,
			"success":      ok,
			"message":      nil,
			"published_at": nil,
		}
		if ok {
			published++
			entry["published_at"] = time.Now().In(cst).Format("2006-01-02T15:04:05+08:00")
		} else {
			failed++
			entry["message"] = message
		}
		results = append(results, entry)
	}
	return map[string]any{
		"success":         failed == 0,
		"total_count":     len(filePaths),
		"published_count": published,
		"failed_count":    failed,
		"operation_id":    util.NewOperationID(),
		"results":         results,
		"duration_ms":     0,
	}
}

// GetPublishStatus 对齐 get_publish_status：draft 缺省视为 true。
func GetPublishStatus(contentDir, filePath string) map[string]any {
	abs := joinContent(contentDir, filePath)
	if !isSafePath(contentDir, abs) {
		return map[string]any{
			"error":     "访问被拒绝:文件不在允许的目录中",
			"file_path": abs,
		}
	}
	data, err := os.ReadFile(abs)
	if os.IsNotExist(err) {
		return map[string]any{"error": "文件不存在", "file_path": abs}
	}
	if err != nil {
		return map[string]any{"error": fmt.Sprintf("状态检查失败: %v", err), "file_path": abs}
	}
	doc, err := frontmatter.Parse(data)
	if err != nil {
		return map[string]any{"error": fmt.Sprintf("状态检查失败: %v", err), "file_path": abs}
	}

	isDraft := true
	if v, ok := doc.Metadata["draft"].(bool); ok {
		isDraft = v
	}
	publishErrors := []string{}
	if title, _ := doc.Metadata["title"].(string); title == "" {
		publishErrors = append(publishErrors, "缺少标题")
	}

	var lastPublished any
	if !isDraft {
		lastPublished = doc.Metadata["publishDate"]
	}
	return map[string]any{
		"file_path":      abs,
		"is_draft":       isDraft,
		"is_publishable": isDraft,
		"last_published": lastPublished,
		"publish_errors": publishErrors,
		"frontmatter":    doc.Metadata,
	}
}
