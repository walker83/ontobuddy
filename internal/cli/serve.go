package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/walker/myonto/internal/web"
)

// ServeCmd 启动 Web UI 服务器。
type ServeCmd struct {
	Host string `help:"监听地址" default:"localhost"`
	Port int    `help:"端口号" default:"7399"`
	Open bool   `short:"O" help:"自动打开浏览器"`
}

func (c *ServeCmd) Run() error {
	s, dir, err := openStore()
	if err != nil {
		return err
	}

	cfg := web.Config{
		Host: c.Host,
		Port: c.Port,
		Dir:  dir,
	}

	srv := web.NewServer(s, cfg)
	addr, err := srv.Start()
	if err != nil {
		return fmt.Errorf("start web server: %w", err)
	}

	fmt.Fprintf(os.Stderr, "myonto web UI: %s\n", addr)
	fmt.Fprintf(os.Stderr, "Press Ctrl+C to stop\n")

	if c.Open {
		openBrowser(addr)
	}

	// 等待中断信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	fmt.Fprintf(os.Stderr, "\nShutting down...\n")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Stop(shutdownCtx)
}

func openBrowser(url string) {
	var cmd string
	var args []string
	switch {
	case isMacOS():
		cmd = "open"
		args = []string{url}
	case isLinux():
		cmd = "xdg-open"
		args = []string{url}
	default:
		return
	}
	startProcess(cmd, args)
}

func isMacOS() bool {
	_, err := os.Stat("/System/Library")
	return err == nil
}

func isLinux() bool {
	_, err := os.Stat("/proc")
	return err == nil
}

func startProcess(name string, args []string) {
	proc, err := os.StartProcess(name, append([]string{name}, args...), &os.ProcAttr{
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
	})
	if err == nil {
		proc.Release()
	}
}
