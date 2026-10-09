package httpapi

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/svtter/hugo-admin/internal/ai"
)

// 对齐 routes/ai_routes.py 与 routes/inline_edit_routes.py。
// /api/frontmatter/generate 待 frontmatter 生成批次迁移。

var reSafeCurrentFile = regexp.MustCompile(`^[\w\-./]+$`)

// GET /api/ai/sessions
func (s *Server) handleAIListSessions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true, "sessions": s.chat.ListSessions(20),
	})
}

// POST /api/ai/sessions {title?}
func (s *Server) handleAICreateSession(w http.ResponseWriter, r *http.Request) {
	data := jsonDict(r)
	title, _ := data["title"].(string)
	session := s.chat.CreateSession(title)
	writeJSON(w, http.StatusCreated, map[string]any{
		"success":       true,
		"session_id":    session["session_id"],
		"title":         session["title"],
		"created_at":    session["created_at"],
		"updated_at":    session["updated_at"],
		"message_count": session["message_count"],
	})
}

// GET /api/ai/sessions/{id}
func (s *Server) handleAIGetSession(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	session := s.chat.GetSession(sessionID)
	if session == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"success": false, "message": "Session not found",
		})
		return
	}
	resp := map[string]any{"success": true}
	for k, v := range session {
		resp[k] = v
	}
	writeJSON(w, http.StatusOK, resp)
}

// DELETE /api/ai/sessions/{id}
func (s *Server) handleAIDeleteSession(w http.ResponseWriter, r *http.Request) {
	s.chat.DeleteSession(r.PathValue("id"))
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true, "message": "Session deleted",
	})
}

// POST /api/ai/chat —— SSE 流式（协议见 internal/ai/sse.go）
func (s *Server) handleAIChat(w http.ResponseWriter, r *http.Request) {
	if !s.aiSvc.Enabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"success": false,
			"message": "AI service not configured. Set DEEPSEEK_API_KEY to enable.",
		})
		return
	}
	data := jsonDict(r)
	message, _ := data["message"].(string)
	sessionID, _ := data["session_id"].(string)
	currentFile, _ := data["current_file"].(string)
	currentPage, _ := data["current_page"].(string)

	if message == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "message": "缺少消息内容",
		})
		return
	}
	if currentFile != "" && !reSafeCurrentFile.MatchString(currentFile) {
		currentFile = ""
	}

	// 上下文前缀注入（对齐 stream_agent_as_sse_sync）
	var parts []string
	if currentPage != "" {
		parts = append(parts, "当前页面: "+currentPage)
	}
	if currentFile != "" {
		parts = append(parts, "当前聚焦文章: "+currentFile)
	}
	if len(parts) > 0 {
		message = "[" + strings.Join(parts, ", ") + "]\n\n" + message
	}

	if sessionID != "" {
		if s.chat.AddMessage(sessionID, "user", message) == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{
				"success": false, "message": "Failed to save message",
			})
			return
		}
	}

	frames, err := s.aiSvc.ChatSSE(r.Context(), message)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"success": false, "message": err.Error(),
		})
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

	assistant := ""
	for frame := range frames {
		fmt.Fprint(w, frame)
		flusher.Flush()
		// 累积纯文本帧用于会话存储（对齐 ai_routes.generate 的提取逻辑）
		if payload, ok := strings.CutPrefix(frame, "data: "); ok {
			payload = strings.TrimSuffix(payload, "\n\n")
			if payload != "[DONE]" && !strings.HasPrefix(payload, "{") {
				unescaped := strings.ReplaceAll(payload, "\\n", "\n")
				assistant += unescaped
			}
		}
	}
	if sessionID != "" && assistant != "" {
		s.chat.AddMessage(sessionID, "assistant", assistant)
	}
}

// POST /api/ai/inline-edit —— 非流式改写
func (s *Server) handleInlineEdit(w http.ResponseWriter, r *http.Request) {
	data := jsonDict(r)
	selectedText := strings.TrimSpace(strOr(data["selected_text"]))
	instruction := strings.TrimSpace(strOr(data["instruction"]))
	contextBefore := strings.TrimSpace(strOr(data["context_before"]))
	contextAfter := strings.TrimSpace(strOr(data["context_after"]))

	if selectedText == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": "缺少 selected_text"})
		return
	}
	if instruction == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": "缺少 instruction"})
		return
	}
	if len([]rune(selectedText)) > ai.MaxSelectedText {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "message": fmt.Sprintf("selected_text 超过 %d 字符限制", ai.MaxSelectedText)})
		return
	}
	if len([]rune(instruction)) > ai.MaxInstruction {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "message": fmt.Sprintf("instruction 超过 %d 字符限制", ai.MaxInstruction)})
		return
	}
	if len([]rune(contextBefore)) > ai.MaxContext || len([]rune(contextAfter)) > ai.MaxContext {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "message": fmt.Sprintf("context 超过 %d 字符限制", ai.MaxContext)})
		return
	}

	if !s.aiSvc.Enabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"success": false,
			"message": "AI service not configured. Set DEEPSEEK_API_KEY to enable.",
		})
		return
	}

	sys, user := ai.BuildInlinePrompts(selectedText, instruction, contextBefore, contextAfter)
	revised, err := s.aiSvc.QuickRewrite(r.Context(), sys, user, 10*1e9)
	if err != nil {
		if strings.Contains(err.Error(), "empty") {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"success": false, "message": "改写失败：模型无输出"})
			return
		}
		if r.Context().Err() != nil {
			writeJSON(w, http.StatusGatewayTimeout, map[string]any{"success": false, "message": "改写超时，请稍后重试"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"success": false, "message": "改写失败，请稍后重试"})
		return
	}

	revised = ai.UnwrapSingleFence(revised)
	if strings.TrimSpace(revised) == "" {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"success": false, "message": "改写失败：模型无输出"})
		return
	}
	if ai.IsDangerousResponse(revised) {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"success": false, "message": "改写失败：模型返回不安全内容"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success":      true,
		"revised_text": revised,
		"model":        s.aiSvc.ModelName(),
	})
}

func strOr(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
