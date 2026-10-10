// Package refs 对齐 services/reference_service.py：
// 扫描 Hugo ref shortcode，构建双向引用索引，支持反链与搜索。
package refs

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/svtter/hugo-admin/internal/db"
	"github.com/svtter/hugo-admin/internal/frontmatter"
)

var refPattern = regexp.MustCompile(`\{\{<\s*ref\s+"([^"]+)"\s*>\}\}`)

// ParseDate 由 posts 包提供（避免循环导入，此处声明为变量注入）。
var ParseDate func(any) *dateHolder

// dateHolder 是 time.Time 的最小接口（由调用方适配）。
type dateHolder = interface{}

type Service struct {
	contentDir string
	db         *db.DB
}

func New(contentDir string, database *db.DB) *Service {
	return &Service{contentDir: contentDir, db: database}
}

// ScanFile 对齐 scan_file：返回 [{target_path, context}]。
func (s *Service) ScanFile(filePath string) []db.RefEntry {
	path := filePath
	if !filepath.IsAbs(path) {
		path = filepath.Join(s.contentDir, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	text := string(data)
	refs := []db.RefEntry{}
	for _, loc := range refPattern.FindAllStringSubmatchIndex(text, -1) {
		target := strings.TrimLeft(text[loc[2]:loc[3]], "/")
		target = s.resolveTarget(target, path)
		start := loc[0] - 30
		if start < 0 {
			start = 0
		}
		end := loc[1] + 30
		if end > len(text) {
			end = len(text)
		}
		ctx := strings.TrimSpace(strings.ReplaceAll(text[start:end], "\n", " "))
		refs = append(refs, db.RefEntry{TargetPath: target, Context: ctx})
	}
	return refs
}

// resolveTarget 对齐 _resolve_target：相对源目录 → 全局搜索文件名。
func (s *Service) resolveTarget(target, sourcePath string) string {
	if strings.Contains(target, "/") && !strings.HasPrefix(target, "./") {
		return target
	}
	candidates := []string{filepath.Join(filepath.Dir(sourcePath), target)}
	if !strings.HasPrefix(target, "./") {
		_ = filepath.WalkDir(s.contentDir, func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.HasSuffix(filepath.ToSlash(p), "/"+target) {
				candidates = append(candidates, p)
			}
			return nil
		})
	}
	for _, candidate := range candidates {
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			if rel, err := filepath.Rel(s.contentDir, candidate); err == nil {
				return rel
			}
		}
	}
	return target
}

// ScanAll 对齐 scan_all：全量扫描重建引用索引并维护 posts 缓存行
// （backlinks 标题 JOIN 与搜索的数据源）。
func (s *Service) ScanAll() {
	allRefs := map[string][]db.RefEntry{}
	_ = filepath.WalkDir(s.contentDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".md") {
			return nil
		}
		allRefs[p] = s.ScanFile(p)
		if meta := parseForCache(s.contentDir, p); meta != nil {
			s.db.UpsertPost(p, meta.rel, meta.title, meta.date, meta.desc,
				meta.desc, meta.cover, meta.tags, meta.cats, meta.modTime)
		}
		return nil
	})
	_ = s.db.BatchUpsertReferences(allRefs)
}

type cacheMeta struct {
	rel, title, date, desc, cover string
	tags, cats                    []string
	modTime                       float64
}

func parseForCache(contentDir, path string) *cacheMeta {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil
	}
	doc, err := frontmatter.Parse(data)
	if err != nil {
		return nil
	}
	rel, err := filepath.Rel(contentDir, path)
	if err != nil {
		return nil
	}
	title, _ := doc.Metadata["title"].(string)
	desc, _ := doc.Metadata["description"].(string)
	cover, _ := doc.Metadata["cover"].(string)
	date := ""
	switch d := doc.Metadata["date"].(type) {
	case string:
		date = d
	case time.Time:
		// TOML/YAML 原生日期（已归一化为 time.Time）
		date = d.Format("2006-01-02")
	}
	var tags, cats []string
	if list, ok := doc.Metadata["tags"].([]any); ok {
		for _, t := range list {
			tags = append(tags, fmt.Sprintf("%v", t))
		}
	}
	if list, ok := doc.Metadata["categories"].([]any); ok {
		for _, c := range list {
			cats = append(cats, fmt.Sprintf("%v", c))
		}
	}
	return &cacheMeta{
		rel: rel, title: title, date: date, desc: desc, cover: cover,
		tags: tags, cats: cats,
		modTime: float64(st.ModTime().UnixNano()) / 1e9,
	}
}

// UpdateFile 对齐 update_file：增量更新单文件引用。
func (s *Service) UpdateFile(filePath string) {
	refs := s.ScanFile(filePath)
	_ = s.db.UpsertReferences(filePath, refs)
}

// GetBacklinks 对齐 get_backlinks。
func (s *Service) GetBacklinks(filePath string) []db.Backlink {
	bl, _ := s.db.GetBacklinks(strings.TrimLeft(filePath, "/"))
	if bl == nil {
		bl = []db.Backlink{}
	}
	return bl
}

// SearchPosts 对齐 search_posts：模糊搜索标题/路径/摘要/描述。
func (s *Service) SearchPosts(query string) []map[string]any {
	rows, err := s.db.SearchPostsLike(query, "", "")
	if err != nil {
		return []map[string]any{}
	}
	out := []map[string]any{}
	for _, r := range rows {
		out = append(out, map[string]any{"path": r.RelativePath, "title": r.Title})
	}
	return out
}
