package main

import (
	"flag"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"time"

	"github.com/shusfun/mistral-gui/internal/app"
)

func main() {
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
	logFile := a.EnableLogging()
	defer logFile.Close()
	defer a.Close()
	if *language != "" {
		_ = a.SetLanguage(*language)
	}
	if !*noOpen {
		go func() { time.Sleep(500 * time.Millisecond); a.OpenBrowser() }()
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	go func() { <-signals; a.Close(); os.Exit(0) }()
	if err := a.ServeListener(listener); err != nil {
		slog.Error("mistral-gui 已退出", "error", err)
		os.Exit(1)
	}
}
