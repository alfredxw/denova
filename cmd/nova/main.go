package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"

	"nova/config"
	"nova/internal/api"
	"nova/internal/app"
)

func main() {
	var (
		workspace string
		port      string
		dev       bool
		noOpen    bool
	)
	flag.StringVar(&workspace, "workspace", "", "作品工作目录 (默认恢复最近打开的书籍)")
	flag.StringVar(&port, "port", "8080", "HTTP 服务端口")
	flag.BoolVar(&dev, "dev", false, "开发模式：同时启动 Vite 前端 dev server")
	flag.BoolVar(&noOpen, "no-open", false, "启动服务后不自动打开浏览器")
	flag.Parse()

	logPath, closeLog := setupLogging("./log")
	defer closeLog()
	log.Printf("[startup] 日志输出已启用 dir=./log current_file=%s", logPath)

	cfg := config.Load()
	if workspace != "" {
		cfg.Workspace = workspace
		cfg.ResumeLastWorkspace = false
	} else if os.Getenv("NOVA_WORKSPACE") != "" {
		cfg.Workspace = os.Getenv("NOVA_WORKSPACE")
		cfg.ResumeLastWorkspace = false
	}

	// 自动检测 skills 目录
	if cfg.SkillsDir == "" {
		candidates := []string{
			"./skills",
			os.Args[0] + "/../skills",
		}
		for _, c := range candidates {
			if fi, err := os.Stat(c); err == nil && fi.IsDir() {
				cfg.SkillsDir = c
				break
			}
		}
	}

	ctx := context.Background()

	// 初始化应用运行时（workspace、session、agent runner）。
	application, err := app.New(ctx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化应用失败: %v\n", err)
		os.Exit(1)
	}

	// 启动 HTTP 服务
	srv := api.NewServer(application, port)

	// 打印启动信息
	url := fmt.Sprintf("http://localhost:%s", port)
	fmt.Printf("\n  Nova AI 小说创作工具\n")
	fmt.Printf("  ─────────────────────\n")
	fmt.Printf("  服务地址: %s\n", url)
	fmt.Printf("  作品目录: %s\n", application.Workspace())
	if dev {
		fmt.Printf("  前端开发: http://localhost:5173\n")
	}
	fmt.Printf("  按 Ctrl+C 停止服务\n\n")

	// 开发模式：同时启动 Vite dev server
	if dev {
		go startViteDev()
	}
	if !noOpen {
		if dev {
			go openBrowser("http://localhost:5173")
		} else {
			go openBrowser(url)
		}
	}

	srv.Run()
}

// openBrowser 打开默认浏览器
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	}
	if cmd != nil {
		_ = cmd.Start()
	}
}

// startViteDev 启动 Vite 前端开发服务器
func startViteDev() {
	// 查找 web 目录
	webDir := "./web"
	if _, err := os.Stat(webDir); os.IsNotExist(err) {
		// 尝试可执行文件同级
		webDir = os.Args[0] + "/../web"
		if _, err := os.Stat(webDir); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "警告: 未找到 web/ 目录，跳过前端 dev server\n")
			return
		}
	}

	cmd := exec.Command("pnpm", "dev", "--host", "0.0.0.0")
	cmd.Dir = webDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Vite dev server 退出: %v\n", err)
	}
}
