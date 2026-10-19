// main.go workbuddy2api 入口：加载配置、构建 pool、起调度器与 HTTP 服务。
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"workbuddy2api/internal/auth"
	"workbuddy2api/internal/logstore"
	"workbuddy2api/internal/oauth"
	"workbuddy2api/internal/pool"
	"workbuddy2api/internal/scheduler"
	"workbuddy2api/internal/server"
	"workbuddy2api/internal/upstream"
)

func main() {
	cfgPath := flag.String("config", "config.json", "path to config json")
	logDir := flag.String("log-dir", os.Getenv("WB2A_LOG_DIR"), "persistent log directory (default: next to state_file)")
	importLogs := flag.String("import-logs", "", "import a docker logs --timestamps file into an empty journal, then exit")
	flag.Parse()
	if *importLogs != "" {
		if *logDir == "" {
			log.Fatal("-import-logs requires -log-dir")
		}
		journal, err := logstore.Open(*logDir, logstore.Options{})
		if err != nil {
			log.Fatal(err)
		}
		defer journal.Close()
		f, err := os.Open(*importLogs)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		n, err := journal.ImportDocker(f)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("imported %d historical log lines\n", n)
		return
	}

	cfg, err := Load(*cfgPath)
	if err != nil {
		// 配置文件不存在时给一次机会用纯默认 + env
		if os.IsNotExist(err) {
			log.Printf("config %s not found, using defaults+env", *cfgPath)
			cfg, err = Load("")
		}
		if err != nil {
			log.Fatalf("load config: %v", err)
		}
	}

	if *logDir == "" {
		*logDir = filepath.Join(filepath.Dir(cfg.StateFile), "logs")
	}
	journal, err := logstore.Open(*logDir, logstore.Options{Output: os.Stderr, Secrets: []string{cfg.APIKey}})
	if err != nil {
		log.Fatalf("open log journal: %v", err)
	}
	defer journal.Close()
	log.SetOutput(journal)

	auths, err := auth.LoadDir(cfg.AuthDir, cfg.Region)
	if err != nil {
		log.Fatalf("load auths: %v", err)
	}
	log.Printf("loaded %d %s account(s) from %s", len(auths), cfg.Region, cfg.AuthDir)

	p := pool.New(cfg.StateFile)
	defer p.Flush()    // 进程退出前强制落盘（后台 flush 每 5s 一次，退出时补一次）
	p.SyncToDir(auths) // 与 auths 目录对齐：新账号加入、已删除文件账号剔除（状态保留）

	up := upstream.New()
	up.HTTP.Timeout = time.Duration(cfg.Upstream.TimeoutSeconds) * time.Second
	up.SanitizeFingerprints = cfg.Features.SanitizeBlacklistFingerprints

	sch := scheduler.New(scheduler.Config{
		Pool:           p,
		Upstream:       up,
		CheckinHours:   cfg.Schedule.CheckinHours,
		KeepaliveHours: cfg.Schedule.KeepaliveHours,
	})

	h := server.NewHandler(server.Config{
		Pool:         p,
		Upstream:     up,
		OAuth:        oauth.NewClient(),
		Maintenance:  sch,
		AuthDir:      cfg.AuthDir,
		Region:       cfg.Region,
		APIKey:       cfg.APIKey,
		Logs:         journal,
		HardCooldown: cfg.HardCreditDur,
		SoftCooldown: cfg.SoftRateDur,
		ErrThreshold: cfg.Cooldown.ErrThresh,
		ErrCooldown:  cfg.ErrCooldownDur,
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go sch.Run(ctx)

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           h,
		ReadHeaderTimeout: 30 * time.Second,
	}
	go func() {
		<-ctx.Done()
		p.Flush() // 信号触发：先落盘再做优雅停机
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Printf("workbuddy2api listening on %s (auth_enabled=%v)", cfg.Listen, cfg.APIKey != "")
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("http: %v", err)
	}
	log.Printf("bye")
}
