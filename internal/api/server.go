package api

import (
	"context"
	"log/slog"
	"net"

	hertzserver "github.com/cloudwego/hertz/pkg/app/server"
	hertzconfig "github.com/cloudwego/hertz/pkg/common/config"
	"github.com/hertz-contrib/gzip"

	"denova/config"
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
	h.Use(requestObservabilityMiddleware)
	h.Use(corsMiddleware)
	accessGate := newRemoteAccessGate(application.RemoteAccessConfig, port)
	h.Use(accessGate.middleware)
	accessGate.registerRoutes(h)
	// The gzip middleware buffers body streams before compressing them. Exclude
	// every SSE route at the server boundary so browsers can consume events as
	// they arrive even when their automatic Accept-Encoding header includes gzip.
	h.Use(gzip.Gzip(gzip.DefaultCompression, gzip.WithExcludedPathRegexes([]string{
		`^/api/.*/stream(?:\?.*)?$`,
		`^/api/(?:chat|interactive/chat|projects/[^/]+/agent-chat/chat|workspace/events)(?:\?.*)?$`,
	})))
	s.registerRoutes(h)
	s.engine = h
	return s
}

// Run 启动 HTTP 服务。
func (s *Server) Run() {
	slog.InfoContext(context.Background(), "http_server_started", "host", s.host, "port", s.port)
	s.engine.Spin()
}
