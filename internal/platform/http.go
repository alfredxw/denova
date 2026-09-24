package platform

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"denova/internal/portablepath"
	"denova/internal/revisionfile"
)

func ErrorResponse(err error) (int, *Error) {
	var platformError *Error
	if !errors.As(err, &platformError) {
		code := "RUNTIME_FAILED"
		if os.IsNotExist(err) {
			code = "NOT_FOUND"
		}
		if errors.Is(err, revisionfile.ErrRevisionConflict) {
			code = "DOCUMENT_CONFLICT"
		}
		platformError = &Error{Code: code, MessageKey: "platform.errors." + code, Diagnostic: err.Error()}
	}
	status := http.StatusBadRequest
	switch platformError.Code {
	case "NOT_FOUND":
		status = http.StatusNotFound
	case "PERMISSION_DENIED":
		status = http.StatusForbidden
	case "NOT_CONFIGURED", "UNSUPPORTED", "API_INCOMPATIBLE", "DEPENDENCY_UNAVAILABLE", "SESSION_BUSY", "DOCUMENT_CONFLICT", "IDEMPOTENCY_CONFLICT", "SAVE_INCOMPATIBLE", "CURSOR_EXPIRED":
		status = http.StatusConflict
	case "LIMIT_EXCEEDED":
		status = http.StatusRequestEntityTooLarge
	case "RUNTIME_FAILED", "RUNTIME_UNAVAILABLE":
		status = http.StatusServiceUnavailable
	}
	return status, platformError
}

func writeError(w http.ResponseWriter, err error) {
	status, body := ErrorResponse(err)
	slog.Warn("platform_request_failed", "code", body.Code, "diagnostic", body.Diagnostic)
	writeResponse(w, status, body)
}

func writeResponse(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func readRequest(r *http.Request, out any) error {
	data, err := io.ReadAll(io.LimitReader(r.Body, MaxDefinitionBytes+1))
	if err != nil {
		return err
	}
	if len(data) > MaxDefinitionBytes {
		return failure("LIMIT_EXCEEDED", "Request exceeds %d bytes", MaxDefinitionBytes)
	}
	return decodeJSON(data, out)
}

func (r *Runtime) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "same-origin")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	if request.Host != strings.TrimPrefix(r.baseURL, "http://") {
		writeError(w, failure("PERMISSION_DENIED", "Runtime host mismatch"))
		return
	}
	if r.ctx.Err() != nil {
		writeError(w, failure("RUNTIME_UNAVAILABLE", "Runtime has stopped"))
		return
	}
	r.mu.RLock()
	status := r.status
	r.mu.RUnlock()
	if status != "running" {
		writeError(w, failure("RUNTIME_UNAVAILABLE", "Runtime is not accepting requests"))
		return
	}
	if !strings.HasPrefix(request.URL.Path, "/api/platform/v1/") {
		if r.hostOnly {
			writeError(w, failure("PERMISSION_DENIED", "Host Agent runtimes do not expose views"))
			return
		}
		w.Header().Set("Content-Security-Policy", "frame-ancestors "+r.parentOrigin+"; object-src 'none'; base-uri 'self'")
		r.serveView(w, request)
		return
	}
	origin := request.Header.Get("Origin")
	if origin != "" && origin != r.baseURL && origin != r.parentOrigin {
		writeError(w, failure("PERMISSION_DENIED", "Runtime origin mismatch"))
		return
	}
	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
	}
	if request.Method == "OPTIONS" {
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Last-Event-ID")
		w.WriteHeader(204)
		return
	}
	var caller *activation
	token := strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
	for _, provider := range r.providers {
		if subtle.ConstantTimeCompare([]byte(token), []byte(provider.connection.Token)) == 1 {
			caller = provider
			break
		}
	}
	if caller == nil {
		writeError(w, failure("PERMISSION_DENIED", "A valid scope credential is required"))
		return
	}
	ctx, cancel := context.WithCancel(request.Context())
	defer cancel()
	stop := context.AfterFunc(r.ctx, cancel)
	defer stop()
	request = request.WithContext(ctx)
	route := strings.TrimPrefix(request.URL.Path, "/api/platform/v1")
	switch {
	case request.Method == "GET" && route == "/context":
		writeResponse(w, 200, caller.context)
	case route == "/settings":
		r.serveSettings(w, request, caller)
	case request.Method == "GET" && route == "/capabilities":
		writeResponse(w, 200, map[string]any{"permissions": caller.grants, "apiMajor": APIMajor, "limits": map[string]int{"requestBytes": MaxDefinitionBytes, "fileBytes": MaxFileBytes, "instructionsBytes": 256 << 10, "assetBytes": MaxAssetBytes, "libraryItemBytes": MaxLibraryItemBytes, "libraryPageItems": 100, "imagePromptBytes": 64 << 10}, "schemaDialect": "https://json-schema.org/draft/2020-12/schema"})
	case request.Method == "GET" && route == "/openapi.json":
		writeResponse(w, 200, OpenAPI())
	case request.Method == "GET" && route == "/contributions":
		writeResponse(w, 200, r.contributionCatalog(caller))
	case route == "/story/instances" && request.Method == http.MethodPost:
		r.serveCreateStory(w, request, caller)
	case route == "/story" || strings.HasPrefix(route, "/story/"):
		r.serveStory(w, request, caller, route)
	case strings.HasPrefix(route, "/library/") || strings.HasPrefix(route, "/assets/") || strings.HasPrefix(route, "/images/"):
		if r.manager.resources == nil {
			writeError(w, failure("NOT_CONFIGURED", "Resource service is unavailable"))
			return
		}
		r.manager.resources.ServeHTTP(w, request, r, caller, route)
	case strings.HasPrefix(route, "/game-data/") || strings.HasPrefix(route, "/plugin-data/"):
		r.serveData(w, request, caller, route)
	case strings.HasPrefix(route, "/tools/") && strings.HasSuffix(route, "/invoke") && request.Method == "POST":
		parts := strings.Split(strings.TrimPrefix(route, "/tools/"), "/")
		if len(parts) != 3 {
			writeError(w, failure("INVALID_ARGUMENT", "Invalid tool route"))
			return
		}
		if !slices.Contains(caller.grants, "tools.invoke") {
			writeError(w, failure("PERMISSION_DENIED", "tools.invoke is not granted"))
			return
		}
		var input struct {
			Input json.RawMessage `json:"input"`
		}
		if err := readRequest(request, &input); err != nil {
			writeError(w, err)
			return
		}
		result, err := r.invoke(request.Context(), caller, parts[0], parts[1], input.Input)
		if err != nil {
			writeError(w, err)
			return
		}
		writeResponse(w, 200, result)
	case strings.HasPrefix(route, "/agents/"):
		if r.manager.agents == nil {
			writeError(w, failure("NOT_CONFIGURED", "Agent service is unavailable"))
			return
		}
		r.manager.agents.ServeHTTP(w, request, r, caller, route)
	default:
		writeError(w, failure("NOT_FOUND", "Unknown platform route %s %s", request.Method, route))
	}
}

func (r *Runtime) serveView(w http.ResponseWriter, request *http.Request) {
	if origin := request.Header.Get("Origin"); (origin != "" && origin != r.baseURL && origin != r.parentOrigin) || request.Header.Get("Sec-Fetch-Site") == "cross-site" && request.Method != "GET" && request.Method != "HEAD" {
		writeError(w, failure("PERMISSION_DENIED", "View origin mismatch"))
		return
	}
	parts := strings.SplitN(strings.TrimPrefix(request.URL.Path, "/views/"), "/", 2)
	if !strings.HasPrefix(request.URL.Path, "/views/") || len(parts) != 2 {
		writeError(w, failure("NOT_FOUND", "Unknown view"))
		return
	}
	for _, view := range r.owner.release.Manifest.Views {
		if view.ID != parts[0] {
			continue
		}
		if view.Source.Kind == "backend" {
			if r.owner.process == nil {
				writeError(w, failure("RUNTIME_UNAVAILABLE", "Backend is not ready"))
				return
			}
			target, _ := url.Parse(r.owner.process.baseURL)
			proxy := httputil.NewSingleHostReverseProxy(target)
			proxy.Transport = localHTTPClient.Transport
			original := proxy.Director
			proxy.Director = func(req *http.Request) {
				original(req)
				req.URL.Path = path.Join(view.Source.Path, parts[1])
				req.Header.Del("Cookie")
				req.Header.Set("Authorization", "Bearer "+r.owner.process.token)
			}
			proxy.ModifyResponse = func(response *http.Response) error {
				response.Header.Del("Set-Cookie")
				response.Header.Del("Access-Control-Allow-Origin")
				response.Header.Set("Content-Security-Policy", "frame-ancestors "+r.parentOrigin)
				return nil
			}
			proxy.ServeHTTP(w, request)
			return
		}
		if request.Method != "GET" && request.Method != "HEAD" {
			writeError(w, failure("PERMISSION_DENIED", "Static views are read-only"))
			return
		}
		name := parts[1]
		if name == "" {
			name = path.Base(view.Source.Path)
		}
		if err := portablepath.Validate(name); err != nil {
			writeError(w, failure("INVALID_ARGUMENT", "%v", err))
			return
		}
		root, err := os.OpenRoot(filepath.Join(r.manager.releasePath(r.owner.release.Ref), filepath.FromSlash(path.Dir(view.Source.Path))))
		if err != nil {
			writeError(w, err)
			return
		}
		defer root.Close()
		file, err := root.Open(name)
		if err != nil {
			writeError(w, err)
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			writeError(w, failure("NOT_FOUND", "View asset is unavailable"))
			return
		}
		w.Header().Set("Content-Type", mime.TypeByExtension(path.Ext(name)))
		http.ServeContent(w, request, name, info.ModTime(), file)
		return
	}
	writeError(w, failure("NOT_FOUND", "Undeclared view"))
}

type FileSnapshot struct {
	Content  string `json:"content"`
	Revision string `json:"revision"`
}

func (r *Runtime) serveData(w http.ResponseWriter, request *http.Request, caller *activation, route string) {
	permission := "gameData"
	prefix := "/game-data"
	kind := Game
	if strings.HasPrefix(route, "/plugin-data/") {
		permission = "pluginData"
		prefix = "/plugin-data"
		kind = Plugin
	}
	if !slices.Contains(caller.grants, permission) || caller.release.Ref.Package.Kind != kind {
		writeError(w, failure("PERMISSION_DENIED", "Own %s permission is required", permission))
		return
	}
	// Backends own their dataDir directly; the facade must never overwrite an
	// open SQLite/database file behind the author's transaction boundary.
	if caller.process != nil {
		writeError(w, failure("UNSUPPORTED", "This backend owns dataDir directly"))
		return
	}
	r.dataMu.Lock()
	defer r.dataMu.Unlock()
	if err := portablepath.PreflightTree(caller.dataDir); err != nil {
		writeError(w, err)
		return
	}
	if request.Method == "GET" && route == prefix+"/files" {
		directory := request.URL.Query().Get("directory")
		if directory == "" {
			directory = "."
		}
		if directory != "." {
			if err := portablepath.Validate(directory); err != nil {
				writeError(w, err)
				return
			}
		}
		entries, err := os.ReadDir(filepath.Join(caller.dataDir, filepath.FromSlash(directory)))
		if err != nil {
			writeError(w, err)
			return
		}
		items := []map[string]any{}
		for _, entry := range entries {
			info, err := entry.Info()
			if err != nil {
				writeError(w, err)
				return
			}
			items = append(items, map[string]any{"path": path.Join(directory, entry.Name()), "directory": entry.IsDir(), "bytes": info.Size()})
		}
		writeResponse(w, 200, map[string]any{"items": items})
		return
	}
	if route != prefix+"/file" {
		writeError(w, failure("NOT_FOUND", "Unknown data route"))
		return
	}
	name := request.URL.Query().Get("path")
	var mutation ContentDocument
	if request.Method == "PUT" || request.Method == "DELETE" {
		if err := readRequest(request, &mutation); err != nil {
			writeError(w, err)
			return
		}
		name = mutation.Path
	}
	if err := portablepath.CheckNoCollision(caller.dataDir, name); err != nil {
		writeError(w, failure("INVALID_ARGUMENT", "%v", err))
		return
	}
	filePath := filepath.Join(caller.dataDir, filepath.FromSlash(name))
	snapshot, err := revisionfile.Read(request.Context(), filePath)
	if err != nil {
		writeError(w, err)
		return
	}
	switch request.Method {
	case "GET":
		if !snapshot.Exists {
			writeError(w, failure("NOT_FOUND", "File %s does not exist", name))
			return
		}
		if len(snapshot.Content) > MaxFileBytes {
			writeError(w, failure("LIMIT_EXCEEDED", "File exceeds text API limit"))
			return
		}
		if !utf8.Valid(snapshot.Content) {
			writeError(w, failure("UNSUPPORTED", "The text file API requires UTF-8 content"))
			return
		}
		writeResponse(w, 200, FileSnapshot{string(snapshot.Content), snapshot.Revision})
	case "PUT", "DELETE":
		expected := revisionfile.MissingRevision
		if mutation.ExpectedRevision != nil {
			expected = *mutation.ExpectedRevision
			if expected == "" {
				writeError(w, failure("INVALID_ARGUMENT", "expectedRevision must be null or a revision"))
				return
			}
		}
		if expected != snapshot.Revision {
			writeError(w, failure("DOCUMENT_CONFLICT", "File %s changed since it was read", name))
			return
		}
		if request.Method == "PUT" {
			result, err := revisionfile.ReplaceIfRevision(request.Context(), filePath, expected, []byte(mutation.Content), revisionfile.Options{FileMode: 0o600, DirectoryMode: 0o700})
			if err != nil {
				writeError(w, err)
				return
			}
			writeResponse(w, 200, map[string]string{"revision": result.Revision})
		} else {
			if snapshot.Exists {
				if err := os.Remove(filePath); err != nil {
					writeError(w, err)
					return
				}
			}
			w.WriteHeader(204)
		}
	default:
		writeError(w, failure("NOT_FOUND", "Unknown data method"))
	}
}

func (r *Runtime) contributionCatalog(caller *activation) []map[string]any {
	items := []map[string]any{}
	for _, provider := range r.providers {
		for _, id := range contributionIDs(provider.release.Manifest.contributions()) {
			if r.visible(caller, provider, id) {
				items = append(items, map[string]any{"id": provider.release.Manifest.ID + "/" + id, "release": provider.release.Ref})
			}
		}
	}
	return items
}

func (r *Runtime) visible(caller, provider *activation, id string) bool {
	if caller == provider {
		return true
	}
	if provider.release.Ref.Package.Kind == Game {
		return false
	}
	for _, dependency := range caller.release.Manifest.Requires {
		if dependency.PluginID == provider.release.Manifest.ID && slices.Contains(dependency.Contributions, id) {
			return true
		}
	}
	return false
}

type ToolResult struct {
	Content string          `json:"content"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (r *Runtime) invoke(ctx context.Context, caller *activation, providerID, toolID string, input json.RawMessage) (ToolResult, error) {
	return r.invokeTool(ctx, caller, providerID, toolID, input, consumerToolInvocation)
}

type toolInvocationAuthority uint8

const (
	consumerToolInvocation toolInvocationAuthority = iota
	authorizedAgentToolInvocation
)

// Only the host's Agent Tool adapter may use authorizedAgentToolInvocation;
// backend requests always retain the consumer capability checks.
func (r *Runtime) invokeTool(ctx context.Context, caller *activation, providerID, toolID string, input json.RawMessage, authority toolInvocationAuthority) (ToolResult, error) {
	provider := r.providers[providerID]
	if providerID == "local" {
		if caller.release.Ref.Package.Kind != Game {
			return ToolResult{}, failure("PERMISSION_DENIED", "local tools belong to the calling game")
		}
		provider = caller
	}
	if provider == nil {
		return ToolResult{}, failure("DEPENDENCY_UNAVAILABLE", "Provider %s is unavailable", providerID)
	}
	allowed := r.visible(caller, provider, toolID)
	if caller.release.Manifest.Game != nil && caller != provider {
		allowed = false
	}
	for _, set := range provider.release.Manifest.contributions().Toolsets {
		if !slices.Contains(set.Tools, toolID) {
			continue
		}
		if caller.release.Manifest.Game != nil {
			allowed = allowed || slices.Contains(caller.release.Manifest.Game.Uses.Toolsets, providerID+"/"+set.ID)
		} else {
			allowed = allowed || r.visible(caller, provider, set.ID)
		}
	}
	if !allowed {
		return ToolResult{}, failure("PERMISSION_DENIED", "Tool %s/%s is not selected", providerID, toolID)
	}
	for _, tool := range provider.release.Manifest.contributions().Tools {
		if tool.ID != toolID {
			continue
		}
		if provider.process == nil {
			return ToolResult{}, failure("RUNTIME_UNAVAILABLE", "Tool backend is unavailable")
		}
		var definition ToolDefinition
		if err := readJSON(filepath.Join(r.manager.releasePath(provider.release.Ref), filepath.FromSlash(tool.Definition)), &definition); err != nil {
			return ToolResult{}, err
		}
		if (definition.Effect == "write" || definition.Effect == "propose") && authority != authorizedAgentToolInvocation && !slices.Contains(caller.grants, "tools.write") {
			return ToolResult{}, failure("PERMISSION_DENIED", "Side-effecting tools require explicit tools.write authorization")
		}
		schema, err := compileSchema(definition.InputSchema)
		if err != nil {
			return ToolResult{}, err
		}
		if err := validateValue(schema, input); err != nil {
			return ToolResult{}, err
		}
		request, err := http.NewRequestWithContext(ctx, "POST", provider.process.baseURL+tool.Endpoint.Path, strings.NewReader(string(input)))
		if err != nil {
			return ToolResult{}, err
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+provider.process.token)
		response, err := localHTTPClient.Do(request)
		if err != nil {
			return ToolResult{}, failure("RUNTIME_FAILED", "Call %s/%s: %v", providerID, toolID, err)
		}
		defer response.Body.Close()
		data, err := io.ReadAll(io.LimitReader(response.Body, MaxDefinitionBytes+1))
		if err != nil {
			return ToolResult{}, err
		}
		if len(data) > MaxDefinitionBytes {
			return ToolResult{}, failure("LIMIT_EXCEEDED", "Tool result exceeds limit")
		}
		if response.StatusCode != 200 {
			return ToolResult{}, failure("RUNTIME_FAILED", "Tool %s/%s returned HTTP %d", providerID, toolID, response.StatusCode)
		}
		var result ToolResult
		if err := decodeJSON(data, &result); err != nil {
			return ToolResult{}, err
		}
		if len(definition.OutputSchema) > 0 {
			schema, err := compileSchema(definition.OutputSchema)
			if err != nil {
				return ToolResult{}, err
			}
			if err := validateValue(schema, result.Data); err != nil {
				return ToolResult{}, err
			}
		}
		slog.Info("platform_tool_completed", "runtime", r.id, "provider", providerID, "tool", toolID)
		return result, nil
	}
	return ToolResult{}, failure("NOT_FOUND", "Unknown tool %s/%s", providerID, toolID)
}
