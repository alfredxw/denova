package api

import (
	"fmt"

	hertzserver "github.com/cloudwego/hertz/pkg/app/server"

	"nova/internal/api/handlers"
	"nova/internal/app"
)

// Server 包含 Hertz 引擎和应用运行时。
type Server struct {
	engine *hertzserver.Hertz
	app    *app.App
	port   string
}

// NewServer 构造 HTTP 服务。
func NewServer(application *app.App, port string) *Server {
	s := &Server{
		app:  application,
		port: port,
	}

	h := hertzserver.Default(
		hertzserver.WithHostPorts("0.0.0.0:"+port),
		hertzserver.WithMaxRequestBodySize(int(handlers.MaxCharacterCardUploadBytes)),
	)
	h.Use(corsMiddleware)
	s.registerRoutes(h)
	s.engine = h
	return s
}

// Run 启动 HTTP 服务。
func (s *Server) Run() {
	fmt.Printf("Nova HTTP 服务启动: http://localhost:%s\n", s.port)
	s.engine.Spin()
}
