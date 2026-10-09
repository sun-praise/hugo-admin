// Package frontmatter 解析与生成 YAML frontmatter 文档，
// 语义对齐 Python 的 python-frontmatter 库：
//
//	---
//	title: 标题
//	---
//	正文
package frontmatter

import (
	"bytes"
	"strings"

	"gopkg.in/yaml.v3"
)

const delimiter = "---"

// Document 是一篇带 frontmatter 的 Markdown 文档。
type Document struct {
	Metadata map[string]any
	Content  string
}

// Parse 解析整篇文档；无 frontmatter 时 Metadata 为空 map、Content 为原文。
// 行为对齐 frontmatter.loads：frontmatter 块后恰好一个空行被剥离。
func Parse(data []byte) (*Document, error) {
	text := string(data)
	meta := map[string]any{}

	if !strings.HasPrefix(text, delimiter) {
		return &Document{Metadata: meta, Content: text}, nil
	}

	// 跳过首行 "---"
	rest := text[len(delimiter):]
	rest = strings.TrimLeft(rest, "\r")
	rest = strings.TrimPrefix(rest, "\n")

	end := findClosingDelimiter(rest)
	if end < 0 {
		return &Document{Metadata: meta, Content: text}, nil
	}
	block := rest[:end]
	// 收尾分隔线之后的内容：剥掉所有前导换行（python-frontmatter 的
	// loads 语义，空格保留），尾部原样保留
	content := rest[end+len(delimiter):]
	content = strings.TrimLeft(content, "\r\n")

	if err := yaml.Unmarshal([]byte(block), &meta); err != nil {
		return nil, err
	}
	return &Document{Metadata: meta, Content: content}, nil
}

// findClosingDelimiter 返回闭合 "---" 行的起始偏移；找不到返回 -1。
func findClosingDelimiter(s string) int {
	offset := 0
	for len(s) > 0 {
		line, rest := cutLine(s)
		if strings.TrimRight(line, " \t\r") == delimiter {
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

// Dump 序列化为 python-frontmatter.dumps 的确切形态：
// "---\n<yaml.strip()>\n---\n\n<content.strip()>"。
// Metadata 为空时仅输出 content 原文。
func (d *Document) Dump() []byte {
	if len(d.Metadata) == 0 {
		return []byte(d.Content)
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	_ = enc.Encode(d.Metadata)
	enc.Close()
	yamlStr := strings.TrimSpace(buf.String())
	return []byte(delimiter + "\n" + yamlStr + "\n" + delimiter + "\n\n" + strings.TrimSpace(d.Content))
}
