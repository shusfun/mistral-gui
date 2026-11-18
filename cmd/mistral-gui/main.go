package main

import (
	"flag"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/shusfun/mistral-gui/internal/app"
	"github.com/shusfun/mistral-gui/internal/desktop"
	"github.com/shusfun/mistral-gui/internal/update"
	"github.com/wailsapp/wails/v3/pkg/updater"
)

func main() {
	updater.HandleHelperMode()
	if exe, err := os.Executable(); err == nil {
		_ = update.RecoverInterruptedTarget(exe)
	}
	if len(os.Args) == 3 && os.Args[1] == "--update-guard" {
		if e := desktop.RunGuard(os.Args[2]); e != nil {
			slog.Error("update.guard", "error", e)
		}
		return
	}
	hidden, restoreErr := desktop.RestoreLaunch()
	if restoreErr != nil {
		slog.Error("update.restore", "error", restoreErr)
		os.Exit(1)
	}
	addr := flag.String("addr", "127.0.0.1:19820", "管理器监听地址")
	dataDir := flag.String("data-dir", "", "数据目录")
	noOpen := flag.Bool("no-open", false, "不打开管理页面")
	language := flag.String("language", "", "zh-CN / en-US")
	quit := flag.Bool("quit", false, "Quit the running manager")
	flag.Parse()
	if *quit {
		_ = app.ConnectExisting(*addr, *dataDir, true)
		return
	}

	// Acquire data-directory exclusive lock before touching state.json or listener (M-S10).
	dataLock, lockErr := app.AcquireDataDirLock(*dataDir)
	if lockErr != nil {
		if !*noOpen && app.ConnectExisting(*addr, *dataDir, false) == nil {
			return
		}
		slog.Error("data_dir_locked", "error", lockErr)
		os.Exit(1)
	}
	defer dataLock.Release()

	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		if !*noOpen && app.ConnectExisting(*addr, *dataDir, false) == nil {
			return
		}
		slog.Error("listen_failed", "error", err)
		os.Exit(1)
	}
	defer listener.Close()

	a, err := app.New(*addr, *dataDir)
	if err != nil {
		slog.Error("初始化 mistral-gui 失败", "error", err)
		os.Exit(1)
	}
	a.AttachLock(dataLock)
	logFile := a.EnableLogging()
	defer logFile.Close()
	defer a.Close()
	if *language != "" {
		_ = a.SetLanguage(*language)
	}
	// Handle both SIGINT and SIGTERM so engine child processes stop cleanly on logout/shutdown (M-S09).
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	go func() { <-signals; a.Close() }()
	if !*noOpen {
		if err = desktop.EnsureWebView(a.Language()); err != nil {
			slog.Error("webview2.failed", "error", err)
			os.Exit(1)
		}
		go func() {
			if e := a.ServeListener(listener); e != nil {
				slog.Error("server.failed", "error", e)
				a.Close()
			}
		}()
		if err = desktop.Run(a, hidden); err != nil {
			slog.Error("desktop.failed", "error", err)
			os.Exit(1)
		}
		return
	}
	if err := a.ServeListener(listener); err != nil {
		slog.Error("mistral-gui 已退出", "error", err)
		os.Exit(1)
	}
}
