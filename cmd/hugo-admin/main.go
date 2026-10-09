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

	"github.com/svtter/hugo-admin/internal/auth"
	"github.com/svtter/hugo-admin/internal/config"
	"github.com/svtter/hugo-admin/internal/db"
	"github.com/svtter/hugo-admin/internal/git"
	"github.com/svtter/hugo-admin/internal/httpapi"
	"github.com/svtter/hugo-admin/internal/hugo"
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
	srv := httpapi.New(cfg, store, broker, gitSvc, hugoMgr, database)
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
