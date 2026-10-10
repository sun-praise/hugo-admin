package ai

// LLM 输出规整：从任意模型输出中提取 JSON 并收敛为前端可安全渲染的
// 任务契约。移植自 v2 services/assist_service.py（normalize.ts/json.ts）。

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var (
	reJSONFence     = regexp.MustCompile("(?is)```(?:json)?\\s*(.*?)```")
	reTrailingComma = regexp.MustCompile(`,\s*([}\]])`)
)

// ExtractJSON 容忍 code fence、JSON 前后的说明文字与尾逗号；
// 提取不到 JSON 对象时返回 nil。
func ExtractJSON(raw string) map[string]any {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil
	}
	if m := reJSONFence.FindStringSubmatch(s); m != nil {
		s = strings.TrimSpace(m[1])
	}
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start == -1 || end <= start {
		return nil
	}
	body := s[start : end+1]
	if obj := unmarshalObject(body); obj != nil {
		return obj
	}
	// 修复尾逗号后重试
	body = reTrailingComma.ReplaceAllString(body, "$1")
	return unmarshalObject(body)
}

func unmarshalObject(body string) map[string]any {
	var obj map[string]any
	if err := json.Unmarshal([]byte(body), &obj); err != nil {
		return nil
	}
	return obj
}

// firstString 依次取第一个非空字符串字段（对齐 normalize.ts 的字段回退）。
func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok {
				if t := strings.TrimSpace(s); t != "" {
					return t
				}
			} else if v != nil {
				if s := strings.TrimSpace(toDisplayString(v)); s != "" {
					return s
				}
			}
		}
	}
	return ""
}

func toDisplayString(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

func firstStringList(m map[string]any, listKey string, altKeys ...string) []any {
	if arr, ok := m[listKey].([]any); ok {
		return arr
	}
	for _, k := range altKeys {
		if arr, ok := m[k].([]any); ok {
			return arr
		}
	}
	return nil
}

func normalizeTranslate(raw map[string]any) map[string]any {
	translation := firstString(raw, "translation", "zh", "text")
	if translation == "" {
		return nil
	}
	hardWords := []any{}
	for _, item := range firstStringList(raw, "hard_words", "terms") {
		o, ok := item.(map[string]any)
		if !ok {
			continue
		}
		word := firstString(o, "word", "label", "zh")
		if word == "" {
			continue
		}
		hardWords = append(hardWords, map[string]any{
			"word":   word,
			"source": firstString(o, "source", "en", "orig"),
			"gloss":  firstString(o, "gloss", "note", "def"),
		})
		if len(hardWords) >= 8 {
			break
		}
	}
	return map[string]any{"translation": translation, "hard_words": hardWords}
}

var polishStyleFallback = []string{"formal", "concise", "restructure"}

func normalizePolish(raw map[string]any) map[string]any {
	var arr []any
	if list, ok := raw["candidates"].([]any); ok {
		arr = list
	}
	candidates := []any{}
	for _, item := range arr {
		o, ok := item.(map[string]any)
		if !ok {
			continue
		}
		text := firstString(o, "text", "rewrite")
		if text == "" {
			continue
		}
		style := firstString(o, "style")
		if style == "" && len(candidates) < len(polishStyleFallback) {
			style = polishStyleFallback[len(candidates)]
		}
		candidates = append(candidates, map[string]any{
			"style": style,
			"note":  firstString(o, "note", "reason"),
			"text":  text,
		})
		if len(candidates) >= 3 {
			break
		}
	}
	// 契约要求恰好 3 个候选；不足即规整失败（客户端可重试），
	// 避免前端三风格布局静默降级。
	if len(candidates) < 3 {
		return nil
	}
	return map[string]any{"candidates": candidates}
}

var (
	reviewTypeSet = map[string]bool{
		"overclaim": true, "title": true, "terminology": true,
		"colloquial": true, "structure": true, "other": true,
	}
	reviewSeveritySet   = map[string]bool{"high": true, "warn": true, "info": true}
	reviewSeverityAlias = map[string]string{"medium": "warn", "low": "info", "ok": "info"}
)

// normalizeReview 空 issues 是合法结果（选区没有问题），返回非 nil。
func normalizeReview(raw map[string]any) map[string]any {
	arr, ok := raw["issues"].([]any)
	if !ok {
		return nil
	}
	issues := []any{}
	for _, item := range arr {
		o, ok := item.(map[string]any)
		if !ok {
			continue
		}
		comment := firstString(o, "comment", "detail", "description", "title")
		if comment == "" {
			continue
		}

		before := firstString(o, "before")
		after := firstString(o, "after")
		if fix, ok := o["fix"].(map[string]any); ok {
			if before == "" {
				before = firstString(fix, "before", "find")
			}
			if after == "" {
				after = firstString(fix, "after", "replace")
			}
		}
		if before == "" || after == "" {
			// 修复项必须成对，否则视为无可修复项
			before, after = "", ""
		}

		issueType := firstString(o, "type", "category")
		if !reviewTypeSet[issueType] {
			issueType = "other"
		}
		severity := strings.ToLower(firstString(o, "severity", "sev"))
		if alias, ok := reviewSeverityAlias[severity]; ok {
			severity = alias
		}
		if !reviewSeveritySet[severity] {
			severity = "warn"
		}

		issue := map[string]any{
			"type":     issueType,
			"severity": severity,
			"comment":  comment,
		}
		if before != "" {
			issue["before"] = before
			issue["after"] = after
		} else {
			issue["before"] = nil
			issue["after"] = nil
		}
		issues = append(issues, issue)
		if len(issues) >= 8 {
			break
		}
	}
	return map[string]any{"issues": issues}
}

// NormalizeFor 把提取出的 JSON 收敛为任务契约；失败返回描述性错误。
func NormalizeFor(task string, raw map[string]any) (map[string]any, error) {
	var normalized map[string]any
	switch task {
	case "translate":
		normalized = normalizeTranslate(raw)
	case "polish":
		normalized = normalizePolish(raw)
	case "review":
		normalized = normalizeReview(raw)
	default:
		return nil, fmt.Errorf("unsupported task: %s", task)
	}
	if normalized == nil {
		return nil, fmt.Errorf("model output does not match the %s JSON contract", task)
	}
	return normalized, nil
}
