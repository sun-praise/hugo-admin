package httpapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/svtter/hugo-admin/internal/posts"
	"github.com/svtter/hugo-admin/internal/util"
	pb "github.com/svtter/hugo-admin/proto"
)

// 对齐 routes/tts_routes.py：文章级 TTS。
// 读正文（剥 frontmatter）→ tts_generation 插件生成（后台任务）→
// 乐观锁写回 frontmatter audio 字段；进度/结果经 SSE（tts.* 带 scope）。

const (
	fmAudio         = "audio"
	fmAudioDuration = "audio_duration_seconds"
	fmAudioFormat   = "audio_format"
	fmAudioID       = "_tts_audio_id"
)

func (s *Server) resolveTTSPlugin() (string, *pb.TTSGeneratorClient, bool) {
	if s.pluginMgr == nil {
		return "", nil, false
	}
	target := s.pluginMgr.FindPluginWithCapability("tts_generation")
	if target == nil {
		return "", nil, false
	}
	name, _ := target["name"].(string)
	conn := s.pluginMgr.Conn(name)
	if conn == nil {
		return "", nil, false
	}
	stub := pb.NewTTSGeneratorClient(conn)
	return name, &stub, true
}

// GET /api/article/tts/status —— 前端据此显隐按钮
func (s *Server) handleArticleTTSStatus(w http.ResponseWriter, r *http.Request) {
	available := false
	var name any
	voices := []any{}
	if nameStr, _, ok := s.resolveTTSPlugin(); ok {
		available = true
		name = nameStr
		if schema := s.pluginMgr.GetConfigSchema(nameStr); schema != nil {
			if props, ok := schema["properties"].(map[string]any); ok {
				if v, ok := props["voices"].(map[string]any); ok {
					if items, ok := v["items"].(map[string]any); ok {
						if enum, ok := items["enum"].([]any); ok {
							voices = enum
						}
					}
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true, "available": available, "plugin": name, "voices": voices,
	})
}

// POST /api/article/tts —— 后台任务 + SSE 事件（对齐 start_background_task）
func (s *Server) handleArticleTTSGenerate(w http.ResponseWriter, r *http.Request) {
	data := jsonDict(r)
	articlePath, _ := data["article_path"].(string)
	if articlePath == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": "缺少文章路径"})
		return
	}
	if s.pluginMgr == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": "插件系统未初始化"})
		return
	}
	if _, _, ok := s.resolveTTSPlugin(); !ok {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": "未找到支持 tts_generation 的插件"})
		return
	}

	text, _ := data["text"].(string)
	var expectedMtime *float64
	if text == "" {
		ok, body, _, mtime := posts.ReadFileWithFrontmatter(s.cfg.ContentDir, articlePath)
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": "读取文章失败"})
			return
		}
		text = body
		expectedMtime = &mtime
	}
	if trimSpaces(text) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": "文章内容为空"})
		return
	}

	eventScope := util.NewOperationID()
	options := map[string]any{}
	for _, k := range []string{"voice", "model", "speed", "format", "language"} {
		if v, ok := data[k]; ok && v != nil {
			options[k] = v
		}
	}

	// 后台任务（SSE 推送事件）
	go s.runArticleTTS(articlePath, text, options, expectedMtime, eventScope)

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true, "pending": true, "event_scope": eventScope,
	})
}

func (s *Server) emitTTS(event string, scope string, payload map[string]any) {
	full := map[string]any{"scope": scope}
	for k, v := range payload {
		full[k] = v
	}
	s.broker.Broadcast(event, full)
}

// runArticleTTS 对齐 _run_tts_with_emits。
func (s *Server) runArticleTTS(articlePath, text string, options map[string]any, expectedMtime *float64, eventScope string) {
	emit := func(event string, payload map[string]any) {
		s.emitTTS(event, eventScope, payload)
	}

	_, stubp, ok := s.resolveTTSPlugin()
	if !ok {
		emit("tts.failed", map[string]any{"message": "未找到支持 tts_generation 的插件"})
		return
	}
	stub := *stubp

	speed := 1.0
	if raw, ok := options["speed"]; ok && raw != nil {
		if v, ok := raw.(float64); ok {
			speed = v
		}
	}
	optStr := func(k string) string {
		v, _ := options[k].(string)
		return v
	}
	req := &pb.TTSRequest{
		Text: text, Voice: optStr("voice"), Model: optStr("model"),
		Speed: float32(speed), Format: optStr("format"), Language: optStr("language"),
		ArticlePath: articlePath,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	stream, err := stub.Generate(ctx, req)
	if err != nil {
		emit("tts.failed", map[string]any{"message": fmt.Sprintf("插件调用失败: %v", err)})
		return
	}
	var result *pb.TTSResult
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			emit("tts.failed", map[string]any{"message": fmt.Sprintf("插件调用失败: %v", err)})
			return
		}
		switch payload := resp.Payload.(type) {
		case *pb.TTSResponse_Progress:
			p := payload.Progress
			emit("tts.progress", map[string]any{
				"stage": p.Stage, "percent": p.Percent, "message": p.Message,
			})
		case *pb.TTSResponse_Result:
			result = payload.Result
		}
	}

	if result == nil || !result.Success {
		msg := "插件未返回结果"
		if result != nil {
			msg = result.Message
		}
		emit("tts.failed", map[string]any{"message": msg})
		return
	}

	// 写回 frontmatter（乐观锁）
	ok2, body, fm, _ := posts.ReadFileWithFrontmatter(s.cfg.ContentDir, articlePath)
	if !ok2 {
		emit("tts.failed", map[string]any{"message": "读取文章失败，无法写回 audio 字段"})
		return
	}
	fm[fmAudio] = result.Url
	if result.DurationSeconds > 0 {
		fm[fmAudioDuration] = result.DurationSeconds
	}
	if result.Format != "" {
		fm[fmAudioFormat] = result.Format
	}
	if result.AudioId != "" {
		fm[fmAudioID] = result.AudioId
	}

	saveOK, saveMsg, newMtime := posts.SaveFile(s.cfg.ContentDir, articlePath, body, fm, expectedMtime)
	if !saveOK {
		if conflict, isConflict := saveMsg.(posts.ConflictInfo); isConflict {
			emit("tts.conflict", map[string]any{
				"message":       "文章已被修改，请保存后重试",
				"current_mtime": conflict.CurrentMTime,
				"url":           result.Url,
				"audio_id":      result.AudioId,
			})
		} else {
			emit("tts.failed", map[string]any{"message": fmt.Sprintf("写入 frontmatter 失败: %v", saveMsg)})
		}
		return
	}
	emit("tts.done", map[string]any{
		"url": result.Url, "duration_seconds": result.DurationSeconds,
		"format": result.Format, "audio_id": result.AudioId, "mtime": newMtime,
	})
}

// DELETE /api/article/tts —— 清字段 + 通知插件删托管音频
func (s *Server) handleArticleTTSDelete(w http.ResponseWriter, r *http.Request) {
	data := jsonDict(r)
	articlePath, _ := data["article_path"].(string)
	if articlePath == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": "缺少文章路径"})
		return
	}
	ok, body, fm, _ := posts.ReadFileWithFrontmatter(s.cfg.ContentDir, articlePath)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": "读取文章失败"})
		return
	}
	audioID, _ := fm[fmAudioID].(string)
	delete(fm, fmAudioID)
	delete(fm, fmAudio)
	delete(fm, fmAudioDuration)
	delete(fm, fmAudioFormat)

	saveOK, saveMsg, newMtime := posts.SaveFile(s.cfg.ContentDir, articlePath, body, fm, nil)
	if !saveOK {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false, "message": fmt.Sprintf("清除 audio 字段失败: %v", saveMsg)})
		return
	}

	// 通知插件删除托管音频（失败只记日志）
	if audioID != "" {
		if _, stubp, ok := s.resolveTTSPlugin(); ok {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if _, err := (*stubp).Delete(ctx, &pb.TTSDeleteRequest{AudioId: audioID}); err != nil {
				fmt.Printf("插件删除托管音频失败（已清 frontmatter）: %v\n", err)
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true, "message": "已删除语音", "mtime": newMtime,
	})
}

func trimSpaces(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r != ' ' && r != '\t' && r != '\n' && r != '\r' {
			out = append(out, r)
		}
	}
	return string(out)
}
