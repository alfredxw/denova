package main

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"denova/config"
	agentrun "denova/internal/agents/run"
	"denova/internal/api"
	"denova/internal/app"
	"denova/internal/buildinfo"
	"denova/internal/observability"
)

// The private pipe belongs to the Electron parent. Its closure cancels the
// backend even on Windows, where Unix process signals are unavailable.
type desktopCommand struct {
	ID    int    `json:"id"`
	Type  string `json:"type"`
	Token string `json:"token,omitempty"`
}

func runDesktop() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	_, output, closeLog := setupLogging("./log")
	defer closeLog()
	observability.ConfigureStructuredLogging(output)

	commands := make(chan desktopCommand)
	runBackground("desktop-control", func() {
		defer cancel()
		defer close(commands)
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			var command desktopCommand
			if err := json.Unmarshal(scanner.Bytes(), &command); err != nil {
				return
			}
			select {
			case commands <- command:
			case <-ctx.Done():
				return
			}
		}
	})
	var start desktopCommand
	select {
	case start = <-commands:
	case <-ctx.Done():
		return ctx.Err()
	}
	token, err := hex.DecodeString(start.Token)
	if start.Type != "start" || err != nil || len(token) != 32 {
		return errors.New("invalid desktop owner handshake")
	}
	cfg := config.Load()
	flags := flag.NewFlagSet("desktop-backend", flag.ContinueOnError)
	flags.BoolVar(&cfg.DevMode, "dev-mode", false, "enable development diagnostics")
	if err := flags.Parse(os.Args[1:]); err != nil {
		return err
	}
	cfg.SkillsDir = resolveSkillsDir(cfg.SkillsDir)
	release, err := acquireDataDirectory(cfg.DataDir())
	if err != nil {
		return err
	}
	defer release()
	agentrun.SetTraceRuntimeConfig(cfg.TraceCaptureLevel, cfg.TraceExporter, cfg.TraceRetentionRuns)
	agentrun.SetTraceContentCaptureEnabled(cfg.Labs.DeveloperMode)
	listener, port, err := reserveBackendListener(config.HTTPListenHost(cfg.AllowLANAccess), "0", false)
	if err != nil {
		return err
	}
	defer listener.Close()
	cfg.RuntimeWebPort, _ = strconv.Atoi(port)
	application, err := app.New(ctx, cfg)
	if err != nil {
		return err
	}
	defer application.Close()
	application.StartAnalytics(buildinfo.Version)
	server := api.NewServerWithListener(application, port, listener)
	server.EnableDesktopAccess(start.Token)
	done := make(chan struct{})
	runBackground("desktop-server", func() {
		defer cancel()
		defer close(done)
		server.RunUntil(ctx)
	})
	// Drain HTTP before App.Close flushes journals and releases the data lock.
	defer func() { cancel(); <-done }()
	baseURL := "http://127.0.0.1:" + port
	if err := waitDesktopReady(ctx, baseURL); err != nil {
		return err
	}
	encoder := json.NewEncoder(os.Stdout)
	if err := encoder.Encode(map[string]any{"type": "ready", "url": baseURL, "version": buildinfo.Version, "locale": cfg.Language}); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case command, ok := <-commands:
			if !ok {
				return nil
			}
			var result any
			var err error
			stop := false
			switch command.Type {
			case "inspect":
				result, err = application.DesktopActivity(ctx)
			case "shutdown-if-idle":
				stop, err = application.FenceDesktopUpdate(ctx)
				result = map[string]bool{"stopping": stop}
			case "shutdown":
				stop = true
				result = map[string]bool{"stopping": true}
			default:
				err = fmt.Errorf("unsupported desktop command %q", command.Type)
			}
			response := map[string]any{"type": "reply", "id": command.ID, "result": result}
			if err != nil {
				response["error"] = err.Error()
			}
			if err := encoder.Encode(response); err != nil {
				return err
			}
			if stop {
				return nil
			}
		}
	}
}

func waitDesktopReady(ctx context.Context, baseURL string) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	client := &http.Client{Timeout: time.Second}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/auth/status", nil)
		if err != nil {
			return err
		}
		response, err := client.Do(request)
		if err == nil {
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("desktop backend readiness: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}
