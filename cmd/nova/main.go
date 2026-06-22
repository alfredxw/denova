package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"

	"nova/config"
	"nova/internal/api"
	"nova/internal/app"
	"nova/internal/observability"
)

func main() {
	var (
		workspace string
		dev       bool
		noOpen    bool
	)
	cfg := config.Load()
	port := defaultPort(cfg)
	frontendPort := defaultFrontendPort(cfg)
	flag.StringVar(&workspace, "workspace", "", "作品工作目录 (默认恢复上次打开的书籍)")
	flag.StringVar(&port, "port", port, "HTTP 服务端口")
	flag.StringVar(&frontendPort, "frontend-port", frontendPort, "前端开发服务端口")
	flag.BoolVar(&dev, "dev", false, "开发模式：同时启动 Vite 前端 dev server")
	flag.BoolVar(&noOpen, "no-open", false, "启动服务后不自动打开浏览器")
	flag.Parse()

	logPath, closeLog := setupLogging("./log")
	defer closeLog()
	observability.ConfigureStructuredLogging()
	log.Printf("[startup] 日志输出已启用 dir=./log current_file=%s", logPath)
	port = selectStartupPort(port, shouldAutoPickPort())
	frontendPort = selectFrontendPort(frontendPort)

	if workspace != "" {
		cfg.Workspace = workspace
		cfg.ResumeLastWorkspace = false
	} else if os.Getenv("NOVA_WORKSPACE") != "" {
		cfg.Workspace = os.Getenv("NOVA_WORKSPACE")
		cfg.ResumeLastWorkspace = false
	}

	cfg.SkillsDir = resolveSkillsDir(cfg.SkillsDir)

	ctx := context.Background()

	// 初始化应用运行时
	application, err := app.New(ctx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化应用失败: %v\n", err)
		os.Exit(1)
	}

	// 启动 HTTP 服务
	srv := api.NewServer(application, port)
	listenHost := config.HTTPListenHost(cfg.AllowLANAccess)

	// 打印启动信息
	url := fmt.Sprintf("http://localhost:%s", port)
	frontendURL := fmt.Sprintf("http://localhost:%s", frontendPort)
	fmt.Printf("\n  Nova AI 小说创作工具\n")
	fmt.Printf("  ─────────────────────\n")
	fmt.Printf("  后端服务: %s\n", url)
	if dev {
		fmt.Printf("  前端入口: %s\n", frontendURL)
	}
	if cfg.AllowLANAccess {
		if dev {
			fmt.Printf("  局域网入口: http://%s:%s\n", config.LANAddress(), frontendPort)
		} else {
			fmt.Printf("  局域网后端: http://%s:%s\n", config.LANAddress(), port)
		}
	}
	fmt.Printf("  作品目录: %s\n", application.Workspace())
	fmt.Printf("  按 Ctrl+C 停止服务\n\n")

	// 开发模式：同时启动 Vite dev server
	if dev {
		go startViteDev(frontendPort, listenHost)
	}
	if !noOpen {
		if dev {
			go openBrowser(frontendURL)
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
func startViteDev(port, host string) {
	// 查找 web 目录
	webDir := "./web"
	if _, err := os.Stat(webDir); os.IsNotExist(err) {
		// 尝试可执行文件同级
		webDir = bundledDir("web")
		if _, err := os.Stat(webDir); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "警告: 未找到 web/ 目录，跳过前端 dev server\n")
			return
		}
	}

	cmd := exec.Command("pnpm", "dev", "--host", host, "--port", port)
	cmd.Dir = webDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Vite dev server 退出: %v\n", err)
	}
}

func defaultPort(cfg *config.Config) string {
	if cfg != nil && cfg.BackendPort > 0 {
		return strconv.Itoa(cfg.BackendPort)
	}
	return "8080"
}

func defaultFrontendPort(cfg *config.Config) string {
	if cfg != nil && cfg.FrontendPort > 0 {
		return strconv.Itoa(cfg.FrontendPort)
	}
	return "5173"
}

func shouldAutoPickPort() bool {
	if os.Getenv("NOVA_BACKEND_PORT") != "" {
		return false
	}
	explicit := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "port" {
			explicit = true
		}
	})
	return !explicit
}

func selectStartupPort(preferred string, autoPick bool) string {
	if portAvailable(preferred) {
		return preferred
	}
	if !autoPick {
		log.Printf("[startup] HTTP 端口不可用 port=%s auto_pick=false", preferred)
		return preferred
	}

	next, err := findAvailablePort(preferred, 20)
	if err != nil {
		log.Printf("[startup] HTTP 端口不可用且自动选择失败 port=%s err=%v", preferred, err)
		return preferred
	}

	fmt.Fprintf(os.Stderr, "提示: 端口 %s 已被占用，已自动改用 %s\n", preferred, next)
	log.Printf("[startup] HTTP 端口 %s 已被占用，自动改用 %s", preferred, next)
	return next
}

// selectFrontendPort 为前端 Vite dev server 自动选择一个可用端口。
// 与 HTTP 后端端口不同，前端端口总是尝试自动选择（因为 Vite 不负责端口协商）。
func selectFrontendPort(preferred string) string {
	if portAvailable(preferred) {
		return preferred
	}

	next, err := findAvailablePort(preferred, 20)
	if err != nil {
		fmt.Fprintf(os.Stderr, "警告: 前端端口 %s 不可用且自动选择失败: %v\n", preferred, err)
		log.Printf("[startup] 前端端口 %s 不可用且自动选择失败 err=%v", preferred, err)
		return preferred
	}

	fmt.Fprintf(os.Stderr, "提示: 前端端口 %s 已被占用，已自动改用 %s\n", preferred, next)
	log.Printf("[startup] 前端端口 %s 已被占用，自动改用 %s", preferred, next)
	return next
}

func findAvailablePort(preferred string, attempts int) (string, error) {
	start, err := strconv.Atoi(preferred)
	if err != nil || start <= 0 || start > 65535 {
		return "", fmt.Errorf("端口号无效: %s", preferred)
	}
	for port := start + 1; port <= 65535 && port <= start+attempts; port++ {
		candidate := strconv.Itoa(port)
		if portAvailable(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("未找到可用端口: %d-%d", start+1, start+attempts)
}

func portAvailable(port string) bool {
	ln, err := net.Listen("tcp", "0.0.0.0:"+port)
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

func bundledDir(name string) string {
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), name)
	}
	return ""
}

func bundledParentDir(name string) string {
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), "..", "..", name)
	}
	return ""
}

func resolveSkillsDir(configured string) string {
	if dir := existingDir(configured); dir != "" {
		return dir
	}
	if configured != "" && os.Getenv("NOVA_SKILLS_DIR") != "" {
		return configured
	}
	candidates := []string{
		"./skills",
		bundledDir("skills"),
		bundledParentDir("skills"),
	}
	for _, c := range candidates {
		if dir := existingDir(c); dir != "" {
			return dir
		}
	}
	return configured
}

func existingDir(path string) string {
	if path == "" {
		return ""
	}
	clean := filepath.Clean(path)
	if fi, err := os.Stat(clean); err == nil && fi.IsDir() {
		if abs, err := filepath.Abs(clean); err == nil {
			return abs
		}
		return clean
	}
	return ""
}
