// Package db 提供与 Python models/database.py 同 schema 的 SQLite
// 存取层（modernc.org/sqlite，纯 Go 无 CGO）。两套实现可并行读写
// 同一数据库文件（灰度期互操作）。本包覆盖 git_push_history 与
// chat_sessions/chat_messages；posts 缓存与 post_references 表
// 待后续批次。
package db

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	sql *sql.DB
}

// Open 打开（必要时创建）数据库并确保 schema（DDL 与 Python 一致）。
func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	handle, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	d := &DB{sql: handle}
	if err := d.initSchema(); err != nil {
		handle.Close()
		return nil, err
	}
	return d, nil
}

func (d *DB) Close() error { return d.sql.Close() }

func (d *DB) initSchema() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS posts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			file_path TEXT UNIQUE NOT NULL,
			relative_path TEXT NOT NULL,
			title TEXT NOT NULL,
			date TEXT,
			description TEXT,
			excerpt TEXT,
			cover TEXT DEFAULT '',
			tags TEXT,
			categories TEXT,
			mod_time REAL NOT NULL,
			cached_at REAL NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_file_path ON posts(file_path)`,
		`CREATE INDEX IF NOT EXISTS idx_mod_time ON posts(mod_time)`,
		`CREATE INDEX IF NOT EXISTS idx_date ON posts(date)`,
		`CREATE INDEX IF NOT EXISTS idx_tags ON posts(tags)`,
		`CREATE INDEX IF NOT EXISTS idx_categories ON posts(categories)`,
		`CREATE TABLE IF NOT EXISTS chat_sessions (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			created_at REAL NOT NULL,
			updated_at REAL NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS chat_messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			message_type TEXT DEFAULT 'text',
			created_at REAL NOT NULL,
			FOREIGN KEY (session_id) REFERENCES chat_sessions(id)
		)`,
		`CREATE TABLE IF NOT EXISTS post_references (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source_path TEXT NOT NULL,
			target_path TEXT NOT NULL,
			context TEXT DEFAULT '',
			UNIQUE(source_path, target_path)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ref_source ON post_references(source_path)`,
		`CREATE INDEX IF NOT EXISTS idx_ref_target ON post_references(target_path)`,
		`CREATE INDEX IF NOT EXISTS idx_session_id ON chat_messages(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_updated_at ON chat_sessions(updated_at)`,
		`CREATE TABLE IF NOT EXISTS git_push_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			remote TEXT NOT NULL,
			branch TEXT NOT NULL,
			from_sha TEXT DEFAULT '',
			to_sha TEXT DEFAULT '',
			commit_count INTEGER DEFAULT 0,
			commit_message TEXT DEFAULT '',
			success INTEGER NOT NULL,
			message TEXT DEFAULT '',
			pushed_at REAL NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_pushed_at ON git_push_history(pushed_at)`,
	}
	for _, stmt := range stmts {
		if _, err := d.sql.Exec(stmt); err != nil {
			return fmt.Errorf("init schema: %w", err)
		}
	}
	// 迁移：为旧库补 posts.cover（对齐 _migrate_db）
	var hasCover bool
	row := d.sql.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('posts') WHERE name='cover'`)
	if err := row.Scan(&hasCover); err == nil && !hasCover {
		if _, err := d.sql.Exec(`ALTER TABLE posts ADD COLUMN cover TEXT DEFAULT ''`); err != nil {
			return fmt.Errorf("migrate posts.cover: %w", err)
		}
	}
	return nil
}

// ============ git_push_history ============

// RecordPush 实现 git.PushRecorder，写入一条推送历史并返回行 id。
func (d *DB) RecordPush(remote, branch, fromSHA, toSHA string, commitCount int, commitMessage, message string, success bool) {
	if _, err := d.recordPush(remote, branch, fromSHA, toSHA, commitCount, commitMessage, message, success); err != nil {
		// 对齐 Python：记录失败只记日志，绝不影响推送结果
		fmt.Printf("记录推送历史失败: %v\n", err)
	}
}

func (d *DB) recordPush(remote, branch, fromSHA, toSHA string, commitCount int, commitMessage, message string, success bool) (int64, error) {
	res, err := d.sql.Exec(`
		INSERT INTO git_push_history
			(remote, branch, from_sha, to_sha, commit_count,
			 commit_message, success, message, pushed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		remote, branch, fromSHA, toSHA, commitCount,
		commitMessage, boolToInt(success), message, float64(time.Now().UnixNano())/1e9)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// PushRecord 对齐 list_pushes 的行结构。
type PushRecord struct {
	ID            int64   `json:"id"`
	Remote        string  `json:"remote"`
	Branch        string  `json:"branch"`
	FromSHA       string  `json:"from_sha"`
	ToSHA         string  `json:"to_sha"`
	CommitCount   int     `json:"commit_count"`
	CommitMessage string  `json:"commit_message"`
	Success       bool    `json:"success"`
	Message       string  `json:"message"`
	PushedAt      float64 `json:"pushed_at"`
	PushedAtISO   string  `json:"pushed_at_iso"`
}

// ListPushes 按 pushed_at 倒序返回推送历史与总数。
func (d *DB) ListPushes(limit, offset int) ([]PushRecord, int, error) {
	var total int
	if err := d.sql.QueryRow(`SELECT COUNT(*) FROM git_push_history`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := d.sql.Query(`
		SELECT id, remote, branch, from_sha, to_sha, commit_count,
		       commit_message, success, message, pushed_at
		FROM git_push_history
		ORDER BY pushed_at DESC, id DESC
		LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	pushes := []PushRecord{}
	for rows.Next() {
		var p PushRecord
		var success int
		if err := rows.Scan(&p.ID, &p.Remote, &p.Branch, &p.FromSHA, &p.ToSHA,
			&p.CommitCount, &p.CommitMessage, &success, &p.Message, &p.PushedAt); err != nil {
			return nil, 0, err
		}
		p.Success = success == 1
		p.PushedAtISO = time.Unix(int64(p.PushedAt), 0).UTC().Format("2006-01-02T15:04:05-07:00")
		pushes = append(pushes, p)
	}
	return pushes, total, rows.Err()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func newSessionID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ============ post_references ============

// RefEntry 是一条引用关系。
type RefEntry struct {
	TargetPath string `json:"target_path"`
	Context    string `json:"context"`
}

// UpsertReferences 对齐 upsert_references：替换某源文件的全部引用。
func (d *DB) UpsertReferences(sourcePath string, refs []RefEntry) error {
	if _, err := d.sql.Exec(`DELETE FROM post_references WHERE source_path = ?`, sourcePath); err != nil {
		return err
	}
	for _, ref := range refs {
		if _, err := d.sql.Exec(
			`INSERT OR IGNORE INTO post_references (source_path, target_path, context) VALUES (?, ?, ?)`,
			sourcePath, ref.TargetPath, ref.Context); err != nil {
			return err
		}
	}
	return nil
}

// BatchUpsertReferences 对齐 batch_upsert_references。
func (d *DB) BatchUpsertReferences(all map[string][]RefEntry) error {
	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	for source, refs := range all {
		if _, err := tx.Exec(`DELETE FROM post_references WHERE source_path = ?`, source); err != nil {
			tx.Rollback()
			return err
		}
		for _, ref := range refs {
			if _, err := tx.Exec(
				`INSERT OR IGNORE INTO post_references (source_path, target_path, context) VALUES (?, ?, ?)`,
				source, ref.TargetPath, ref.Context); err != nil {
				tx.Rollback()
				return err
			}
		}
	}
	return tx.Commit()
}

// Backlink 对齐 get_backlinks 的返回行。
type Backlink struct {
	Path    string `json:"path"`
	Title   string `json:"title"`
	Context string `json:"context"`
}

// GetBacklinks 对齐 get_backlinks：JOIN posts 表取标题。
// Go 版 posts 表由 UpsertPost 填充（引用扫描时写入）。
func (d *DB) GetBacklinks(targetPath string) ([]Backlink, error) {
	rows, err := d.sql.Query(`
		SELECT p.relative_path, COALESCE(p.title, ''), COALESCE(pr.context, '')
		FROM post_references pr
		LEFT JOIN posts p ON pr.source_path = p.file_path
		WHERE pr.target_path = ?
		ORDER BY p.date DESC`, targetPath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Backlink{}
	for rows.Next() {
		var b Backlink
		if err := rows.Scan(&b.Path, &b.Title, &b.Context); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// GetAllReferences 对齐 get_all_references。
func (d *DB) GetAllReferences() (map[string][]string, error) {
	rows, err := d.sql.Query(`SELECT source_path, target_path FROM post_references`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var src, tgt string
		if err := rows.Scan(&src, &tgt); err != nil {
			return nil, err
		}
		out[src] = append(out[src], tgt)
	}
	return out, rows.Err()
}

// UpsertPost 对齐 upsert_post（引用扫描时维护 posts 缓存行，
// 供 backlinks 的标题 JOIN 与搜索使用）。
func (d *DB) UpsertPost(filePath, relativePath, title, date, description, excerpt, cover string, tags, categories []string, modTime float64) error {
	tagsJSON, _ := json.Marshal(tags)
	catsJSON, _ := json.Marshal(categories)
	_, err := d.sql.Exec(`
		INSERT OR REPLACE INTO posts
		(file_path, relative_path, title, date, description, excerpt, cover,
		 tags, categories, mod_time, cached_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		filePath, relativePath, title, date, description, excerpt, cover,
		string(tagsJSON), string(catsJSON), modTime, float64(time.Now().UnixNano())/1e9)
	return err
}

// SearchPostRow 是搜索结果行。
type SearchPostRow struct {
	RelativePath string
	Title        string
	Description  string
	Excerpt      string
	Tags         string
	Categories   string
}

// SearchPostsLike 对齐 search_posts 的 LIKE 查询。
func (d *DB) SearchPostsLike(query, category, tag string) ([]SearchPostRow, error) {
	sql := `SELECT relative_path, title, description, excerpt, tags, categories FROM posts WHERE 1=1`
	var args []any
	if query != "" {
		sql += ` AND (title LIKE ? OR description LIKE ? OR excerpt LIKE ? OR relative_path LIKE ?)`
		term := "%" + query + "%"
		args = append(args, term, term, term, term)
	}
	if category != "" {
		sql += ` AND categories LIKE ?`
		args = append(args, fmt.Sprintf(`%%"%s"%%`, category))
	}
	if tag != "" {
		sql += ` AND tags LIKE ?`
		args = append(args, fmt.Sprintf(`%%"%s"%%`, tag))
	}
	sql += ` ORDER BY date DESC`
	rows, err := d.sql.Query(sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SearchPostRow{}
	for rows.Next() {
		var r SearchPostRow
		if err := rows.Scan(&r.RelativePath, &r.Title, &r.Description, &r.Excerpt, &r.Tags, &r.Categories); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
