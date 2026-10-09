// hugo-admin 的 Go 版入口（迁移骨架阶段）。
// 运行：SECRET_KEY/AUTH_STORE 等环境变量与 Python 侧同义。
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/svtter/hugo-admin/internal/ai"
	"github.com/svtter/hugo-admin/internal/auth"
	"github.com/svtter/hugo-admin/internal/chathistory"
	"github.com/svtter/hugo-admin/internal/config"
	"github.com/svtter/hugo-admin/internal/db"
	"github.com/svtter/hugo-admin/internal/git"
	"github.com/svtter/hugo-admin/internal/httpapi"
	"github.com/svtter/hugo-admin/internal/hugo"
	"github.com/svtter/hugo-admin/internal/plugin"
	"github.com/svtter/hugo-admin/internal/realtime"
)

func main() {
	root, err := os.Getwd()
	if err != nil {
		log.Fatalf("无法确定工作目录: %v", err)
	}
	if v := os.Getenv("HUGO_ADMIN_ROOT"); v != "" {
		root = v
	}

	cfg := config.Load(root)
	if !filepath.IsAbs(cfg.AuthStorePath) {
		cfg.AuthStorePath = filepath.Join(root, cfg.AuthStorePath)
	}

	store, err := auth.OpenStore(cfg.AuthStorePath)
	if err != nil {
		log.Fatalf("凭据存储初始化失败: %v", err)
	}

	broker := realtime.NewBroker()
	hugoMgr := hugo.NewManager(cfg.HugoRoot, os.Getenv("HUGO_SERVER_BASE_URL"), broker)

	// 数据库：路径对齐 app.py（CONTENT_DIR/.admin/cache.db）
	database, err := db.Open(filepath.Join(cfg.ContentDir, ".admin", "cache.db"))
	if err != nil {
		log.Fatalf("数据库初始化失败: %v", err)
	}
	defer database.Close()
	gitSvc, err := git.New(cfg.HugoRoot, database) // 推送历史落库
	if err != nil {
		log.Fatalf("git 服务初始化失败: %v", err)
	}

	// AI：配置语义对齐 config.py（AI_API_KEY/AI_BASE_URL/AI_MODEL）
	aiSvc := ai.New(os.Getenv("AI_API_KEY"), os.Getenv("AI_BASE_URL"), os.Getenv("AI_MODEL"),
		ai.Deps{ContentDir: cfg.ContentDir, Git: gitSvc, Hugo: hugoMgr})
	if !aiSvc.Enabled() {
		log.Print("AI service disabled: AI_API_KEY not configured")
	}

	// 插件系统：~/.hugo-admin（与 Python 共享插件目录/配置/密钥）
	pluginMgr := plugin.NewManager(plugin.DefaultBaseDir())
	pluginMgr.StartAll()
	defer pluginMgr.StopAll()

	srv := httpapi.New(cfg, store, broker, httpapi.Options{
		Git: gitSvc, Hugo: hugoMgr, Database: database,
		AI: aiSvc, Chat: chathistory.New(database),
		Plugins: pluginMgr,
	})
	httpSrv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           srv,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("hugo-admin (go) %s listening on :%s", cfg.Version, cfg.Port)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}
