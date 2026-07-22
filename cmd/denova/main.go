package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"denova/config"
	"denova/internal/agent"
	"denova/internal/api"
	"denova/internal/app"
	"denova/internal/buildinfo"
	"denova/internal/observability"
)

func main() {
	var (
		workspace string
		dev       bool
		devMode   bool
		noOpen    bool
	)
	if hasVersionArg(os.Args[1:]) {
		fmt.Println(buildinfo.Version)
		return
	}
	cfg := config.Load()
	port := defaultPort(cfg)
	frontendPort := defaultFrontendPort(cfg)
	flag.StringVar(&workspace, "workspace", "", "作品工作目录 (默认恢复上次打开的书籍)")
	flag.StringVar(&port, "port", port, "HTTP 服务端口")
	flag.StringVar(&frontendPort, "frontend-port", frontendPort, "前端开发服务端口")
	flag.BoolVar(&dev, "dev", false, "开发模式：同时启动 Vite 前端 dev server")
	flag.BoolVar(&devMode, "dev-mode", false, "开发启动模式：由 scripts/bootstrap.sh 传入，开启开发诊断能力")
	flag.BoolVar(&noOpen, "no-open", false, "启动服务后不自动打开浏览器")
	flag.Parse()

	cfg.DevMode = dev || devMode
	agent.SetModelInputLoggingEnabled(cfg.DevMode && cfg.LLMInputLogEnabled)
	agent.SetTraceRuntimeConfig(cfg.TraceCaptureLevel, cfg.TraceExporter, cfg.TraceRetentionRuns)

	logPath, closeLog := setupLogging("./log")
	defer closeLog()
	observability.ConfigureStructuredLogging()
	log.Printf("[startup] 日志输出已启用 dir=./log current_file=%s", logPath)
	requestedPort := port
	listenHost := config.HTTPListenHost(cfg.AllowLANAccess)
	listener, port, err := reserveBackendListener(listenHost, requestedPort, !portWasExplicitlySet(os.Args[1:]))
	if err != nil {
		reportBackendPortConflict(os.Stderr, requestedPort, err)
		waitForAnyKey(os.Stdin)
		os.Exit(1)
	}
	defer func() { _ = listener.Close() }()
	if port != requestedPort {
		reportBackendPortFallback(os.Stderr, requestedPort, port)
	}
	frontendPort = selectFrontendPort(frontendPort, port)
	if runtimeWebPort, err := strconv.Atoi(port); err == nil {
		cfg.RuntimeWebPort = runtimeWebPort
	}
	if dev {
		if runtimeWebPort, err := strconv.Atoi(frontendPort); err == nil {
			cfg.RuntimeWebPort = runtimeWebPort
		}
	}

	if workspace != "" {
		cfg.Workspace = workspace
		cfg.ResumeLastWorkspace = false
	} else if workspaceEnv := envCompat("DENOVA_WORKSPACE", "NOVA_WORKSPACE"); workspaceEnv != "" {
		cfg.Workspace = workspaceEnv
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
	defer application.Close()

	// 启动 HTTP 服务
	srv := api.NewServerWithListener(application, port, listener)

	// 打印启动信息
	url := fmt.Sprintf("http://localhost:%s", port)
	frontendURL := fmt.Sprintf("http://localhost:%s", frontendPort)
	fmt.Printf("\n  Denova AI 小说创作工具\n")
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
		go startViteDev(frontendPort, listenHost, port)
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

func hasVersionArg(args []string) bool {
	for _, arg := range args {
		if arg == "--version" || arg == "-version" {
			return true
		}
	}
	return false
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
func startViteDev(port, host, backendPort string) {
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
	cmd.Env = viteDevEnv(os.Environ(), port, backendPort)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Vite dev server 退出: %v\n", err)
	}
}

func viteDevEnv(base []string, frontendPort, backendPort string) []string {
	env := setEnvValue(base, "DENOVA_BACKEND_PORT", backendPort)
	env = setEnvValue(env, "DENOVA_FRONTEND_PORT", frontendPort)
	return env
}

func setEnvValue(base []string, key, value string) []string {
	prefix := key + "="
	env := make([]string, 0, len(base)+1)
	for _, item := range base {
		if strings.HasPrefix(item, prefix) {
			continue
		}
		env = append(env, item)
	}
	return append(env, prefix+value)
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

// reserveBackendListener atomically claims the selected HTTP port. The listener
// is passed to the HTTP server later so another process cannot take the port
// between availability detection and server startup.
func reserveBackendListener(host, preferred string, autoPick bool) (net.Listener, string, error) {
	listener, err := listenOnPort(host, preferred)
	if err == nil {
		return listener, preferred, nil
	}
	if !autoPick {
		return nil, preferred, err
	}

	start, parseErr := strconv.Atoi(preferred)
	if parseErr != nil || start < 1 || start > 65535 {
		return nil, preferred, fmt.Errorf("invalid port %q", preferred)
	}
	for candidate := start + 1; candidate <= 65535 && candidate <= start+20; candidate++ {
		port := strconv.Itoa(candidate)
		listener, candidateErr := listenOnPort(host, port)
		if candidateErr == nil {
			return listener, port, nil
		}
	}
	return nil, preferred, fmt.Errorf("no available port found in %d-%d: %w", start+1, min(start+20, 65535), err)
}

func listenOnPort(host, port string) (net.Listener, error) {
	return net.Listen("tcp", net.JoinHostPort(host, port))
}

func portWasExplicitlySet(args []string) bool {
	for _, arg := range args {
		if arg == "--" {
			return false
		}
		if arg == "-port" || arg == "--port" || strings.HasPrefix(arg, "-port=") || strings.HasPrefix(arg, "--port=") {
			return true
		}
	}
	return false
}

func reportBackendPortFallback(output io.Writer, requestedPort, selectedPort string) {
	fmt.Fprintf(output, "提示：端口 %s 已被占用，已自动改用 %s。\n", requestedPort, selectedPort)
	fmt.Fprintf(output, "Notice: port %s is in use; switched to %s.\n", requestedPort, selectedPort)
	log.Printf("[startup] HTTP port %s is unavailable; switched to %s", requestedPort, selectedPort)
}

func reportBackendPortConflict(output io.Writer, port string, err error) {
	fmt.Fprintf(output, "错误：显式指定的端口 %s 不可用：%v\n", port, err)
	fmt.Fprintf(output, "Error: explicitly specified port %s is unavailable: %v\n", port, err)
	fmt.Fprintln(output, "请释放该端口或指定其他 --port 值。按任意键（或 Enter）退出。")
	fmt.Fprintln(output, "Release the port or choose another --port value. Press any key (or Enter) to exit.")
	log.Printf("[startup] explicitly specified HTTP port is unavailable port=%s err=%v", port, err)
}

func waitForAnyKey(input io.Reader) {
	_, _ = bufio.NewReader(input).ReadByte()
}

// selectFrontendPort 为前端 Vite dev server 自动选择一个可用端口。
// 与 HTTP 后端端口不同，前端端口总是尝试自动选择（因为 Vite 不负责端口协商）。
func selectFrontendPort(preferred string, reservedPorts ...string) string {
	if !portReserved(preferred, reservedPorts...) && portAvailable(preferred) {
		return preferred
	}

	next, err := findAvailablePort(preferred, 20, reservedPorts...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "警告: 前端端口 %s 不可用且自动选择失败: %v\n", preferred, err)
		log.Printf("[startup] 前端端口 %s 不可用且自动选择失败 err=%v", preferred, err)
		return preferred
	}

	fmt.Fprintf(os.Stderr, "提示: 前端端口 %s 已被占用，已自动改用 %s\n", preferred, next)
	log.Printf("[startup] 前端端口 %s 已被占用，自动改用 %s", preferred, next)
	return next
}

func findAvailablePort(preferred string, attempts int, reservedPorts ...string) (string, error) {
	start, err := strconv.Atoi(preferred)
	if err != nil || start <= 0 || start > 65535 {
		return "", fmt.Errorf("端口号无效: %s", preferred)
	}
	for port := start + 1; port <= 65535 && port <= start+attempts; port++ {
		candidate := strconv.Itoa(port)
		if !portReserved(candidate, reservedPorts...) && portAvailable(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("未找到可用端口: %d-%d", start+1, start+attempts)
}

func portReserved(port string, reservedPorts ...string) bool {
	value, err := strconv.Atoi(port)
	if err != nil {
		return false
	}
	for _, reserved := range reservedPorts {
		reservedValue, err := strconv.Atoi(reserved)
		if err == nil && reservedValue == value {
			return true
		}
	}
	return false
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
	if configured != "" && envCompat("DENOVA_SKILLS_DIR", "NOVA_SKILLS_DIR") != "" {
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

func envCompat(current, legacy string) string {
	if v := os.Getenv(current); v != "" {
		return v
	}
	return os.Getenv(legacy)
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
