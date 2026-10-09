package ai

// SSE 流映射：trpc-agent-go 事件流 → Python ai_routes 的 SSE 协议。
// 协议（前端消费的帧格式）：
//   data: <转义文本>\n\n                       —— 文本增量
//   data: {"type":"tool_call",...}\n\n          —— 工具调用
//   data: {"type":"tool_result",...}\n\n        —— 工具结果
//   data: {"type":"error","error":...}\n\n      —— 错误
//   data: [DONE]\n\n                            —— 结束

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"trpc.group/trpc-go/trpc-agent-go/model"
)

// sseText 对齐 _sse_data_line：去除 \r、\n 转义为字面 \\n。
func sseText(text string) string {
	text = strings.ReplaceAll(text, "\r", "")
	text = strings.ReplaceAll(text, "\n", "\\n")
	return "data: " + text + "\n\n"
}

func sseJSON(payload map[string]any) string {
	b, _ := json.Marshal(payload)
	return "data: " + string(b) + "\n\n"
}

// decodeMaybeJSONString 把工具结果内容解一层 JSON 字符串编码
// （tool.response 的 content 是 JSON 编码后的字符串，如 "\"找到..\""）。
func decodeMaybeJSONString(s string) string {
	var out string
	if err := json.Unmarshal([]byte(s), &out); err == nil {
		return out
	}
	return s
}

// ============ inline-edit 的提示词与后处理（对齐 inline_edit_routes） ============

const (
	MaxSelectedText = 5000
	MaxInstruction  = 1000
	MaxContext      = 1000
)

// BuildInlinePrompts 对齐 _build_prompts。
func BuildInlinePrompts(selectedText, instruction, contextBefore, contextAfter string) (sys, user string) {
	sys = "你是一个 Markdown 改写助手。" +
		"用户会给你一段 Markdown 片段和它前后的上下文，以及改写指令。" +
		"你只输出改写后的 Markdown 片段本身，不要任何解释、前缀、后缀、" +
		"代码块包裹或换行声明。"
	user = "上下文（前）：\n" + contextBefore + "\n\n" +
		"需要改写的片段：\n" + selectedText + "\n\n" +
		"上下文（后）：\n" + contextAfter + "\n\n" +
		"改写指令：" + instruction
	return sys, user
}

var (
	reMarkdownMarker = regexp.MustCompile("(`|#|\\*\\*|\\[|>|\\* |- |\\+ |\\d+\\. )")
	reLangTag        = regexp.MustCompile(`^[a-zA-Z0-9_+\-]+$`)
	reDangerous      = regexp.MustCompile(`(?i)<\s*script\b|<\s*iframe\b|javascript\s*:|on\w+\s*=`)
)

// looksLikeMarkdown 对齐 _looks_like_markdown：空行算 Markdown。
func looksLikeMarkdown(line string) bool {
	s := strings.TrimSpace(line)
	if s == "" {
		return true
	}
	return reMarkdownMarker.MatchString(s)
}

// UnwrapSingleFence 对齐 _unwrap_single_fence：整体被单个代码块包裹时解包。
func UnwrapSingleFence(text string) string {
	stripped := strings.TrimSpace(text)
	if !strings.HasPrefix(stripped, "```") || !strings.HasSuffix(stripped, "```") {
		return text
	}
	body := stripped[3 : len(stripped)-3]
	if strings.Contains(body, "```") {
		return text
	}
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	if len(lines) > 0 && strings.TrimSpace(lines[0]) != "" && !looksLikeMarkdown(lines[0]) {
		if reLangTag.MatchString(strings.TrimSpace(lines[0])) {
			lines = lines[1:]
		}
	}
	inner := strings.TrimSpace(strings.Join(lines, "\n"))
	if inner == "" {
		return text
	}
	return inner
}

// IsDangerousResponse 对齐 XSS 防御性拒绝。
func IsDangerousResponse(text string) bool {
	return reDangerous.MatchString(text)
}

// ChatSSE 运行 agent 并把事件流映射为 SSE 帧序列。
// 返回的 channel 关闭即流结束（最后一帧为 data: [DONE]）。
func (s *Service) ChatSSE(ctx context.Context, message string) (<-chan string, error) {
	if !s.enabled {
		return nil, fmt.Errorf("AI service is not configured")
	}
	sessionID := time.Now().Format("20060102-150405.000000000")
	events, err := s.runner.Run(ctx, "web", sessionID, model.NewUserMessage(message))
	if err != nil {
		return nil, err
	}

	ch := make(chan string, 64)
	go func() {
		defer close(ch)
		done := false
		for ev := range events {
			if ev.Error != nil {
				ch <- sseJSON(map[string]any{"type": "error", "error": ev.Error.Message})
				continue
			}
			switch ev.Object {
			case "chat.completion.chunk":
				// 文本增量（finish_reason-only 的 chunk 无内容，自然跳过）
				if len(ev.Choices) > 0 && ev.Choices[0].Delta.Content != "" {
					ch <- sseText(ev.Choices[0].Delta.Content)
				}
			case "chat.completion":
				// 轮次汇总：只处理工具调用（文本已在 chunk 发过）
				if len(ev.Choices) > 0 {
					for _, tc := range ev.Choices[0].Message.ToolCalls {
						var args any
						if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
							args = tc.Function.Arguments
						}
						ch <- sseJSON(map[string]any{
							"type":         "tool_call",
							"tool":         tc.Function.Name,
							"args":         args,
							"tool_call_id": tc.ID,
						})
					}
				}
			case "tool.response":
				if len(ev.Choices) > 0 {
					msg := ev.Choices[0].Message
					ch <- sseJSON(map[string]any{
						"type":         "tool_result",
						"tool_call_id": msg.ToolID,
						"result":       decodeMaybeJSONString(msg.Content),
					})
				}
			case "runner.completion":
				ch <- "data: [DONE]\n\n"
				done = true
			}
			if done {
				return
			}
		}
		if !done {
			// 事件流意外关闭时兜底结束
			ch <- "data: [DONE]\n\n"
		}
	}()
	return ch, nil
}
