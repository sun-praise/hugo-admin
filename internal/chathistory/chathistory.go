// Package chathistory 封装聊天会话/消息的业务逻辑，
// 对齐 services/chat_history_service.py（含"新对话"自动标题）。
package chathistory

import (
	"regexp"
	"strings"

	"github.com/svtter/hugo-admin/internal/db"
)

const defaultTitle = "新对话"

type Service struct {
	db *db.DB
}

func New(database *db.DB) *Service { return &Service{db: database} }

// CreateSession 对齐 create_session：空标题用默认"新对话"。
func (s *Service) CreateSession(title string) map[string]any {
	if title == "" {
		title = defaultTitle
	}
	session, err := s.db.CreateChatSession(title)
	if err != nil {
		return nil
	}
	return map[string]any{
		"session_id":    session.ID,
		"title":         session.Title,
		"created_at":    session.CreatedAt,
		"updated_at":    session.UpdatedAt,
		"message_count": 0,
	}
}

// AddMessage 对齐 add_message：首条用户消息自动生成标题。
func (s *Service) AddMessage(sessionID, role, content string) map[string]any {
	msg, err := s.db.AddChatMessage(sessionID, role, content, "text")
	if err != nil {
		return nil
	}
	if role == "user" {
		if session, ok, _ := s.db.GetChatSession(sessionID); ok && session.Title == defaultTitle {
			s.db.UpdateChatSessionTitle(sessionID, generateTitle(content))
		}
	}
	return map[string]any{
		"message_id":   msg.ID,
		"session_id":   sessionID,
		"role":         role,
		"content":      content,
		"message_type": "text",
		"created_at":   msg.CreatedAt,
	}
}

// GetSession 对齐 get_session：会话 + 消息列表；不存在返回 nil。
func (s *Service) GetSession(sessionID string) map[string]any {
	session, ok, err := s.db.GetChatSession(sessionID)
	if err != nil || !ok {
		return nil
	}
	messages := sessionMessages(s.db, sessionID)
	return map[string]any{
		"session_id":    sessionID,
		"title":         session.Title,
		"created_at":    session.CreatedAt,
		"updated_at":    session.UpdatedAt,
		"message_count": len(messages),
		"messages":      messages,
	}
}

// ListSessions 对齐 list_sessions：updated_at 倒序，默认 20 条。
func (s *Service) ListSessions(limit int) []map[string]any {
	sessions, err := s.db.ListChatSessions()
	if err != nil {
		return []map[string]any{}
	}
	if limit <= 0 || limit > len(sessions) {
		limit = len(sessions)
	}
	out := make([]map[string]any, 0, limit)
	for _, session := range sessions[:limit] {
		out = append(out, map[string]any{
			"session_id":    session.ID,
			"title":         session.Title,
			"created_at":    session.CreatedAt,
			"updated_at":    session.UpdatedAt,
			"message_count": len(sessionMessages(s.db, session.ID)),
		})
	}
	return out
}

// DeleteSession 删除会话（幂等）。
func (s *Service) DeleteSession(sessionID string) bool {
	return s.db.DeleteChatSession(sessionID) == nil
}

func sessionMessages(database *db.DB, sessionID string) []db.ChatMessage {
	msgs, err := database.GetChatMessages(sessionID)
	if err != nil {
		return []db.ChatMessage{}
	}
	return msgs
}

var reWhitespace = regexp.MustCompile(`\s+`)

// generateTitle 对齐 _generate_title_from_content：压空白、截 50 字符。
func generateTitle(content string) string {
	title := reWhitespace.ReplaceAllString(strings.TrimSpace(content), " ")
	if len([]rune(title)) > 50 {
		runes := []rune(title)
		title = string(runes[:47]) + "..."
	}
	if title == "" {
		return defaultTitle
	}
	return title
}
