// Package posts 实现文章只读域：扫描 content/post、解析 frontmatter、
// 过滤分页与标签/分类聚合。行为逐条对齐 Python 的
// utils/blog_parser.py 与 services/post_service.py。
package posts

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/svtter/hugo-admin/internal/frontmatter"
)

// BlogPost 对齐 Python BlogPost。
type BlogPost struct {
	FilePath     string
	RelativePath string
	Title        string
	Date         *time.Time
	Description  string
	Tags         []string
	Categories   []string
	Draft        bool
	Content      string
	Excerpt      string
	Cover        string
	ModTime      int64 // unix 秒
}

var (
	reHeading = regexp.MustCompile(`#+ `)
	reLink    = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`)
	reFormat  = regexp.MustCompile("[*_`]")
)

// parsePost 解析单个 Markdown 文件。解析失败时返回 nil（对齐 Python
// 的 try/except + 跳过）。
func parsePost(path, contentDir string) *BlogPost {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return nil
	}

	doc, err := frontmatter.Parse(data)
	if err != nil {
		return nil
	}
	meta := doc.Metadata

	p := &BlogPost{
		FilePath:     path,
		RelativePath: relativePath(path, contentDir),
		Title:        strOr(meta["title"], ""),
		Description:  strOr(meta["description"], ""),
		Draft:        boolOr(meta["draft"], false),
		Tags:         strList(meta["tags"]),
		Categories:   strList(meta["categories"]),
		Content:      doc.Content,
		ModTime:      st.ModTime().Unix(),
	}
	p.Cover = resolveCover(meta)
	p.Date = parseDate(meta["date"])
	p.Excerpt = generateExcerpt(p.Content)
	return p
}

// resolveCover 复刻 cover → image → images 的兜底链。
func resolveCover(meta map[string]any) string {
	cover := ""
	for _, key := range []string{"cover", "image"} {
		if v := meta[key]; v != nil {
			cover = scalarOrFirst(v)
			if cover != "" {
				return strings.TrimSpace(cover)
			}
		}
	}
	if v, ok := meta["images"]; ok && v != nil {
		return strings.TrimSpace(scalarOrFirst(v))
	}
	return ""
}

func scalarOrFirst(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []any:
		if len(t) == 0 {
			return ""
		}
		return strOr(t[0], "")
	default:
		if v == nil {
			return ""
		}
		return fmt.Sprintf("%v", v)
	}
}

// parseDate 对齐 Python：ISO 字符串（含 Z）→ YYYY-MM-DD → 失败为 nil。
// YAML 原生日期由 yaml.v3 解成 time.Time，同样接受。
func parseDate(v any) *time.Time {
	if v == nil {
		return nil
	}
	switch t := v.(type) {
	case time.Time:
		return &t
	case string:
		s := strings.ReplaceAll(t, "Z", "+00:00")
		for _, layout := range []string{time.RFC3339, "2006-01-02"} {
			if ts, err := time.Parse(layout, s); err == nil {
				return &ts
			}
		}
	}
	return nil
}

func strOr(v any, fallback string) string {
	if s, ok := v.(string); ok {
		return s
	}
	if v == nil {
		return fallback
	}
	return fmt.Sprintf("%v", v)
}

// strList 对齐 Python：缺失为空列表，单个字符串包一层。
func strList(v any) []string {
	switch t := v.(type) {
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			out = append(out, strOr(item, ""))
		}
		return out
	case string:
		return []string{t}
	default:
		return nil
	}
}

func boolOr(v any, fallback bool) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return fallback
}

// generateExcerpt 复刻 Python _generate_excerpt：剥离标题/链接/格式标记
// 后取前 200 字符，超长补 "..."。
func generateExcerpt(content string) string {
	if content == "" {
		return ""
	}
	text := reHeading.ReplaceAllString(content, "")
	text = reLink.ReplaceAllString(text, "$1")
	text = reFormat.ReplaceAllString(text, "")
	text = strings.TrimSpace(text)
	runes := []rune(text)
	if len(runes) <= 200 {
		return string(runes)
	}
	return string(runes[:200]) + "..."
}

func relativePath(path, contentDir string) string {
	rel, err := filepath.Rel(contentDir, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return path
	}
	return rel
}

// GetBlogPosts 扫描 contentDir/post 下所有 .md（递归），按日期降序；
// 无日期的排最后。跳过空文章（无标题且无正文）。
func GetBlogPosts(contentDir string) []*BlogPost {
	postDir := filepath.Join(contentDir, "post")
	var posts []*BlogPost

	_ = filepath.WalkDir(postDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		p := parsePost(path, contentDir)
		if p == nil {
			return nil
		}
		if p.Title == "" && p.Content == "" {
			return nil
		}
		posts = append(posts, p)
		return nil
	})

	sort.SliceStable(posts, func(i, j int) bool {
		ti, tj := posts[i].Date, posts[j].Date
		// Python 语义：无日期视为最小；tz-aware 去时区后按壁挂时间比较。
		// 用字段字符串比较可同时覆盖两种情况（字典序 = 时间序）。
		vi, vj := wallclock(ti), wallclock(tj)
		return vi > vj
	})
	return posts
}

// wallclock 返回可比的壁挂时间字符串；无日期返回 ""（最小）。
func wallclock(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02T15:04:05.000000000")
}

// PostItem 是 /api/posts 列表项，字段名与 Python jsonify 输出一致。
type PostItem struct {
	Title       string   `json:"title"`
	Path        string   `json:"path"`
	FullPath    string   `json:"full_path"`
	Date        string   `json:"date"`
	Description string   `json:"description"`
	Excerpt     string   `json:"excerpt"`
	Tags        []string `json:"tags"`
	Categories  []string `json:"categories"`
	Cover       string   `json:"cover"`
	CoverURL    string   `json:"cover_url"`
	ModTime     string   `json:"mod_time"`
}

// PostsResult 是 /api/posts 响应。
type PostsResult struct {
	Posts      []PostItem `json:"posts"`
	Total      int        `json:"total"`
	Page       int        `json:"page"`
	PerPage    int        `json:"per_page"`
	TotalPages int        `json:"total_pages"`
	HasNext    bool       `json:"has_next"`
	HasPrev    bool       `json:"has_prev"`
}

// GetPosts 对齐 PostService.get_posts：搜索/过滤 + 分页。
func GetPosts(contentDir, query, category, tag string, page, perPage int) PostsResult {
	all := GetBlogPosts(contentDir)

	if query != "" {
		all = filterBySearch(all, query)
	}
	if category != "" {
		filtered := all[:0:0]
		for _, p := range all {
			if containsStr(p.Categories, category) {
				filtered = append(filtered, p)
			}
		}
		all = filtered
	}
	if tag != "" {
		filtered := all[:0:0]
		for _, p := range all {
			if containsStr(p.Tags, tag) {
				filtered = append(filtered, p)
			}
		}
		all = filtered
	}

	total := len(all)
	totalPages := (total + perPage - 1) / perPage
	if page < 1 {
		page = 1
	}
	start := (page - 1) * perPage
	end := start + perPage
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}

	items := make([]PostItem, 0, end-start)
	for _, p := range all[start:end] {
		dateStr := ""
		if p.Date != nil {
			dateStr = p.Date.Local().Format("2006-01-02")
		}
		items = append(items, PostItem{
			Title:       p.Title,
			Path:        p.RelativePath,
			FullPath:    p.FilePath,
			Date:        dateStr,
			Description: p.Description,
			Excerpt:     p.Excerpt,
			Tags:        p.Tags,
			Categories:  p.Categories,
			Cover:       p.Cover,
			CoverURL:    ResolveCoverURL(p.RelativePath, p.Cover),
			ModTime:     time.Unix(p.ModTime, 0).Format("2006-01-02 15:04"),
		})
	}
	return PostsResult{
		Posts:      items,
		Total:      total,
		Page:       page,
		PerPage:    perPage,
		TotalPages: totalPages,
		HasNext:    page < totalPages,
		HasPrev:    page > 1,
	}
}

// filterBySearch 对齐 filter_posts_by_search(fields=["all"])：
// 标题/描述/正文/标签/分类拼接后小写包含。
func filterBySearch(posts []*BlogPost, q string) []*BlogPost {
	q = strings.ToLower(q)
	out := make([]*BlogPost, 0)
	for _, p := range posts {
		text := strings.ToLower(strings.Join([]string{
			p.Title, p.Description, p.Content,
			strings.Join(p.Tags, " "), strings.Join(p.Categories, " "),
		}, " "))
		if strings.Contains(text, q) {
			out = append(out, p)
		}
	}
	return out
}

// NameCount 是标签/分类聚合计数。
type NameCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// GetAllTags 对齐 get_all_tags：计数并按数量降序（稳定）。
func GetAllTags(contentDir string) []NameCount {
	return aggregate(GetBlogPosts(contentDir), func(p *BlogPost) []string { return p.Tags })
}

// GetAllCategories 对齐 get_all_categories。
func GetAllCategories(contentDir string) []NameCount {
	return aggregate(GetBlogPosts(contentDir), func(p *BlogPost) []string { return p.Categories })
}

func aggregate(posts []*BlogPost, pick func(*BlogPost) []string) []NameCount {
	counts := map[string]int{}
	var order []string // 首次出现顺序，保证与 Python dict 插入序一致
	for _, p := range posts {
		for _, name := range pick(p) {
			if _, seen := counts[name]; !seen {
				order = append(order, name)
			}
			counts[name]++
		}
	}
	out := make([]NameCount, 0, len(order))
	for _, name := range order {
		out = append(out, NameCount{Name: name, Count: counts[name]})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	return out
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// ResolveCoverURL 对齐 _resolve_cover_url：绝对 URL 或 / 开头原样返回；
// 相对路径基于文章目录解析为 /content/ 前缀。
func ResolveCoverURL(relativePath, cover string) string {
	if strings.TrimSpace(cover) == "" {
		return ""
	}
	cover = strings.TrimSpace(cover)
	if strings.HasPrefix(cover, "http://") || strings.HasPrefix(cover, "https://") || strings.HasPrefix(cover, "/") {
		return cover
	}
	parts := strings.Split(filepath.Dir(relativePath), string(filepath.Separator))
	normalized := []string{}
	for _, part := range append(parts, strings.Split(filepath.ToSlash(cover), "/")...) {
		if part == ".." {
			if len(normalized) > 0 {
				normalized = normalized[:len(normalized)-1]
			}
		} else if part != "." && part != "" {
			normalized = append(normalized, part)
		}
	}
	if len(normalized) == 0 {
		return ""
	}
	return "/content/" + strings.Join(normalized, "/")
}
