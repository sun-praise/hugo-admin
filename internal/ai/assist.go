package ai

// 选区辅助（selection assist）：翻译/润色/评价三任务的 prompt、
// 输入上限与流式一次性调用（无工具）。
// 移植自 v2 的 services/assist_service.py；评价维度按博客语境重写
// （断言过强/标题一致/术语一致/口语化/结构可读性）。

import (
	"context"
	"fmt"
	"strings"
	"time"

	"trpc.group/trpc-go/trpc-agent-go/agent/llmagent"
	"trpc.group/trpc-go/trpc-agent-go/event"
	"trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/model/anthropic"
	"trpc.group/trpc-go/trpc-agent-go/runner"
)

const (
	MaxAssistSelectedText = 5000
	MaxAssistContext      = 2000
	MaxAssistFullText     = 30000
	// AssistStreamTimeout 单任务整体预算（评价带全文，预算从宽；
	// 客户端断开由 request context 立即取消，不等超时）。
	AssistStreamTimeout = 2 * time.Minute
)

// AssistTasks 白名单（v2 routes 里 task ∈ translate|polish|review）。
func IsAssistTask(task string) bool {
	return task == "translate" || task == "polish" || task == "review"
}

const strictJSONOnly = "只输出严格 JSON：不要 markdown 代码块，不要 JSON 前后的任何说明文字。"

const assistTranslatePrompt = `你是一名专业译者，嵌入在博客 Markdown 编辑器中。
任务：翻译 <selection>：英文源文译为简体中文，中文源文译为英文；Markdown 语法、链接与代码块保持原样。
` + strictJSONOnly + `
Schema：
{"translation": string,
 "hard_words": [{"word": string, "source": string, "gloss": string}]}
- translation：完整译文。
- hard_words：0-6 个读者可能不熟的难词（术语、正式/书面词、易混词）。
  - word：译文中出现的原样子串（界面将据此逐字加下划线）。
  - source：原文中对应的词或短语。
  - gloss：一行简短释义。`

const assistPolishPrompt = `你是一名写作编辑，嵌入在博客 Markdown 编辑器中。
任务：仅改写 <selection>，返回恰好 3 个候选。
` + strictJSONOnly + `
Schema：
{"candidates": [
  {"style": "formal"|"concise"|"restructure", "note": string, "text": string}]}
- formal：更正式书面（动词精确，去口语化与自夸用语）。
- concise：更紧凑，保留全部信息，通常更短。
- restructure：句式重组（如拆长句、调整语序），语义不变。
- note：一句话说明改了什么。
- text：与选区同语言的完整改写文本，Markdown 语法保持原样。`

const assistReviewPrompt = `你是一名严格的博客文章审校，嵌入在 Markdown 编辑器中。
任务：结合完整 <document> 评审 <selection>，关注以下博客写作问题：
- overclaim：断言过强或无依据的事实性风险（如「最好」「绝对」「所有人都」）；
- title：与文章标题承诺不一致（答非所问、标题党）；
- terminology：术语前后不一致；
- colloquial：口语化或语气不当；
- structure：结构与可读性（段落过长、逻辑跳跃、冗余）。
` + strictJSONOnly + `
Schema：
{"issues": [
  {"type": "overclaim"|"title"|"terminology"|"colloquial"|"structure"|"other",
   "severity": "high"|"warn"|"info",
   "comment": string, "before": string|null, "after": string|null}]}
- 给出 2-5 条意见；comment 用简体中文撰写，引用讨论中的原文词句。
- 仅当存在局部可修复的问题时给出 before/after：before 必须逐字复制自正文
  （精确子串，编辑器将据此定位替换），after 为替换文本；否则两项均为 null。
- severity：high=事实风险或严重不一致；warn=建议修改；info=提示，至多一条 info 为正面评价。`

func assistSystemPrompt(task string) string {
	switch task {
	case "translate":
		return assistTranslatePrompt
	case "polish":
		return assistPolishPrompt
	case "review":
		return assistReviewPrompt
	}
	return ""
}

// BuildAssistPrompts 组装任务的 system/user prompt。
// review 以 full_text 为 <document>，其余任务用前后文。
func BuildAssistPrompts(task, selectedText, contextBefore, contextAfter, fullText string) (string, string, error) {
	var sys string
	switch task {
	case "translate":
		sys = assistTranslatePrompt
	case "polish":
		sys = assistPolishPrompt
	case "review":
		sys = assistReviewPrompt
	default:
		return "", "", fmt.Errorf("unsupported task: %s", task)
	}

	var parts []string
	if task == "review" {
		parts = append(parts, "<document>\n"+fullText+"\n</document>")
		parts = append(parts, "<selection>\n"+selectedText+"\n</selection>")
	} else {
		if contextBefore != "" {
			parts = append(parts, "<context_before>\n"+contextBefore+"\n</context_before>")
		}
		parts = append(parts, "<selection>\n"+selectedText+"\n</selection>")
		if contextAfter != "" {
			parts = append(parts, "<context_after>\n"+contextAfter+"\n</context_after>")
		}
	}
	return sys, strings.Join(parts, "\n\n"), nil
}

// AssistEvent 是 AssistStream 的流事件：Delta 为文本增量；
// 流上出现 Err 时流随即结束（消费方应回 error 帧而非伪装成功）。
type AssistEvent struct {
	Delta string
	Err   error
}

// AssistStream 运行指定任务的无工具流式调用，逐段产出文本增量。
// ctx 取消（客户端断开/超时）即中止上游请求；通道关闭即流结束。
func (s *Service) AssistStream(ctx context.Context, task, userPrompt string) (<-chan AssistEvent, error) {
	if !s.enabled {
		return nil, fmt.Errorf("AI service is not configured")
	}
	m := anthropic.New(s.model,
		anthropic.WithAPIKey(s.apiKey),
		anthropic.WithBaseURL(s.baseURL),
	)
	ag := llmagent.New("assist-"+task,
		llmagent.WithModel(m),
		llmagent.WithInstruction(assistSystemPrompt(task)),
		llmagent.WithGenerationConfig(model.GenerationConfig{Stream: true}),
	)
	sessionID := time.Now().Format("20060102-150405.000000000")
	events, err := runner.NewRunner("assist", ag).Run(ctx, "web", sessionID, model.NewUserMessage(userPrompt))
	if err != nil {
		return nil, err
	}

	ch := make(chan AssistEvent, 64)
	go func() {
		defer close(ch)
		send := func(ev AssistEvent) bool {
			// handler 可能因客户端断开停止读取：无条件发送会在缓冲写满后
			// 永久阻塞并拖住 runner；select ctx.Done() 保证可退出。
			select {
			case ch <- ev:
				return true
			case <-ctx.Done():
				return false
			}
		}
		for {
			var ev *event.Event
			var ok bool
			select {
			case ev, ok = <-events:
				if !ok {
					return
				}
			case <-ctx.Done():
				// runner 是否关闭 events 取决于库实现，不能假设；
				// 本 goroutine 必须能独立退出，避免泄漏。
				return
			}
			if ev.Error != nil {
				send(AssistEvent{Err: fmt.Errorf("%s", ev.Error.Message)})
				return
			}
			// 无工具调用，只关心文本增量；轮次汇总消息不重复下发。
			if ev.Object == "chat.completion.chunk" && len(ev.Choices) > 0 && ev.Choices[0].Delta.Content != "" {
				if !send(AssistEvent{Delta: ev.Choices[0].Delta.Content}) {
					return
				}
			}
		}
	}()
	return ch, nil
}
