package db

import (
	"database/sql"
	"time"
)

// chat_sessions / chat_messages，对齐 Python 的同名方法。
// 供 AI 批次的聊天历史使用。

type ChatSession struct {
	ID        string  `json:"id"`
	Title     string  `json:"title"`
	CreatedAt float64 `json:"created_at"`
	UpdatedAt float64 `json:"updated_at"`
}

type ChatMessage struct {
	ID          int64   `json:"id"`
	SessionID   string  `json:"session_id"`
	Role        string  `json:"role"`
	Content     string  `json:"content"`
	MessageType string  `json:"message_type"`
	CreatedAt   float64 `json:"created_at"`
}

func now() float64 { return float64(time.Now().UnixNano()) / 1e9 }

// CreateChatSession 对齐 create_chat_session（id 为 32 位 hex）。
func (d *DB) CreateChatSession(title string) (ChatSession, error) {
	ts := now()
	s := ChatSession{ID: newSessionID(), Title: title, CreatedAt: ts, UpdatedAt: ts}
	_, err := d.sql.Exec(
		`INSERT INTO chat_sessions (id, title, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		s.ID, s.Title, s.CreatedAt, s.UpdatedAt)
	return s, err
}

// AddChatMessage 对齐 add_chat_message：插入消息并刷新会话 updated_at。
func (d *DB) AddChatMessage(sessionID, role, content, messageType string) (ChatMessage, error) {
	if messageType == "" {
		messageType = "text"
	}
	ts := now()
	res, err := d.sql.Exec(
		`INSERT INTO chat_messages (session_id, role, content, message_type, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		sessionID, role, content, messageType, ts)
	if err != nil {
		return ChatMessage{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return ChatMessage{}, err
	}
	if _, err := d.sql.Exec(
		`UPDATE chat_sessions SET updated_at = ? WHERE id = ?`, ts, sessionID); err != nil {
		return ChatMessage{}, err
	}
	return ChatMessage{
		ID: id, SessionID: sessionID, Role: role,
		Content: content, MessageType: messageType, CreatedAt: ts,
	}, nil
}

// GetChatSession 对齐 get_chat_session。
func (d *DB) GetChatSession(sessionID string) (ChatSession, bool, error) {
	var s ChatSession
	err := d.sql.QueryRow(
		`SELECT id, title, created_at, updated_at FROM chat_sessions WHERE id = ?`, sessionID,
	).Scan(&s.ID, &s.Title, &s.CreatedAt, &s.UpdatedAt)
	if err == sql.ErrNoRows {
		return ChatSession{}, false, nil
	}
	if err != nil {
		return ChatSession{}, false, err
	}
	return s, true, nil
}

// ListChatSessions 按 updated_at 倒序。
func (d *DB) ListChatSessions() ([]ChatSession, error) {
	rows, err := d.sql.Query(
		`SELECT id, title, created_at, updated_at FROM chat_sessions ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ChatSession{}
	for rows.Next() {
		var s ChatSession
		if err := rows.Scan(&s.ID, &s.Title, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// GetChatMessages 按 created_at 升序。
func (d *DB) GetChatMessages(sessionID string) ([]ChatMessage, error) {
	rows, err := d.sql.Query(
		`SELECT id, session_id, role, content, message_type, created_at
		 FROM chat_messages WHERE session_id = ? ORDER BY created_at ASC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ChatMessage{}
	for rows.Next() {
		var m ChatMessage
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Role, &m.Content, &m.MessageType, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// DeleteChatSession 删除会话及其所有消息。
func (d *DB) DeleteChatSession(sessionID string) error {
	if _, err := d.sql.Exec(`DELETE FROM chat_messages WHERE session_id = ?`, sessionID); err != nil {
		return err
	}
	_, err := d.sql.Exec(`DELETE FROM chat_sessions WHERE id = ?`, sessionID)
	return err
}

// UpdateChatSessionTitle 更新会话标题。
func (d *DB) UpdateChatSessionTitle(sessionID, title string) error {
	_, err := d.sql.Exec(`UPDATE chat_sessions SET title = ? WHERE id = ?`, title, sessionID)
	return err
}
