// Package httpapi 是 Go 重写的 HTTP 层骨架：全局认证守卫、auth/version
// 路由、SSE 事件通道与静态资源服务。路由语义与 Python 侧逐条对齐，
// 契约样本见 contracts/api_samples.jsonl。
package httpapi

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/svtter/hugo-admin/internal/ai"
	"github.com/svtter/hugo-admin/internal/auth"
	"github.com/svtter/hugo-admin/internal/chathistory"
	"github.com/svtter/hugo-admin/internal/config"
	"github.com/svtter/hugo-admin/internal/db"
	"github.com/svtter/hugo-admin/internal/git"
	"github.com/svtter/hugo-admin/internal/hugo"
	"github.com/svtter/hugo-admin/internal/plugin"
	"github.com/svtter/hugo-admin/internal/realtime"
	"github.com/svtter/hugo-admin/internal/refs"
	"github.com/svtter/hugo-admin/internal/settings"
)

// publicAPIPath 对应 Python auth_routes.PUBLIC_API_PATHS；
// /api/health 是 Go 侧新增的运维端点，同样公开。
var publicAPIPath = map[string]bool{
	"/api/auth/login": true,
	"/api/auth/me":    true,
	"/api/version":    true,
	"/api/health":     true,
}

// Options 聚合可选服务依赖（nil 时对应端点返回 500/503）。
type Options struct {
	Git       *git.Service
	Hugo      *hugo.Manager
	Database  *db.DB
	AI        *ai.Service
	Chat      *chathistory.Service
	Plugins   *plugin.Manager
	Refs      *refs.Service
	Settings  *settings.Service
	EnvAPIKey string
}

type Server struct {
	cfg           *config.Config
	store         *auth.Store
	broker        *realtime.Broker
	gitSvc        *git.Service
	hugo          *hugo.Manager
	database      *db.DB
	aiSvc         *ai.Service
	chat          *chathistory.Service
	pluginMgr     *plugin.Manager
	refsSvc       *refs.Service
	settingsSvc   *settings.Service
	sessionAPIKey string
	envAPIKey     string
	mux           *http.ServeMux
}

func New(cfg *config.Config, store *auth.Store, broker *realtime.Broker, opts Options) *Server {
	s := &Server{cfg: cfg, store: store, broker: broker, mux: http.NewServeMux(),
		gitSvc: opts.Git, hugo: opts.Hugo, database: opts.Database,
		aiSvc: opts.AI, chat: opts.Chat, pluginMgr: opts.Plugins,
		refsSvc: opts.Refs, settingsSvc: opts.Settings, envAPIKey: opts.EnvAPIKey}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.recoverMiddleware(s.guardMiddleware(s.logMiddleware(s.mux))).ServeHTTP(w, r)
}

func (s *Server) routes() {
	m := s.mux
	m.HandleFunc("GET /api/version", s.handleVersion)
	m.HandleFunc("GET /api/health", s.handleHealth)
	m.HandleFunc("POST /api/auth/login", s.handleLogin)
	m.HandleFunc("GET /api/auth/me", s.handleMe)
	m.HandleFunc("POST /api/auth/logout", s.handleLogout)
	m.HandleFunc("POST /api/auth/password", s.handlePassword)
	m.HandleFunc("GET /api/events", s.handleEvents)
	m.HandleFunc("GET /api/posts", s.handlePosts)
	m.HandleFunc("GET /api/posts/tags", s.handlePostTags)
	m.HandleFunc("GET /api/posts/categories", s.handlePostCategories)
	m.HandleFunc("POST /api/file/read", s.handleFileRead)
	m.HandleFunc("POST /api/file/read-with-frontmatter", s.handleFileReadWithFM)
	m.HandleFunc("POST /api/file/save", s.handleFileSave)
	m.HandleFunc("POST /api/post/create", s.handlePostCreate)
	m.HandleFunc("GET /api/git/status", s.handleGitStatus)
	m.HandleFunc("GET /api/git/commits", s.handleGitCommits)
	m.HandleFunc("GET /api/git/pushes", s.handleGitPushes)
	m.HandleFunc("POST /api/git/push", s.handleGitPush)
	m.HandleFunc("POST /api/publish/system", s.handlePublishSystem)
	m.HandleFunc("GET /api/article/status", s.handleArticleStatus)
	m.HandleFunc("POST /api/article/status/bulk", s.handleArticleStatusBulk)
	m.HandleFunc("POST /api/article/publish", s.handleArticlePublish)
	m.HandleFunc("POST /api/article/publish/bulk", s.handleArticlePublishBulk)
	m.HandleFunc("GET /api/server/status", s.handleServerStatus)
	m.HandleFunc("POST /api/server/start", s.handleServerStart)
	m.HandleFunc("POST /api/server/stop", s.handleServerStop)
	m.HandleFunc("GET /api/server/logs", s.handleServerLogs)
	m.HandleFunc("GET /api/ai/sessions", s.handleAIListSessions)
	m.HandleFunc("POST /api/ai/sessions", s.handleAICreateSession)
	m.HandleFunc("GET /api/ai/sessions/{id}", s.handleAIGetSession)
	m.HandleFunc("DELETE /api/ai/sessions/{id}", s.handleAIDeleteSession)
	m.HandleFunc("POST /api/ai/chat", s.handleAIChat)
	m.HandleFunc("POST /api/ai/inline-edit", s.handleInlineEdit)
	m.HandleFunc("POST /api/image/upload", s.handleImageUpload)
	m.HandleFunc("POST /api/image/list", s.handleImageList)
	m.HandleFunc("GET /api/plugins", s.handlePluginList)
	m.HandleFunc("GET /api/plugins/market", s.handlePluginMarket)
	m.HandleFunc("GET /api/plugins/{name}/config-schema", s.handlePluginConfigSchema)
	m.HandleFunc("GET /api/plugins/{name}/config", s.handlePluginConfigGet)
	m.HandleFunc("PUT /api/plugins/{name}/config", s.handlePluginConfigSet)
	m.HandleFunc("POST /api/plugins/{name}/enable", s.handlePluginEnable)
	m.HandleFunc("POST /api/plugins/{name}/disable", s.handlePluginDisable)
	m.HandleFunc("POST /api/plugins/{name}/image/upload", s.handlePluginImageUpload)
	m.HandleFunc("DELETE /api/plugins/{name}/image/{image_id}", s.handlePluginImageDelete)
	m.HandleFunc("POST /api/plugins/{name}/tts/generate", s.handlePluginTTSGenerate)
	m.HandleFunc("DELETE /api/plugins/{name}/tts/{audio_id}", s.handlePluginTTSDelete)
	m.HandleFunc("GET /api/settings", s.handleSettingsGet)
	m.HandleFunc("PUT /api/settings", s.handleSettingsPut)
	m.HandleFunc("GET /api/config", s.handleConfigList)
	m.HandleFunc("GET /api/config/{filename}", s.handleConfigGet)
	m.HandleFunc("PUT /api/config/{filename}", s.handleConfigPut)
	m.HandleFunc("GET /api/themes", s.handleThemeList)
	m.HandleFunc("GET /api/themes/available", s.handleThemeAvailable)
	m.HandleFunc("POST /api/themes/install", s.handleThemeInstall)
	m.HandleFunc("POST /api/themes/activate", s.handleThemeActivate)
	m.HandleFunc("POST /api/themes/preview", s.handleThemePreview)
	m.HandleFunc("GET /api/content/{filename...}", s.handleContentFile)
	m.HandleFunc("GET /api/article/tts/status", s.handleArticleTTSStatus)
	m.HandleFunc("POST /api/article/tts", s.handleArticleTTSGenerate)
	m.HandleFunc("DELETE /api/article/tts", s.handleArticleTTSDelete)
	m.HandleFunc("POST /api/email/push-latest", s.handleEmailPushLatest)
	m.HandleFunc("POST /api/email/push-article", s.handleEmailPushArticle)
	m.HandleFunc("GET /api/email/preview-latest", s.handleEmailPreviewLatest)
	m.HandleFunc("GET /api/email/preview-article", s.handleEmailPreviewArticle)
	m.HandleFunc("POST /api/references/scan", s.handleRefsScan)
	m.HandleFunc("GET /api/references/backlinks", s.handleRefsBacklinks)
	m.HandleFunc("GET /api/posts/search", s.handlePostsSearch)
	m.HandleFunc("POST /api/frontmatter/generate", s.handleFMGenerate)
	m.HandleFunc("POST /api/image/generate-cover", s.handleImageGenCover)
	m.HandleFunc("POST /api/article/import", s.handleArticleImport)
	m.HandleFunc("POST /api/project/init", s.handleProjectInit)
	m.HandleFunc("GET /api/project/active", s.handleProjectActive)
	m.HandleFunc("POST /api/project/active/reset", s.handleProjectActiveReset)
	m.HandleFunc("POST /api/project/clean-layouts", s.handleProjectCleanLayouts)
	m.HandleFunc("POST /api/cache/refresh", s.handleCacheRefresh)
	m.HandleFunc("GET /api/cache/stats", s.handleCacheStats)
	m.HandleFunc("GET /admin-ui/", s.handleStatic)
	m.HandleFunc("/", s.handleSPA)
}

// ============ 中间件 ============

func (s *Server) logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Microsecond))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Flush 透传给 SSE。
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *Server) recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("panic: %v (%s %s)", err, r.Method, r.URL.Path)
				writeJSON(w, http.StatusInternalServerError, map[string]any{
					"success": false, "message": "服务器内部错误",
				})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// guardMiddleware 复刻 Python install_auth_guard：未登录访问非白名单的
// /api/* 一律 401。
func (s *Server) guardMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") || publicAPIPath[r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}
		if _, ok := s.sessionUsername(r); !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]any{
				"success": false, "message": "未登录或会话已过期",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ============ 会话 ============

func (s *Server) sessionUsername(r *http.Request) (string, bool) {
	cookie, err := r.Cookie("session")
	if err != nil {
		return "", false
	}
	sess, err := auth.DecodeSession([]byte(s.cfg.SecretKey), cookie.Value, auth.PermanentSessionLifetime, time.Now())
	if err != nil {
		return "", false
	}
	username, ok := sess["username"].(string)
	return username, ok && username != ""
}

func (s *Server) setSessionCookie(w http.ResponseWriter, username string) {
	// 与 Flask login 的插入序一致（username 在前、_permanent 在后），
	// 产出与 Python bit 级相同的 cookie
	payload := fmt.Sprintf(`{"username":%s,"_permanent":true}`, strconv.Quote(username))
	value, err := auth.EncodeSessionRaw([]byte(s.cfg.SecretKey), []byte(payload), time.Now())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false, "message": "服务器内部错误",
		})
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    value,
		Path:     "/",
		MaxAge:   int(auth.PermanentSessionLifetime.Seconds()),
		Expires:  time.Now().Add(auth.PermanentSessionLifetime),
		HttpOnly: true,
	})
}

// ============ 路由处理 ============

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"version": s.cfg.Version})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

// jsonDict 复刻 Python _json_dict：非对象的 JSON 体一律视为空 dict。
func jsonDict(r *http.Request) map[string]any {
	var data any
	_ = json.NewDecoder(r.Body).Decode(&data)
	if m, ok := data.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	data := jsonDict(r)
	username, _ := data["username"].(string)
	password, _ := data["password"].(string)
	if username == "" || password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "message": "缺少用户名或密码",
		})
		return
	}
	if !s.store.Verify(username, password) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"success": false, "message": "用户名或密码错误",
		})
		return
	}
	s.setSessionCookie(w, username)
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true, "user": map[string]any{"username": username},
	})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	username, ok := s.sessionUsername(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"success": false, "message": "未登录",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true, "user": map[string]any{"username": username},
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name: "session", Value: "", Path: "/", MaxAge: -1,
		Expires: time.Unix(0, 0), HttpOnly: true,
	})
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *Server) handlePassword(w http.ResponseWriter, r *http.Request) {
	username, ok := s.sessionUsername(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"success": false, "message": "未登录",
		})
		return
	}
	data := jsonDict(r)
	current, _ := data["current_password"].(string)
	next, _ := data["new_password"].(string)
	if current == "" || next == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "message": "缺少当前密码或新密码",
		})
		return
	}
	if !s.store.Verify(username, current) {
		// 401 专用于"未登录"；当前密码错误属于校验失败，用 400（同 Python 注释）
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "message": "当前密码错误",
		})
		return
	}
	if err := s.store.SetPassword(username, next); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "message": err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	ch := s.broker.Subscribe()
	defer s.broker.Unsubscribe(ch)

	fmt.Fprint(w, "event: connected\ndata: {\"message\":\"已连接到服务器\"}\n\n")
	flusher.Flush()

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case ev := <-ch:
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Name, ev.Data)
			flusher.Flush()
		}
	}
}

// handleStatic 服务 /admin-ui/ 前端构建产物；缺失文件返回 JSON 404
// （与 Flask 原生 static 路由 + JSON 404 处理器的组合一致）。
func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	rel := strings.TrimPrefix(r.URL.Path, "/admin-ui/")
	clean := filepath.Clean("/" + rel)
	for _, part := range strings.Split(clean, "/") {
		if strings.HasPrefix(part, ".") {
			writeJSON(w, http.StatusForbidden, map[string]any{
				"success": false, "message": "访问被拒绝",
			})
			return
		}
	}
	full := filepath.Join(s.cfg.AdminUIDir, clean)
	if st, err := os.Stat(full); err != nil || st.IsDir() {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"success": false, "message": "静态文件不存在",
		})
		return
	}
	http.ServeFile(w, r, full)
}

// handleSPA 复刻 Python 404 处理器：/api/* → JSON 404，其余路径回退
// 到 React SPA 的 index.html。
func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"success": false, "message": "接口不存在",
		})
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	index := filepath.Join(s.cfg.AdminUIDir, "index.html")
	if _, err := os.Stat(index); err != nil {
		http.Error(w, "admin-ui not built", http.StatusNotFound)
		return
	}
	http.ServeFile(w, r, index)
}

// ============ 工具 ============

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}
