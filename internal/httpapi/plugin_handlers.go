package httpapi

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"time"

	"github.com/svtter/hugo-admin/internal/plugin"
	pb "github.com/svtter/hugo-admin/proto"
)

// 对齐 routes/plugin_routes.py 全部端点。
// TTS 进度经 SSE broker 推送（事件名 tts.progress，payload 带 scope）。

func (s *Server) plugins() *plugin.Manager { return s.pluginMgr }

// GET /api/plugins
func (s *Server) handlePluginList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true, "plugins": s.plugins().ListPlugins(),
	})
}

// GET /api/plugins/{name}/config-schema
func (s *Server) handlePluginConfigSchema(w http.ResponseWriter, r *http.Request) {
	schema := s.plugins().GetConfigSchema(r.PathValue("name"))
	if len(schema) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"success": false, "message": "Plugin not found",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "schema": schema})
}

// GET /api/plugins/{name}/config
func (s *Server) handlePluginConfigGet(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if s.plugins().Stub(name) == nil && s.plugins().GetConfigSchema(name) == nil && !s.pluginExists(name) {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"success": false, "message": "Plugin not found",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "config": s.plugins().GetConfig(name)})
}

// PUT /api/plugins/{name}/config —— 保存成功但推送失败返回 207（对齐 Python）
func (s *Server) handlePluginConfigSet(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !s.pluginExists(name) {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"success": false, "message": "Plugin not found",
		})
		return
	}
	config := jsonDict(r)
	if s.plugins().SetConfig(name, config) {
		writeJSON(w, http.StatusOK, map[string]any{
			"success": true, "message": "Configuration saved",
		})
		return
	}
	writeJSON(w, http.StatusMultiStatus, map[string]any{
		"success": false, "message": "Configuration saved but failed to push to plugin",
	})
}

// POST /api/plugins/{name}/enable
func (s *Server) handlePluginEnable(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if s.plugins().EnablePlugin(name) {
		writeJSON(w, http.StatusOK, map[string]any{
			"success": true, "message": fmt.Sprintf("Plugin %s enabled", name),
		})
		return
	}
	writeJSON(w, http.StatusInternalServerError, map[string]any{
		"success": false, "message": fmt.Sprintf("Failed to enable %s", name),
	})
}

// POST /api/plugins/{name}/disable
func (s *Server) handlePluginDisable(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if s.plugins().DisablePlugin(name) {
		writeJSON(w, http.StatusOK, map[string]any{
			"success": true, "message": fmt.Sprintf("Plugin %s disabled", name),
		})
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]any{
		"success": false, "message": fmt.Sprintf("Plugin %s not found", name),
	})
}

// GET /api/plugins/market
func (s *Server) handlePluginMarket(w http.ResponseWriter, r *http.Request) {
	catalog := s.plugins().FetchMarket()
	resp := map[string]any{"success": true}
	for k, v := range catalog {
		resp[k] = v
	}
	writeJSON(w, http.StatusOK, resp)
}

// POST /api/plugins/{name}/image/upload —— multipart → gRPC 客户端流式
func (s *Server) handlePluginImageUpload(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "message": "No file provided",
		})
		return
	}
	defer file.Close()
	if header.Filename == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "message": "Empty filename",
		})
		return
	}
	conn := s.plugins().Conn(name)
	if conn == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"success": false,
			"message": fmt.Sprintf("Plugin %s not running or does not support image_upload", name),
		})
		return
	}

	articlePath := r.FormValue("article_path")
	stub := pb.NewImageUploaderClient(conn)
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	stream, err := stub.Upload(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false, "message": "Upload failed: " + err.Error(),
		})
		return
	}

	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	buf := make([]byte, 64*1024)
	first := true
	for {
		n, readErr := file.Read(buf)
		if n > 0 {
			chunk := &pb.ImageUploadChunk{Data: buf[:n], IsLast: false}
			if first {
				chunk.Filename = header.Filename
				chunk.MimeType = mimeType
				chunk.ArticlePath = articlePath
				first = false
			}
			if err := stream.Send(chunk); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{
					"success": false, "message": "Upload failed: " + err.Error(),
				})
				return
			}
		}
		if readErr != nil {
			break
		}
	}
	// 对齐 Python：最终发送空终止帧 is_last=true
	if err := stream.Send(&pb.ImageUploadChunk{IsLast: true}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false, "message": "Upload failed: " + err.Error(),
		})
		return
	}
	resp, err := stream.CloseAndRecv()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false, "message": "Upload failed: " + err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success":  resp.Success,
		"url":      resp.Url,
		"image_id": resp.ImageId,
		"message":  resp.Message,
	})
}

// DELETE /api/plugins/{name}/image/{image_id}
func (s *Server) handlePluginImageDelete(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	conn := s.plugins().Conn(name)
	if conn == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"success": false, "message": fmt.Sprintf("Plugin %s not running", name),
		})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	resp, err := pb.NewImageUploaderClient(conn).Delete(ctx, &pb.ImageDeleteRequest{ImageId: r.PathValue("image_id")})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false, "message": "Delete failed: " + err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": resp.Success, "message": resp.Message})
}

// POST /api/plugins/{name}/tts/generate —— 服务端流；progress 经 SSE 推送
func (s *Server) handlePluginTTSGenerate(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	data := jsonDict(r)
	text, _ := data["text"].(string)
	if text == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "message": "缺少待合成文本",
		})
		return
	}
	conn := s.plugins().Conn(name)
	if conn == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"success": false,
			"message": fmt.Sprintf("Plugin %s not running or does not support tts_generation", name),
		})
		return
	}

	speed := 1.0
	if raw, ok := data["speed"]; ok && raw != nil {
		switch v := raw.(type) {
		case float64:
			speed = v
		case string:
			parsed, err := strconv.ParseFloat(v, 32)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{
					"success": false, "message": fmt.Sprintf("无效的参数: %v", err),
				})
				return
			}
			speed = parsed
		default:
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"success": false, "message": "无效的参数: speed",
			})
			return
		}
	}
	str := func(key string) string {
		v, _ := data[key].(string)
		return v
	}
	req := &pb.TTSRequest{
		Text: text, Voice: str("voice"), Model: str("model"),
		Speed: float32(speed), Format: str("format"), Language: str("language"),
		ArticlePath: str("article_path"),
	}

	ctx, cancel := context.WithTimeout(r.Context(), 300*time.Second)
	defer cancel()
	stream, err := pb.NewTTSGeneratorClient(conn).Generate(ctx, req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false, "message": "TTS failed: " + err.Error(),
		})
		return
	}

	eventScope := str("event_scope")
	var result *pb.TTSResult
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{
				"success": false, "message": "TTS failed: " + err.Error(),
			})
			return
		}
		switch payload := resp.Payload.(type) {
		case *pb.TTSResponse_Progress:
			p := payload.Progress
			s.broker.Broadcast("tts.progress", map[string]any{
				"scope": eventScope, "stage": p.Stage,
				"percent": p.Percent, "message": p.Message,
			})
		case *pb.TTSResponse_Result:
			result = payload.Result
		}
	}
	if result == nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"success": false, "message": "插件未返回 TTS 结果",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": result.Success, "url": result.Url,
		"duration_seconds": result.DurationSeconds, "audio_id": result.AudioId,
		"format": result.Format, "message": result.Message,
	})
}

// DELETE /api/plugins/{name}/tts/{audio_id}
func (s *Server) handlePluginTTSDelete(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	conn := s.plugins().Conn(name)
	if conn == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"success": false, "message": fmt.Sprintf("Plugin %s not running", name),
		})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	resp, err := pb.NewTTSGeneratorClient(conn).Delete(ctx, &pb.TTSDeleteRequest{AudioId: r.PathValue("audio_id")})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false, "message": "Delete failed: " + err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": resp.Success, "message": resp.Message})
}

func (s *Server) pluginExists(name string) bool {
	for _, info := range s.plugins().ListPlugins() {
		if info["name"] == name {
			return true
		}
	}
	return false
}

// uploadViaPlugin 经 ImageUploader 能力插件上传；写响应并返回 true，
// 失败返回 false 由调用方回退本地保存。
func (s *Server) uploadViaPlugin(w http.ResponseWriter, r *http.Request, name string, file io.Reader, header *multipart.FileHeader, articlePath string) bool {
	conn := s.plugins().Conn(name)
	if conn == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	stream, err := pb.NewImageUploaderClient(conn).Upload(ctx)
	if err != nil {
		return false
	}
	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	buf := make([]byte, 64*1024)
	first := true
	for {
		n, readErr := file.Read(buf)
		if n > 0 {
			chunk := &pb.ImageUploadChunk{Data: buf[:n]}
			if first {
				chunk.Filename = header.Filename
				chunk.MimeType = mimeType
				chunk.ArticlePath = articlePath
				first = false
			}
			if stream.Send(chunk) != nil {
				return false
			}
		}
		if readErr != nil {
			break
		}
	}
	// 对齐 Python：最终发送空终止帧 is_last=true
	if stream.Send(&pb.ImageUploadChunk{IsLast: true}) != nil {
		return false
	}
	resp, err := stream.CloseAndRecv()
	if err != nil || !resp.Success {
		fmt.Printf("Plugin image upload failed: %v %s\n", err, resp.GetMessage())
		return false
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true, "url": resp.Url, "message": "图片上传成功",
	})
	return true
}
