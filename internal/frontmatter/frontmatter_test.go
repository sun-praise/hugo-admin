package frontmatter

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestParseWithFrontmatter(t *testing.T) {
	doc, err := Parse([]byte("---\ntitle: 标题\ntags:\n  - go\n  - hugo\ndraft: false\n---\n\n# 正文\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if doc.Metadata["title"] != "标题" {
		t.Fatalf("title = %v", doc.Metadata["title"])
	}
	if doc.Metadata["draft"] != false {
		t.Fatalf("draft = %v", doc.Metadata["draft"])
	}
	tags, _ := doc.Metadata["tags"].([]any)
	if len(tags) != 2 || tags[0] != "go" {
		t.Fatalf("tags = %#v", doc.Metadata["tags"])
	}
	// frontmatter.loads 语义：分隔线后的首个换行被剥离
	if doc.Content != "# 正文\n" && strings.TrimPrefix(doc.Content, "\n") != "# 正文\n" {
		t.Fatalf("content = %q", doc.Content)
	}
}

func TestParseWithoutFrontmatter(t *testing.T) {
	doc, err := Parse([]byte("# 纯正文\n"))
	if err != nil || doc.Metadata["title"] != nil {
		t.Fatalf("doc = %#v, err = %v", doc, err)
	}
	if doc.Content != "# 纯正文\n" {
		t.Fatalf("content = %q", doc.Content)
	}
}

func TestParseUnclosedFrontmatter(t *testing.T) {
	// 无闭合分隔线：对齐 frontmatter.loads，整体视为正文
	doc, err := Parse([]byte("---\ntitle: x\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if doc.Metadata["title"] != nil && doc.Content == "" {
		t.Fatalf("未闭合应视为正文: %#v", doc)
	}
}

func TestDumpRoundTrip(t *testing.T) {
	doc, err := Parse([]byte("---\ntitle: Round\nnum: 42\n---\n内容\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	dumped := doc.Dump()
	// python-frontmatter.dumps 的确切形态（实测字节）
	want := "---\nnum: 42\ntitle: Round\n---\n\n内容"
	if string(dumped) != want {
		t.Fatalf("dump = %q, want %q", dumped, want)
	}
	doc2, err := Parse(dumped)
	if err != nil {
		t.Fatalf("re-parse: %v (%q)", err, dumped)
	}
	if doc2.Metadata["title"] != "Round" || doc2.Content != "内容" {
		t.Fatalf("round trip 失败: %#v / %q", doc2.Metadata, doc2.Content)
	}
}

func TestParseTOML(t *testing.T) {
	// Fried-Rice exampleSite 风格：+++ + 引号日期 + 多行数组
	doc, err := Parse([]byte("+++\nauthor = \"Hugo Authors\"\ntitle = \"Emoji Support\"\ndate = \"2019-03-05\"\ncategories = [\n    \"Test\"\n]\ntags = [\n    \"emoji\",\n]\nimage = \"cover.jpg\"\n+++\n\nEmoji 内容\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if doc.Metadata["title"] != "Emoji Support" {
		t.Fatalf("title = %v", doc.Metadata["title"])
	}
	cats, _ := doc.Metadata["categories"].([]any)
	if len(cats) != 1 || cats[0] != "Test" {
		t.Fatalf("categories = %#v", doc.Metadata["categories"])
	}
	tags, _ := doc.Metadata["tags"].([]any)
	if len(tags) != 1 || tags[0] != "emoji" {
		t.Fatalf("tags = %#v", doc.Metadata["tags"])
	}
	if doc.Content != "Emoji 内容\n" {
		t.Fatalf("content = %q", doc.Content)
	}
	if !doc.TOML {
		t.Fatal("应标记为 TOML 文档")
	}
}

func TestParseTOMLNativeDate(t *testing.T) {
	// TOML 原生日期（无引号）解成 time.Time，与 YAML 原生日期同路
	doc, err := Parse([]byte("+++\ntitle = \"x\"\ndate = 2019-03-05\n+++\nbody\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	ts, ok := doc.Metadata["date"].(time.Time)
	if !ok {
		t.Fatalf("date 类型 = %T", doc.Metadata["date"])
	}
	if ts.Year() != 2019 || ts.Month() != time.March || ts.Day() != 5 {
		t.Fatalf("date = %v", ts)
	}
}

func TestDumpTOMLRoundTrip(t *testing.T) {
	doc, err := Parse([]byte("+++\ntitle = \"Round\"\ndate = \"2019-03-05\"\n+++\n内容\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	dumped := doc.Dump()
	if !strings.HasPrefix(string(dumped), "+++\n") {
		t.Fatalf("dump 应保留 +++ 格式: %q", dumped)
	}
	doc2, err := Parse(dumped)
	if err != nil {
		t.Fatalf("re-parse: %v (%q)", err, dumped)
	}
	if doc2.Metadata["title"] != "Round" || doc2.Content != "内容" {
		t.Fatalf("round trip 失败: %#v / %q", doc2.Metadata, doc2.Content)
	}
}

func TestParseTOMLUnclosed(t *testing.T) {
	// 无闭合 +++：整体视为正文
	text := "+++\ntitle = \"x\"\n"
	doc, err := Parse([]byte(text))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(doc.Metadata) != 0 || doc.Content != text || doc.TOML {
		t.Fatalf("未闭合应视为正文: %#v", doc)
	}
}

func TestParseTOMLInvalidDegrades(t *testing.T) {
	// 形似 frontmatter（首行 +++，后文另有 +++ 行）但块内容非合法 TOML：
	// 对齐 Python v2，整体降级为正文，不让文章从列表消失
	text := "+++\n这是正文开头的一行\n+++\n继续正文\n"
	doc, err := Parse([]byte(text))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(doc.Metadata) != 0 || doc.Content != text {
		t.Fatalf("应降级为纯正文: meta=%#v content=%q", doc.Metadata, doc.Content)
	}
	if doc.TOML {
		t.Fatal("不应标记为 TOML 文档")
	}
}

func TestParseTOMLNestedDates(t *testing.T) {
	doc, err := Parse([]byte("+++\ndate = 2019-03-05\n[extra]\nwhen = 2020-01-02\nlist = [2021-06-30]\n+++\n正文\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, ok := doc.Metadata["date"].(time.Time); !ok {
		t.Fatalf("date 类型 = %T", doc.Metadata["date"])
	}
	extra, _ := doc.Metadata["extra"].(map[string]any)
	if extra == nil {
		t.Fatalf("extra = %#v", doc.Metadata["extra"])
	}
	if _, ok := extra["when"].(time.Time); !ok {
		t.Fatalf("嵌套 when 类型 = %T", extra["when"])
	}
	list, _ := extra["list"].([]any)
	if len(list) != 1 {
		t.Fatalf("list = %#v", extra["list"])
	}
	if _, ok := list[0].(time.Time); !ok {
		t.Fatalf("数组内日期类型 = %T", list[0])
	}
}

func TestDumpTOMLFiltersNil(t *testing.T) {
	// /api/file/save 可能透传 JSON null；TOML 无法编码 nil，应剔除而非降级为 ---
	doc, err := Parse([]byte("+++\ntitle = \"T\"\ncategories = [\"a\"]\n+++\n正文\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	doc.Metadata["cover"] = nil
	dumped := doc.Dump()
	if !strings.HasPrefix(string(dumped), "+++\n") {
		t.Fatalf("应保持 +++: %q", dumped)
	}
	if strings.Contains(string(dumped), "cover") {
		t.Fatalf("nil 字段应被剔除: %q", dumped)
	}
	doc2, err := Parse(dumped)
	if err != nil || doc2.Metadata["title"] != "T" {
		t.Fatalf("re-parse: %v %#v", err, doc2.Metadata)
	}
}

func TestParseDumpTOMLLocalTime(t *testing.T) {
	// 无日期语义的 LocalTime 原样保留，round trip 不丢
	doc, err := Parse([]byte("+++\ntitle = \"T\"\nstart = 09:30:00\n+++\n正文\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	dumped := doc.Dump()
	if !strings.HasPrefix(string(dumped), "+++\n") {
		t.Fatalf("dump 应保持 +++: %q", dumped)
	}
	doc2, err := Parse(dumped)
	if err != nil {
		t.Fatalf("re-parse: %v (%q)", err, dumped)
	}
	if s := fmt.Sprintf("%v", doc2.Metadata["start"]); !strings.HasPrefix(s, "09:30:00") {
		t.Fatalf("LocalTime round trip = %v", doc2.Metadata["start"])
	}
}

func TestDumpTOMLFiltersNestedNil(t *testing.T) {
	doc, err := Parse([]byte("+++\ntitle = \"T\"\n[params]\ncover = \"c.jpg\"\n+++\n正文\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	doc.Metadata["desc"] = nil
	params, ok := doc.Metadata["params"].(map[string]any)
	if !ok {
		t.Fatalf("params = %#v（类型断言失败应显式报错）", doc.Metadata["params"])
	}
	params["banner"] = nil
	dumped := doc.Dump()
	if !strings.HasPrefix(string(dumped), "+++\n") {
		t.Fatalf("嵌套 nil 应被剔除而非降级: %q", dumped)
	}
	if strings.Contains(string(dumped), "desc") || strings.Contains(string(dumped), "banner") {
		t.Fatalf("nil 字段残留: %q", dumped)
	}
	if !strings.Contains(string(dumped), "cover") {
		t.Fatalf("非 nil 嵌套字段应保留: %q", dumped)
	}
	doc2, err := Parse(dumped)
	if err != nil || doc2.Metadata["title"] != "T" {
		t.Fatalf("re-parse: %v %#v", err, doc2.Metadata)
	}
}

func TestDumpTOMLAllNil(t *testing.T) {
	doc, err := Parse([]byte("+++\ntitle = \"T\"\n+++\n正文\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	doc.Metadata["title"] = nil
	dumped := doc.Dump()
	// 全 nil：保留空 +++ 块的 TOML 形态，不丢格式标记
	if !strings.HasPrefix(string(dumped), "+++\n") {
		t.Fatalf("应保留空 +++ 块: %q", dumped)
	}
	doc2, err := Parse(dumped)
	if err != nil || !doc2.TOML || len(doc2.Metadata) != 0 {
		t.Fatalf("re-parse: %v %#v", err, doc2)
	}
}

func TestParseTOMLArrayOfTables(t *testing.T) {
	// [[array of tables]]：钉住 go-toml 解码形态并确认归一化路径可达
	doc, err := Parse([]byte("+++\ntitle = \"T\"\n[[items]]\nname = \"a\"\nwhen = 2019-03-05\n\n[[items]]\nname = \"b\"\nwhen = 2020-01-02\n+++\n正文\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	items, ok := doc.Metadata["items"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("items = %#v", doc.Metadata["items"])
	}
	first, ok := items[0].(map[string]any)
	if !ok || first["name"] != "a" {
		t.Fatalf("items[0] = %#v", items[0])
	}
	if _, ok := first["when"].(time.Time); !ok {
		t.Fatalf("数组表内日期未归一化: %T", first["when"])
	}
}
