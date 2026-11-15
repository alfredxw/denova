package api

import (
	"compress/gzip"
	"context"
	"log/slog"
	"net"

	hertzserver "github.com/cloudwego/hertz/pkg/app/server"
	hertzconfig "github.com/cloudwego/hertz/pkg/common/config"
	hertzgzip "github.com/hertz-contrib/gzip"

	"denova/config"
	"denova/internal/api/sse"
	"denova/internal/app"
	"denova/internal/update"
)

// Release uploads need multipart overhead above the archive limit. Hertz spills
// large multipart files to disk; individual handlers enforce their content limits.
const maxRequestBodyBytes = int(update.MaxLocalArchiveBytes) + (1 << 20)

// Server 包含 Hertz 引擎和应用运行时。
type Server struct {
	engine *hertzserver.Hertz
	app    *app.App
	port   string
	host   string
	access *remoteAccessGate
}

// NewServer 构造 HTTP 服务。
func NewServer(application *app.App, port string) *Server {
	return newServer(application, port, nil)
}

// NewServerWithListener constructs an HTTP server using an already reserved
// listener. Callers retain responsibility for choosing the listener address.
func NewServerWithListener(application *app.App, port string, listener net.Listener) *Server {
	return newServer(application, port, listener)
}

func newServer(application *app.App, port string, listener net.Listener) *Server {
	configureHertzLogging()
	remoteAccess := application.RemoteAccessConfig()
	host := config.HTTPListenHost(remoteAccess.AllowLANAccess)
	s := &Server{
		app:  application,
		port: port,
		host: host,
	}

	options := []hertzconfig.Option{
		hertzserver.WithHostPorts(host + ":" + port),
		hertzserver.WithMaxRequestBodySize(maxRequestBodyBytes),
	}
	if listener != nil {
		options = append(options, hertzserver.WithListener(listener))
	}
	h := hertzserver.Default(options...)
	streamShutdown, closeStreams := context.WithCancel(context.Background())
	h.OnShutdown = append(h.OnShutdown, func(context.Context) {
		slog.Info("http_server_shutdown_started")
		closeStreams()
	})
	h.Use(sse.ShutdownMiddleware(streamShutdown))
	h.Use(requestObservabilityMiddleware)
	h.Use(corsMiddleware)
	accessGate := newRemoteAccessGate(application.RemoteAccessConfig, port)
	s.access = accessGate
	h.Use(accessGate.middleware)
	h.Use(analyticsMiddleware(application))
	accessGate.registerRoutes(h)
	// The gzip middleware buffers body streams before compressing them. Exclude
	// every SSE route at the server boundary so browsers can consume events as
	// they arrive even when their automatic Accept-Encoding header includes gzip.
	// Fast compression keeps large settings/history responses off the CPU hot
	// path while preserving compressed transfers. SSE stays unbuffered.
	h.Use(hertzgzip.Gzip(gzip.BestSpeed, hertzgzip.WithExcludedPathRegexes([]string{
		`^/api/.*/stream(?:\?.*)?$`,
		`^/api/(?:chat|interactive/chat|projects/[^/]+/(?:agent-chat/chat|events)|workspace/events)(?:\?.*)?$`,
	})))
	// Error envelopes must be correlated before gzip turns the body into a
	// compressed stream, so this stays inside the gzip middleware.
	h.Use(errorEnvelopeMiddleware)
	s.registerRoutes(h)
	s.engine = h
	return s
}

// Run 启动 HTTP 服务。
func (s *Server) Run() {
	slog.InfoContext(context.Background(), "http_server_started", "host", s.host, "port", s.port)
	s.engine.Spin()
	slog.InfoContext(context.Background(), "http_server_stopped")
}

// EnableDesktopAccess grants the owned desktop transport access. Call once,
// before serving; the per-launch secret travels only over the parent pipe.
func (s *Server) EnableDesktopAccess(token, origin string) {
	s.access.desktopToken, s.access.desktopOrigin = token, origin
}

// RunUntil lets the desktop owner request the same graceful HTTP shutdown as
// an ordinary server signal, including draining live event subscriptions.
func (s *Server) RunUntil(ctx context.Context) {
	s.engine.SetCustomSignalWaiter(func(failures chan error) error {
		select {
		case <-ctx.Done():
			return nil
		case err := <-failures:
			return err
		}
	})
	s.Run()
}
