package httpapi

// POST /api/ai/assist/{task} —— 选区辅助 SSE（translate|polish|review）。
// 事件协议（对齐前端 AssistPanel/sse.ts）：
//   data: {"type":"start","task":...,"model":...}
//   data: {"type":"delta","task":...,"text":...}
//   data: {"type":"done","task":...,"result":{...}}   —— 规整后的任务契约
//   data: {"type":"error","task":...,"error":...}
// 客户端断开经 request context 直接取消上游请求。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/sun-praise/hugo-admin/v3/internal/ai"
)

// handleAssist 对齐 v2 routes/assist_routes.py 的校验与 SSE 语义。
func (s *Server) handleAssist(w http.ResponseWriter, r *http.Request) {
	task := r.PathValue("task")
	if !ai.IsAssistTask(task) {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"success": false, "message": fmt.Sprintf("unsupported task: %s", task)})
		return
	}
	if !s.aiSvc.Enabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"success": false,
			"message": "AI service not configured. Set DEEPSEEK_API_KEY to enable.",
		})
		return
	}

	data := jsonDict(r)
	selectedText := strings.TrimSpace(strOr(data["selected_text"]))
	if selectedText == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": "缺少 selected_text"})
		return
	}
	if len([]rune(selectedText)) > ai.MaxAssistSelectedText {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "message": fmt.Sprintf("selected_text 超过上限 %d 字符", ai.MaxAssistSelectedText)})
		return
	}
	contextBefore := strOr(data["context_before"])
	contextAfter := strOr(data["context_after"])
	if len([]rune(contextBefore)) > ai.MaxAssistContext || len([]rune(contextAfter)) > ai.MaxAssistContext {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"message": fmt.Sprintf("context_before/context_after 必须为不超过 %d 字符的字符串", ai.MaxAssistContext)})
		return
	}
	fullText := strOr(data["full_text"])
	if task == "review" {
		if strings.TrimSpace(fullText) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"success": false, "message": "review 任务必须提供 full_text"})
			return
		}
		// 整体拒绝而非截断：半篇上下文得出的评价意见有误导性。
		if len([]rune(fullText)) > ai.MaxAssistFullText {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"success": false,
				"message": fmt.Sprintf("full_text 超过上限 %d 字符，请缩短文章后再试", ai.MaxAssistFullText)})
			return
		}
	}

	_, user, err := ai.BuildAssistPrompts(task, selectedText, contextBefore, contextAfter, fullText)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"success": false, "message": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), ai.AssistStreamTimeout)
	defer cancel()
	events, err := s.aiSvc.AssistStream(ctx, task, user)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"success": false, "message": err.Error()})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	writeFrame := func(payload map[string]any) bool {
		b, err := json.Marshal(payload)
		if err != nil {
			return false
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	if !writeFrame(map[string]any{"type": "start", "task": task, "model": s.aiSvc.ModelName()}) {
		return
	}

	var parts []string
	clientGone := false
	for ev := range events {
		if ev.Err != nil {
			writeFrame(map[string]any{"type": "error", "task": task, "error": ev.Err.Error()})
			return
		}
		parts = append(parts, ev.Delta)
		if !writeFrame(map[string]any{"type": "delta", "task": task, "text": ev.Delta}) {
			clientGone = true
			break
		}
	}
	if clientGone {
		return
	}
	// 上下文超时/断连后 runner 可能静默关闭事件流：不能把半截输出
	// 当完整结果规整；客户端已断开时不发帧，超时且客户端仍在时发 error。
	if ctx.Err() != nil {
		if r.Context().Err() == nil {
			writeFrame(map[string]any{"type": "error", "task": task, "error": "assist stream timed out"})
		}
		return
	}

	obj := ai.ExtractJSON(strings.Join(parts, ""))
	if obj == nil {
		writeFrame(map[string]any{"type": "error", "task": task, "error": "model output contains no JSON object"})
		return
	}
	result, err := ai.NormalizeFor(task, obj)
	if err != nil {
		writeFrame(map[string]any{"type": "error", "task": task, "error": err.Error()})
		return
	}
	writeFrame(map[string]any{"type": "done", "task": task, "result": result})
}
