package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	hertzapp "github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/hertz-contrib/websocket"

	"denova/internal/terminal"
)

// terminalUpgrader upgrades an HTTP connection to a WebSocket. Same-origin checking is handled
// by allowedTerminalOrigin, which also lets the local Vite dev server through.
var terminalUpgrader = websocket.HertzUpgrader{
	ReadBufferSize:  16 * 1024,
	WriteBufferSize: 16 * 1024,
	CheckOrigin:     allowedTerminalOrigin,
}

// terminalWriteWait bounds one frame write. It only prevents a stuck peer from leaking a
// goroutine and does not bound the session itself, which is long-lived by design.
const terminalWriteWait = 30 * time.Second

type terminalCreateRequest struct {
	OwnerTabID string   `json:"owner_tab_id"`
	Workspace  string   `json:"workspace"`
	ProfileID  string   `json:"profile_id"`
	Title      string   `json:"title"`
	Command    string   `json:"command"`
	Args       []string `json:"args"`
	Cwd        string   `json:"cwd"`
	Cols       int      `json:"cols"`
	Rows       int      `json:"rows"`
}

type terminalSessionResponse struct {
	terminal.Info
	// Token is returned so the frontend can open the attach WebSocket for this session.
	Token string `json:"token"`
}

// HandleTerminalSessions returns the terminal sessions that are currently alive.
func (h *Handlers) HandleTerminalSessions(_ context.Context, c *hertzapp.RequestContext) {
	manager := h.app.Terminals()
	if manager == nil {
		writeErrorKey(c, consts.StatusServiceUnavailable, "api.terminal.disabled")
		return
	}
	cfg := manager.Config()
	// The list also carries attach tokens: this endpoint is authenticated, and after a reload the
	// frontend uses them to re-attach to sessions still running (e.g. codex / claude code in a tab).
	infos := manager.List()
	sessions := make([]terminalSessionResponse, 0, len(infos))
	for _, info := range infos {
		session, err := manager.Get(info.ID)
		if err != nil {
			continue
		}
		sessions = append(sessions, terminalSessionResponse{Info: info, Token: session.Token()})
	}
	writeJSON(c, consts.StatusOK, map[string]any{
		"enabled":       cfg.Enabled,
		"shell":         manager.ResolveShell(),
		"default_cwd":   h.app.TerminalDefaultCwd(),
		"max_sessions":  cfg.MaxSessions,
		"sessions":      sessions,
		"scrollback_kb": cfg.ScrollbackBytes / 1024,
	})
}

// HandleTerminalSessionCreate starts a new terminal session.
func (h *Handlers) HandleTerminalSessionCreate(_ context.Context, c *hertzapp.RequestContext) {
	manager := h.app.Terminals()
	if manager == nil {
		writeErrorKey(c, consts.StatusServiceUnavailable, "api.terminal.disabled")
		return
	}
	var req terminalCreateRequest
	if err := c.BindAndValidate(&req); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidBody")
		return
	}
	ownerTabID := strings.TrimSpace(req.OwnerTabID)
	if ownerTabID == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidBody")
		return
	}
	workspace, err := h.app.ResolveTerminalWorkspace(req.Workspace)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	requestedCwd := req.Cwd
	if strings.TrimSpace(requestedCwd) == "" {
		requestedCwd = workspace
	}
	profileID := strings.TrimSpace(req.ProfileID)
	launch, err := manager.ResolveLaunchProfile(profileID, req.Command, req.Args)
	if err != nil {
		writeTerminalError(c, err)
		return
	}
	spec := terminal.Spec{
		OwnerTabID:     ownerTabID,
		ProfileID:      profileID,
		Title:          strings.TrimSpace(req.Title),
		Command:        launch.Command,
		Args:           launch.Args,
		StartupCommand: launch.StartupCommand,
		Cwd:            h.app.ResolveTerminalCwd(requestedCwd),
		Cols:           req.Cols,
		Rows:           req.Rows,
		Workspace:      workspace,
	}
	session, err := manager.Create(spec)
	if err != nil {
		writeTerminalError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, terminalSessionResponse{Info: session.Info(), Token: session.Token()})
}

// HandleTerminalSessionDelete terminates and removes a terminal session.
func (h *Handlers) HandleTerminalSessionDelete(_ context.Context, c *hertzapp.RequestContext) {
	manager := h.app.Terminals()
	if manager == nil {
		writeErrorKey(c, consts.StatusServiceUnavailable, "api.terminal.disabled")
		return
	}
	id := c.Param("id")
	if err := manager.Close(id); err != nil {
		// DELETE is intentionally idempotent: startup reconciliation and a tab-close race may both
		// release the same process, and neither should surface a false user-facing failure.
		if errors.Is(err, terminal.ErrNotFound) {
			writeJSON(c, consts.StatusOK, map[string]any{"id": id, "closed": false})
			return
		}
		writeTerminalError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"id": id, "closed": true})
}

// HandleTerminalAttach upgrades the HTTP connection to a WebSocket attached to one session.
//
// Auth: browsers cannot send an Authorization header on a WebSocket handshake, so this route is
// exempt from Basic Auth in the middleware and instead validates the token handed out when the
// session was created (see middleware.go).
func (h *Handlers) HandleTerminalAttach(_ context.Context, c *hertzapp.RequestContext) {
	manager := h.app.Terminals()
	if manager == nil {
		writeErrorKey(c, consts.StatusServiceUnavailable, "api.terminal.disabled")
		return
	}
	id := c.Param("id")
	session, err := manager.Get(id)
	if err != nil {
		writeTerminalError(c, err)
		return
	}
	token := strings.TrimSpace(c.Query("token"))
	if token == "" || token != session.Token() {
		log.Printf("[api/handlers/handler_terminal.go] terminal attach rejected id=%s reason=token_mismatch", id)
		writeErrorKey(c, consts.StatusForbidden, "api.terminal.tokenInvalid")
		return
	}
	if err := terminalUpgrader.Upgrade(c, func(conn *websocket.Conn) {
		serveTerminalSocket(session, conn)
	}); err != nil {
		log.Printf("[api/handlers/handler_terminal.go] terminal websocket upgrade failed id=%s err=%v", id, err)
	}
}

// terminalClientMessage is a control frame sent by the frontend.
type terminalClientMessage struct {
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
	Cols int    `json:"cols,omitempty"`
	Rows int    `json:"rows,omitempty"`
}

// terminalSocket is the WebSocket surface owned by the terminal relay. Keeping the boundary
// private makes the lifecycle testable without changing the wire protocol.
type terminalSocket interface {
	Close() error
	ReadMessage() (messageType int, payload []byte, err error)
	SetWriteDeadline(time.Time) error
	WriteMessage(messageType int, payload []byte) error
}

// serveTerminalSocket relays terminal data in both directions over one WebSocket.
//
// Frame contract:
//   - server -> client: binary frames carry raw pty output, text frames carry JSON control
//     messages (ready / exit / error / pong).
//   - client -> server: binary frames carry raw input, text frames carry JSON control messages
//     with type input / resize / ping.
func serveTerminalSocket(session *terminal.Session, conn terminalSocket) {
	serveTerminalSocketWithSubscriberQueue(session, conn, 0)
}

func serveTerminalSocketWithSubscriberQueue(session *terminal.Session, conn terminalSocket, subscriberQueue int) {
	// Any failed relay leg owns the whole transport. Closing through one gate keeps the reader,
	// writer and output subscription from leaving one another falsely alive.
	done := make(chan struct{})
	var closeSocketOnce sync.Once
	closeSocket := func() {
		closeSocketOnce.Do(func() {
			close(done)
			_ = conn.Close()
		})
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("[api/handlers/handler_terminal.go] terminal socket panic recovered id=%s err=%v", session.ID(), recovered)
		}
		closeSocket()
	}()

	history, output, detach := session.Attach(subscriberQueue)
	defer detach()

	writes := make(chan func() error, 64)

	// Single writer goroutine: WebSockets forbid concurrent writes, so output and control frames queue here.
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("[api/handlers/handler_terminal.go] terminal writer panic recovered id=%s err=%v", session.ID(), recovered)
			}
			closeSocket()
		}()
		for {
			select {
			case <-done:
				return
			case write := <-writes:
				if err := write(); err != nil {
					log.Printf("[api/handlers/handler_terminal.go] terminal write failed id=%s err=%v", session.ID(), err)
					return
				}
			}
		}
	}()

	enqueue := func(write func() error) bool {
		select {
		case <-done:
			return false
		case writes <- write:
			return true
		default:
			// The session retains bounded scrollback, so disconnecting a client that cannot drain
			// its own write queue is recoverable and preferable to blocking every relay goroutine.
			log.Printf("[api/handlers/handler_terminal.go] terminal write queue saturated, closing socket id=%s", session.ID())
			closeSocket()
			return false
		}
	}
	writeBinary := func(payload []byte) bool {
		return enqueue(func() error {
			_ = conn.SetWriteDeadline(time.Now().Add(terminalWriteWait))
			return conn.WriteMessage(websocket.BinaryMessage, payload)
		})
	}
	writeControl := func(message map[string]any) bool {
		payload, err := json.Marshal(message)
		if err != nil {
			log.Printf("[api/handlers/handler_terminal.go] marshal terminal control failed id=%s err=%v", session.ID(), err)
			return true
		}
		return enqueue(func() error {
			_ = conn.SetWriteDeadline(time.Now().Add(terminalWriteWait))
			return conn.WriteMessage(websocket.TextMessage, payload)
		})
	}

	if len(history) > 0 {
		writeBinary(history)
	}
	writeControl(map[string]any{"type": "ready", "session": session.Info()})

	// Output relay goroutine: pty output -> WebSocket.
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("[api/handlers/handler_terminal.go] terminal output relay panic recovered id=%s err=%v", session.ID(), recovered)
			}
		}()
		for {
			select {
			case <-done:
				return
			case chunk, ok := <-output:
				if !ok {
					// A live session only closes this subscription when the client falls behind or
					// the session is explicitly released. End the owning transport as well: otherwise
					// the read loop leaves a WebSocket that can accept input but never show output.
					select {
					case <-session.Exited():
						code, message := session.ExitStatus()
						writeControl(map[string]any{"type": "exit", "code": code, "error": message})
					default:
						log.Printf("[api/handlers/handler_terminal.go] terminal output subscription ended, closing socket id=%s", session.ID())
						closeSocket()
					}
					return
				}
				if !writeBinary(chunk) {
					return
				}
			}
		}
	}()

	// The read loop owns this goroutine; leaving it means the client disconnected.
	for {
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			log.Printf("[api/handlers/handler_terminal.go] terminal socket closed id=%s err=%v", session.ID(), err)
			return
		}
		if messageType == websocket.BinaryMessage {
			if err := session.Write(payload); err != nil {
				writeControl(map[string]any{"type": "error", "error": err.Error()})
			}
			continue
		}
		if messageType != websocket.TextMessage {
			continue
		}
		var message terminalClientMessage
		if err := json.Unmarshal(payload, &message); err != nil {
			log.Printf("[api/handlers/handler_terminal.go] invalid terminal client frame id=%s err=%v", session.ID(), err)
			continue
		}
		switch message.Type {
		case "input":
			if err := session.Write([]byte(message.Data)); err != nil {
				writeControl(map[string]any{"type": "error", "error": err.Error()})
			}
		case "resize":
			if err := session.Resize(message.Cols, message.Rows); err != nil && !errors.Is(err, terminal.ErrSessionExited) {
				log.Printf("[api/handlers/handler_terminal.go] terminal resize failed id=%s err=%v", session.ID(), err)
			}
		case "ping":
			writeControl(map[string]any{"type": "pong"})
		default:
			log.Printf("[api/handlers/handler_terminal.go] unknown terminal client frame id=%s type=%q", session.ID(), message.Type)
		}
	}
}

func writeTerminalError(c *hertzapp.RequestContext, err error) {
	switch {
	case errors.Is(err, terminal.ErrDisabled):
		writeErrorKey(c, consts.StatusServiceUnavailable, "api.terminal.disabled")
	case errors.Is(err, terminal.ErrNotFound):
		writeErrorKey(c, consts.StatusNotFound, "api.terminal.notFound")
	case errors.Is(err, terminal.ErrTooManySessions):
		writeErrorKey(c, consts.StatusTooManyRequests, "api.terminal.tooMany")
	case errors.Is(err, terminal.ErrInvalidProfile):
		log.Printf("[api/handlers/handler_terminal.go] invalid terminal profile err=%v", err)
		writeErrorKey(c, consts.StatusBadRequest, "api.terminal.invalidProfile")
	case errors.Is(err, terminal.ErrInvalidLaunchCommand):
		log.Printf("[api/handlers/handler_terminal.go] invalid terminal launch command err=%v", err)
		writeErrorKey(c, consts.StatusBadRequest, "api.terminal.invalidLaunchCommand")
	default:
		log.Printf("[api/handlers/handler_terminal.go] terminal request failed err=%v", err)
		writeError(c, consts.StatusInternalServerError, err.Error())
	}
}

// allowedTerminalOrigin permits same-origin requests plus the local dev frontend (Vite).
func allowedTerminalOrigin(c *hertzapp.RequestContext) bool {
	origin := strings.TrimSpace(string(c.Request.Header.Peek("Origin")))
	if origin == "" {
		return true
	}
	host := strings.TrimSpace(string(c.Request.Header.Peek("Host")))
	for _, scheme := range []string{"http://", "https://"} {
		if strings.EqualFold(origin, scheme+host) {
			return true
		}
	}
	return isLocalDevOrigin(origin)
}

func isLocalDevOrigin(origin string) bool {
	lowered := strings.ToLower(origin)
	for _, prefix := range []string{"http://localhost:", "http://127.0.0.1:", "https://localhost:", "https://127.0.0.1:"} {
		if strings.HasPrefix(lowered, prefix) {
			return true
		}
	}
	return false
}
