// Package frontmatter 解析与生成 frontmatter 文档，
// YAML（---，语义对齐 Python 的 python-frontmatter 库）与 TOML（+++，
// Hugo 官方示例内容常用）双格式支持；读入什么格式，Dump 就写回什么格式。
//
//	---
//	title: 标题
//	---
//	正文
package frontmatter

import (
	"bytes"
	"strings"
	"time"

	toml "github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

const (
	yamlDelimiter = "---"
	tomlDelimiter = "+++"
)

// Document 是一篇带 frontmatter 的 Markdown 文档。
// TOML 为 true 时 Dump 用 +++/TOML 序列化（读入保留原格式，
// 避免编辑保存后把 +++ 文件改写成 ---）。
type Document struct {
	Metadata map[string]any
	Content  string
	TOML     bool
}

// Parse 解析整篇文档；无 frontmatter 时 Metadata 为空 map、Content 为原文。
// 行为对齐 frontmatter.loads：frontmatter 块后恰好一个空行被剥离。
func Parse(data []byte) (*Document, error) {
	text := string(data)
	meta := map[string]any{}

	delim := yamlDelimiter
	isTOML := false
	if strings.HasPrefix(text, tomlDelimiter) {
		delim = tomlDelimiter
		isTOML = true
	}
	if !strings.HasPrefix(text, delim) {
		return &Document{Metadata: meta, Content: text}, nil
	}

	// 跳过首行分隔线
	rest := text[len(delim):]
	rest = strings.TrimLeft(rest, "\r")
	rest = strings.TrimPrefix(rest, "\n")

	end := findClosingDelimiter(rest, delim)
	if end < 0 {
		return &Document{Metadata: meta, Content: text}, nil
	}
	block := rest[:end]
	// 收尾分隔线之后的内容：剥掉所有前导换行（python-frontmatter 的
	// loads 语义，空格保留），尾部原样保留
	content := rest[end+len(delim):]
	content = strings.TrimLeft(content, "\r\n")

	if isTOML {
		if err := toml.Unmarshal([]byte(block), &meta); err != nil {
			// 对齐 Python v2（python-frontmatter 默认只认 ---）：
			// 形似 +++ 但非合法 frontmatter 的文件整体视为正文，
			// 不让整篇文章从列表里消失
			return &Document{Metadata: map[string]any{}, Content: text}, nil
		}
		normalizeTOMLDates(meta)
		return &Document{Metadata: meta, Content: content, TOML: true}, nil
	}
	if err := yaml.Unmarshal([]byte(block), &meta); err != nil {
		return nil, err
	}
	return &Document{Metadata: meta, Content: content}, nil
}

// findClosingDelimiter 返回闭合分隔线行的起始偏移；找不到返回 -1。
func findClosingDelimiter(s, delim string) int {
	offset := 0
	for len(s) > 0 {
		line, rest := cutLine(s)
		if strings.TrimRight(line, " \t\r") == delim {
			return offset
		}
		offset += len(line)
		if len(rest) < len(s) { // 该行后有换行符
			offset++
		}
		s = rest
	}
	return -1
}

func cutLine(s string) (line, rest string) {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i], s[i+1:]
	}
	return s, ""
}

// normalizeTOMLDates 把 go-toml 的 LocalDate/LocalDateTime 归一化成
// time.Time（UTC 零点，与 yaml.v3 原生日期一致），让消费方走同一路径；
// LocalTime（无日期语义）原样保留，Dump 时由 go-toml 编码回原值。
func normalizeTOMLDates(m map[string]any) {
	for k, v := range m {
		m[k] = normalizeTOMLValue(v)
	}
}

func normalizeTOMLValue(v any) any {
	switch t := v.(type) {
	case toml.LocalDate:
		return t.AsTime(time.UTC)
	case toml.LocalDateTime:
		return t.AsTime(time.UTC)
	case map[string]any:
		normalizeTOMLDates(t)
		return t
	case []any:
		for i := range t {
			t[i] = normalizeTOMLValue(t[i])
		}
		return t
	default:
		return v
	}
}

// Dump 序列化为 python-frontmatter.dumps 的确切形态：
// "---\n<yaml.strip()>\n---\n\n<content.strip()>"；TOML 文档对应
// "+++\n<toml.strip()>\n+++\n\n<content.strip()>"（编码失败回退 YAML）。
// Metadata 为空时仅输出 content 原文。
func (d *Document) Dump() []byte {
	if len(d.Metadata) == 0 {
		return []byte(d.Content)
	}
	body := strings.TrimSpace(d.Content)
	if d.TOML {
		// JSON null 字段（/api/file/save 透传的 nil）无法被 TOML 编码，
		// 剔除后再序列化，避免触发下面的 YAML 静默降级
		filtered := filterNil(d.Metadata)
		if len(filtered) == 0 {
			return []byte(body)
		}
		var buf bytes.Buffer
		if err := toml.NewEncoder(&buf).Encode(filtered); err == nil {
			return []byte(tomlDelimiter + "\n" + strings.TrimSpace(buf.String()) + "\n" + tomlDelimiter + "\n\n" + body)
		}
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	_ = enc.Encode(d.Metadata)
	enc.Close()
	yamlStr := strings.TrimSpace(buf.String())
	return []byte(yamlDelimiter + "\n" + yamlStr + "\n" + yamlDelimiter + "\n\n" + body)
}

// filterNil 返回剔除 nil 值后的浅拷贝（TOML 无法编码 null）。
func filterNil(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		if v != nil {
			out[k] = v
		}
	}
	return out
}
