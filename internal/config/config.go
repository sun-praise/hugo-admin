// Package config 加载运行配置：.env 文件（python-dotenv 兼容格式）+
// 环境变量覆盖，字段语义与 Python 侧 config.py 保持一致。
package config

import (
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Port          string // PORT，默认 5050（同 Python app.py）
	SecretKey     string // SECRET_KEY，默认值同 config.py 开发配置
	AuthStorePath string // 凭据文件，默认 data/auth.json
	AdminUIDir    string // 前端构建产物目录，默认 admin-ui
	ContentDir    string // Hugo 内容目录，默认 <root>/content
	Version       string // 从 __version__.py 读取，读不到则回退
}

// Load 依序加载 .env 与环境变量；root 为仓库根目录（解析相对路径用）。
func Load(root string) *Config {
	if root == "" {
		root, _ = os.Getwd()
	}
	loadDotEnv(filepath.Join(root, ".env"))

	c := &Config{
		Port:          envOr("PORT", "5050"),
		SecretKey:     envOr("SECRET_KEY", "dev-secret-key-change-in-production"),
		AuthStorePath: envOr("AUTH_STORE", filepath.Join(root, "data", "auth.json")),
		AdminUIDir:    envOr("ADMIN_UI_DIR", filepath.Join(root, "admin-ui")),
		ContentDir:    envOr("CONTENT_DIR", filepath.Join(root, "content")),
		Version:       readVersion(filepath.Join(root, "__version__.py")),
	}
	return c
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// loadDotEnv 解析 KEY=VALUE 行；# 开头为注释，值可加引号。
// 已存在的环境变量不覆盖（与 python-dotenv 的 override=False 一致）。
func loadDotEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}
		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, value)
		}
	}
}

// readVersion 解析 __version__.py 里的 __version__ = "x.y.z"，
// 迁移期保持 /api/version 与 Python 侧同源；Python 文件移除后回退。
func readVersion(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "0.0.0-go"
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(line, "__version__ ="); ok {
			v = strings.TrimSpace(v)
			v = strings.Trim(v, `"'`)
			if v != "" {
				return v
			}
		}
	}
	return "0.0.0-go"
}
