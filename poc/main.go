// Go 重写 PoC：验证三件事
//  1. SSE 替代 flask-socketio 的服务端推送（tts 进度 / server_log）
//  2. request_logs socket 事件降级为 REST
//  3. trpc-agent-go + DeepSeek Anthropic 兼容端点替代 claude-agent-sdk，
//     自定义工具进程内注册，流式输出
package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"trpc.group/trpc-go/trpc-agent-go/agent/llmagent"
	"trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/model/anthropic"
	"trpc.group/trpc-go/trpc-agent-go/runner"
	"trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

//go:embed index.html
var indexHTML []byte

const defaultBaseURL = "https://api.deepseek.com/anthropic"

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// --- SSE broker：对标 socketio.emit 的广播语义 ---

type sseEvent struct {
	name string
	data string
}

type broker struct {
	mu   sync.Mutex
	subs map[chan sseEvent]struct{}
}

func newBroker() *broker {
	return &broker{subs: make(map[chan sseEvent]struct{})}
}

func (b *broker) subscribe() chan sseEvent {
	ch := make(chan sseEvent, 32)
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subs[ch] = struct{}{}
	return ch
}

func (b *broker) unsubscribe(ch chan sseEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.subs, ch)
}

func (b *broker) broadcast(name string, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- sseEvent{name: name, data: string(data)}:
		default: // 慢订阅者直接丢帧，不阻塞广播
		}
	}
}

// --- 日志环形缓冲：对标 hugo_manager.get_recent_logs ---

type logBuffer struct {
	mu    sync.Mutex
	lines []string
	max   int
}

func (l *logBuffer) add(line string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, time.Now().Format("15:04:05")+" "+line)
	if len(l.lines) > l.max {
		l.lines = l.lines[len(l.lines)-l.max:]
	}
}

func (l *logBuffer) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]string, len(l.lines))
	copy(out, l.lines)
	return out
}

// --- 演示工具：替代 create_sdk_mcp_server 注册的 hugo 自定义工具 ---

type recentPost struct {
	Title string   `json:"title"`
	Date  string   `json:"date"`
	Tags  []string `json:"tags"`
}

type listPostsReq struct {
	Limit int `json:"limit,omitempty" jsonschema:"description=返回文章数量上限，默认 3"`
}

func listRecentPosts(ctx context.Context, req listPostsReq) ([]recentPost, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 3
	}
	all := []recentPost{
		{Title: "用 Go 重写 Flask 服务的实践", Date: "2026-10-01", Tags: []string{"go", "重构"}},
		{Title: "SSE 与 WebSocket 的取舍", Date: "2026-09-12", Tags: []string{"web", "实时推送"}},
		{Title: "Hugo 多项目内容管理笔记", Date: "2026-08-30", Tags: []string{"hugo", "博客"}},
	}
	if limit > len(all) {
		limit = len(all)
	}
	return all[:limit], nil
}

// --- HTTP 服务 ---

type server struct {
	broker    *broker
	logs      *logBuffer
	aiRunner  runner.Runner
	aiEnabled bool
	aiModel   string
}

func writeSSE(w http.ResponseWriter, name, data string) {
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, data)
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":     "ok",
		"ai_enabled": s.aiEnabled,
		"ai_model":   s.aiModel,
	})
}

func (s *server) handleLogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"logs": s.logs.snapshot()})
}

func (s *server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	ch := s.broker.subscribe()
	defer s.broker.unsubscribe(ch)
	s.logs.add("SSE 客户端连接")

	writeSSE(w, "connected", `{"message":"已连接到服务器"}`)
	flusher.Flush()

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			s.logs.add("SSE 客户端断开")
			return
		case <-heartbeat.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case ev := <-ch:
			writeSSE(w, ev.name, ev.data)
			flusher.Flush()
		}
	}
}

func (s *server) handleTTSDemo(w http.ResponseWriter, r *http.Request) {
	go func() {
		for i := 0; i <= 100; i += 10 {
			s.broker.broadcast("tts_progress", map[string]int{"percent": i})
			time.Sleep(200 * time.Millisecond)
		}
		s.broker.broadcast("tts_done", map[string]string{"status": "ok"})
		s.logs.add("TTS 演示任务完成")
	}()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "started"})
}

type rewriteReq struct {
	Text string `json:"text"`
}

func (s *server) handleAIRewrite(w http.ResponseWriter, r *http.Request) {
	if !s.aiEnabled {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "AI 未配置：请设置 ANTHROPIC_API_KEY（或 DEEPSEEK_API_KEY）后重启"})
		return
	}

	var req rewriteReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Text == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "请求体需要 {\"text\": \"...\"}"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()

	prompt := "请润色以下博客片段，保持 Markdown 结构和中文表达习惯：\n\n" + req.Text
	events, err := s.aiRunner.Run(ctx, "poc-user", fmt.Sprintf("sess-%d", time.Now().UnixNano()),
		model.NewUserMessage(prompt))
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
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

	toolSeen := map[string]bool{}
	for evt := range events {
		if evt.Error != nil {
			writeSSE(w, "error", fmt.Sprintf(`{"message":%q}`, evt.Error.Message))
			flusher.Flush()
			continue
		}
		if evt.Object != "chat.completion.chunk" || len(evt.Choices) == 0 {
			continue
		}
		delta := evt.Choices[0].Delta
		for _, tc := range delta.ToolCalls {
			if tc.Function.Name != "" && !toolSeen[tc.Function.Name] {
				toolSeen[tc.Function.Name] = true
				writeSSE(w, "ai_tool", fmt.Sprintf(`{"name":%q}`, tc.Function.Name))
				flusher.Flush()
			}
		}
		if delta.Content != "" {
			writeSSE(w, "ai_delta", fmt.Sprintf(`{"text":%q}`, delta.Content))
			flusher.Flush()
		}
	}
	writeSSE(w, "done", `{"status":"ok"}`)
	flusher.Flush()
	s.logs.add("AI 改写完成")
}

func newAIRunner(apiKey, baseURL, modelName string) (runner.Runner, error) {
	m := anthropic.New(modelName,
		anthropic.WithAPIKey(apiKey),
		anthropic.WithBaseURL(baseURL),
	)
	postsTool := function.NewFunctionTool(listRecentPosts,
		function.WithName("list_recent_posts"),
		function.WithDescription("列出博客最近的已发布文章（标题/日期/标签），用于对齐写作风格"),
	)
	ag := llmagent.New("hugo-editor",
		llmagent.WithModel(m),
		llmagent.WithTools([]tool.Tool{postsTool}),
		llmagent.WithInstruction(
			"你是 Hugo 博客的中文编辑。改写时保留原意，行文简洁自然，不堆砌形容词；"+
				"如需了解近期文章的风格，可调用 list_recent_posts。"),
		llmagent.WithGenerationConfig(model.GenerationConfig{Stream: true}),
	)
	return runner.NewRunner("hugo-admin-poc", ag), nil
}

func main() {
	apiKey := envOr("ANTHROPIC_API_KEY", os.Getenv("DEEPSEEK_API_KEY"))
	baseURL := envOr("ANTHROPIC_BASE_URL", defaultBaseURL)
	modelName := envOr("ANTHROPIC_MODEL", "deepseek-chat")
	addr := ":" + envOr("PORT", "8931")

	s := &server{
		broker:  newBroker(),
		logs:    &logBuffer{max: 200},
		aiModel: modelName,
	}
	s.logs.add("PoC 服务启动")

	if apiKey != "" {
		r, err := newAIRunner(apiKey, baseURL, modelName)
		if err != nil {
			log.Fatalf("初始化 AI runner: %v", err)
		}
		defer r.Close()
		s.aiRunner = r
		s.aiEnabled = true
		log.Printf("AI 已启用: model=%s base=%s", modelName, baseURL)
	} else {
		log.Print("AI 未启用：未检测到 ANTHROPIC_API_KEY / DEEPSEEK_API_KEY")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/server/logs", s.handleLogs)
	mux.HandleFunc("GET /api/events", s.handleEvents)
	mux.HandleFunc("POST /api/tts/demo", s.handleTTSDemo)
	mux.HandleFunc("POST /api/ai/rewrite", s.handleAIRewrite)
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(indexHTML)
	})

	log.Printf("listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
