// Package ai 用 trpc-agent-go 重写 Python 的 AIService：
// anthropic 兼容端点 + 只读工具三件套（search_posts/read_post/git_status，
// 对齐 allowed_tools 的实际暴露面）+ SSE 流式 + quick rewrite。
package ai

import (
	"context"
	"fmt"
	"strings"
	"time"

	"trpc.group/trpc-go/trpc-agent-go/agent/llmagent"
	"trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/model/anthropic"
	"trpc.group/trpc-go/trpc-agent-go/runner"
	"trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"

	"github.com/svtter/hugo-admin/internal/git"
	"github.com/svtter/hugo-admin/internal/hugo"
	"github.com/svtter/hugo-admin/internal/posts"
)

const systemPrompt = "You are a helpful read-only AI assistant for a Hugo blog. " +
	"You can search and read blog posts. You cannot modify, " +
	"create, deploy, or manage the blog in any way. " +
	"Always explain what you are doing before calling tools."

// Deps 是工具的后端依赖（与 Python AIService.Deps 对应）。
type Deps struct {
	ContentDir string
	Git        *git.Service
	Hugo       *hugo.Manager
}

type Service struct {
	enabled bool
	apiKey  string
	baseURL string
	model   string
	deps    Deps
	runner  runner.Runner
}

func New(apiKey, baseURL, modelName string, deps Deps) *Service {
	s := &Service{apiKey: apiKey, baseURL: baseURL, model: modelName, deps: deps}
	if apiKey == "" {
		return s
	}
	m := anthropic.New(modelName,
		anthropic.WithAPIKey(apiKey),
		anthropic.WithBaseURL(baseURL),
	)
	ag := llmagent.New("hugo-assistant",
		llmagent.WithModel(m),
		llmagent.WithTools(readOnlyTools(deps)),
		llmagent.WithInstruction(systemPrompt),
		llmagent.WithGenerationConfig(model.GenerationConfig{Stream: true}),
	)
	s.runner = runner.NewRunner("hugo-admin", ag)
	s.enabled = true
	return s
}

func (s *Service) Enabled() bool { return s.enabled }

// ModelName 返回配置的模型名（inline-edit 响应里回显）。
func (s *Service) ModelName() string { return s.model }

// readOnlyTools 对齐 allowed_tools 暴露面：仅三个只读工具。
func readOnlyTools(deps Deps) []tool.Tool {
	type searchIn struct {
		Query string `json:"query" jsonschema:"description=搜索关键词"`
	}
	searchPosts := function.NewFunctionTool(
		func(ctx context.Context, in searchIn) (string, error) {
			result := posts.GetPosts(deps.ContentDir, in.Query, "", "", 1, 10)
			if len(result.Posts) == 0 {
				return fmt.Sprintf("未找到匹配 '%s' 的文章", in.Query), nil
			}
			var lines []string
			lines = append(lines, fmt.Sprintf("找到 %d 篇文章：\n", result.Total))
			for _, p := range result.Posts {
				lines = append(lines, fmt.Sprintf("- **%s**", p.Title))
				lines = append(lines, fmt.Sprintf("  路径: `%s`", p.Path))
				lines = append(lines, fmt.Sprintf("  日期: %s\n", p.Date))
			}
			return strings.Join(lines, "\n"), nil
		},
		function.WithName("search_posts"),
		function.WithDescription("Search for blog posts"),
	)

	type readIn struct {
		FilePath string `json:"file_path" jsonschema:"description=文章文件路径"`
	}
	readPost := function.NewFunctionTool(
		func(ctx context.Context, in readIn) (string, error) {
			ok, content, _ := posts.ReadFile(deps.ContentDir, in.FilePath)
			if ok {
				return fmt.Sprintf("文件 `%s` 内容：\n\n%s", in.FilePath, content), nil
			}
			return fmt.Sprintf("读取失败: %s", content), nil
		},
		function.WithName("read_post"),
		function.WithDescription("Read the content of a blog post"),
	)

	type emptyIn struct{}
	gitStatus := function.NewFunctionTool(
		func(ctx context.Context, in emptyIn) (string, error) {
			st := deps.Git.GetStatus()
			// Python 工具引用了不存在的 branch/clean/changes 字段（bug），
			// 此处输出真实状态
			var lines []string
			lines = append(lines, "Git 仓库状态：\n")
			lines = append(lines, fmt.Sprintf("- 有改动: %v", st["has_changes"]))
			lines = append(lines, fmt.Sprintf("- 暂存: %v", st["staged"]))
			lines = append(lines, fmt.Sprintf("- 未暂存: %v", st["unstaged"]))
			lines = append(lines, fmt.Sprintf("- 未跟踪: %v", st["untracked"]))
			return strings.Join(lines, "\n"), nil
		},
		function.WithName("git_status"),
		function.WithDescription("Check the current git status of the blog repository"),
	)

	return []tool.Tool{searchPosts, readPost, gitStatus}
}

// QuickRewrite 对齐 quick_rewrite：无工具、非流式的一次性调用，
// 超时或空结果返回错误。
func (s *Service) QuickRewrite(ctx context.Context, sysPrompt, userPrompt string, timeout time.Duration) (string, error) {
	if !s.enabled {
		return "", fmt.Errorf("AI service is not configured")
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	m := anthropic.New(s.model,
		anthropic.WithAPIKey(s.apiKey),
		anthropic.WithBaseURL(s.baseURL),
	)
	ag := llmagent.New("rewriter",
		llmagent.WithModel(m),
		llmagent.WithInstruction(sysPrompt),
		llmagent.WithGenerationConfig(model.GenerationConfig{Stream: false}),
	)
	events, err := runner.NewRunner("quick-rewrite", ag).Run(ctx, "inline-edit", time.Now().Format("150405.000000000"), model.NewUserMessage(userPrompt))
	if err != nil {
		return "", err
	}
	var parts []string
	for ev := range events {
		if ev.Object == "chat.completion" && len(ev.Choices) > 0 {
			parts = append(parts, ev.Choices[0].Message.Content)
		}
	}
	result := strings.TrimSpace(strings.Join(parts, ""))
	if result == "" {
		return "", fmt.Errorf("quick_rewrite returned empty text")
	}
	return result, nil
}
