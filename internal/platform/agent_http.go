package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	agent "github.com/alfredxw/denova/agent"
)

type agentRunInput struct {
	CommandID string `json:"commandId" jsonschema:"minLength=1,maxLength=256"`
	Input     struct {
		Text string `json:"text"`
	} `json:"input"`
}

type streamEvent struct {
	sequence uint64
	Kind     string
	Data     any
}

func (e *agentExecution) consume(event agent.Event) {
	e.mu.Lock()
	defer e.mu.Unlock()
	kind := "state"
	var data any
	switch payload := event.Payload.(type) {
	case agent.RunStarted:
		e.receipt.Result.Status = "running"
	case agent.AssistantDelta:
		e.receipt.Result.Text += payload.Delta
		kind = "delta"
		data = map[string]string{"delta": payload.Delta}
	case agent.InteractionRequested:
		e.receipt.Result.Status = "waiting"
		kind = "interaction"
		data = payload.Request
	case agent.InteractionResolved:
		e.receipt.Result.Status = "running"
	default:
		return
	}
	if kind == "state" {
		data = map[string]string{"status": e.receipt.Result.Status}
	}
	e.next++
	e.events = append(e.events, streamEvent{sequence: e.next, Kind: kind, Data: data})
	if len(e.events) > 256 {
		e.events = slices.Clone(e.events[len(e.events)-256:])
	}
}

func (e *agentExecution) finish(status string, runErr error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	select {
	case <-e.done:
		return
	default:
	}
	defer close(e.done)
	e.receipt.Result.Status = status
	if runErr != nil {
		_, e.receipt.Result.Error = ErrorResponse(runErr)
		slog.Warn("platform_agent_run_failed", "run", e.receipt.Result.Run.RunID, "error", runErr)
	}
	if snapshot, err := e.session.Snapshot(context.Background()); err == nil {
		for _, run := range snapshot.RecentRuns {
			if e.run != nil && run.ID == e.run.ID() {
				e.receipt.Result.Text = run.Output
			}
		}
	}
	if status == "completed" {
		page, err := e.product.ReadHistoryPage(context.Background(), -1, 100)
		if err != nil {
			e.receipt.Result.Status = "incomplete"
			_, e.receipt.Result.Error = ErrorResponse(err)
		}
		for i := len(page.Entries) - 1; i >= 0; i-- {
			entry := page.Entries[i]
			if e.run == nil || entry.RunID != e.run.ID() || entry.Role != "assistant" || entry.ID == "" {
				continue
			}
			e.receipt.Result.Completion = &struct {
				Agent    AgentRef `json:"agent"`
				RecordID string   `json:"recordId"`
			}{Agent: e.receipt.Result.Run.Agent, RecordID: entry.ID}
			break
		}
	}
	raw, err := json.Marshal(e.receipt)
	if err == nil {
		_, err = e.product.SetPlatformRecord(context.Background(), "request/"+stableID(e.receipt.CommandID), e.revision, raw)
	}
	if err != nil {
		e.receipt.Result.Status = "incomplete"
		_, e.receipt.Result.Error = ErrorResponse(err)
		slog.Error("platform_agent_receipt_commit_failed", "run", e.receipt.Result.Run.RunID, "error", err)
	}
	e.next++
	e.events = append(e.events, streamEvent{sequence: e.next, Kind: "result", Data: e.receipt.Result})
	slog.Info("platform_agent_run_settled", "run", e.receipt.Result.Run.RunID, "status", e.receipt.Result.Status)
}

func (s *AgentService) ServeHTTP(w http.ResponseWriter, request *http.Request, runtime *Runtime, caller *activation, route string) {
	if !slices.Contains(caller.grants, "agents.run") {
		writeError(w, failure("PERMISSION_DENIED", "agents.run is not granted"))
		return
	}
	if request.Method == "GET" && route == "/agents/definitions" {
		items := []string{}
		if _, err := s.definition(runtime, caller, "builtin/assistant"); err == nil {
			items = append(items, "builtin/assistant")
		}
		for _, entry := range caller.release.Manifest.privateAgents() {
			id := "local:" + entry.ID
			if _, err := s.definition(runtime, caller, id); err == nil {
				items = append(items, id)
			}
		}
		writeResponse(w, 200, map[string]any{"items": items})
		return
	}
	if route == "/agents/sessions" && request.Method == "POST" {
		var input EnsureAgentSession
		if err := readRequest(request, &input); err != nil {
			writeError(w, err)
			return
		}
		session, created, err := s.ensure(request.Context(), runtime, caller, input)
		if err != nil {
			writeError(w, err)
			return
		}
		status := 200
		if created {
			status = 201
		}
		writeResponse(w, status, session)
		return
	}
	if strings.HasPrefix(route, "/agents/sessions/") {
		parts := strings.Split(strings.TrimPrefix(route, "/agents/sessions/"), "/")
		id := parts[0]
		if len(parts) == 2 && parts[1] == "runs" && request.Method == "POST" {
			var input agentRunInput
			if err := readRequest(request, &input); err != nil {
				writeError(w, err)
				return
			}
			result, err := s.start(request.Context(), runtime, caller, id, input.CommandID, input.Input.Text)
			if err != nil {
				writeError(w, err)
				return
			}
			writeResponse(w, 202, result)
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		product, config, err := s.authorizedSession(request.Context(), caller, id)
		if err != nil {
			writeError(w, err)
			return
		}
		if request.Method == "GET" && len(parts) == 1 {
			writeResponse(w, 200, AgentSession{Ref: agentReference(config.Scope.ProjectID, id), Definition: config.Definition, Key: config.Key})
			return
		}
		if request.Method == "GET" && len(parts) == 2 && parts[1] == "history" {
			before, _ := strconv.Atoi(request.URL.Query().Get("cursor"))
			if request.URL.Query().Get("cursor") == "" {
				before = -1
			}
			limit, _ := strconv.Atoi(request.URL.Query().Get("limit"))
			if limit <= 0 || limit > 100 {
				limit = 50
			}
			page, err := product.ReadHistoryPage(request.Context(), before, limit)
			if err != nil {
				writeError(w, err)
				return
			}
			items := []map[string]any{}
			for _, entry := range page.Entries {
				items = append(items, map[string]any{"recordId": entry.ID, "role": entry.Role, "text": entry.Content, "createdAt": entry.CreatedAt})
			}
			cursor := ""
			if page.HasMore {
				cursor = strconv.Itoa(page.NextBefore)
			}
			writeResponse(w, 200, map[string]any{"items": items, "cursor": cursor})
			return
		}
		if request.Method == "GET" && len(parts) == 2 && parts[1] == "runs" {
			raw, _, err := product.PlatformRecord(request.Context(), "request/"+stableID(request.URL.Query().Get("commandId")))
			if err != nil {
				writeError(w, err)
				return
			}
			if len(raw) == 0 {
				writeError(w, failure("NOT_FOUND", "Unknown commandId"))
				return
			}
			var receipt agentReceipt
			if err := json.Unmarshal(raw, &receipt); err != nil {
				writeError(w, err)
				return
			}
			writeResponse(w, 200, s.resultLocked(receipt))
			return
		}
	}
	if strings.HasPrefix(route, "/agents/runs/") {
		parts := strings.Split(strings.TrimPrefix(route, "/agents/runs/"), "/")
		runID := parts[0]
		sessionID, key, ok := strings.Cut(runID, ".")
		if !ok || len(key) != 64 {
			writeError(w, failure("NOT_FOUND", "Unknown run"))
			return
		}
		s.mu.Lock()
		product, _, err := s.authorizedSession(request.Context(), caller, sessionID)
		if err != nil {
			s.mu.Unlock()
			writeError(w, err)
			return
		}
		raw, _, err := product.PlatformRecord(request.Context(), "request/"+key)
		if err != nil || len(raw) == 0 {
			s.mu.Unlock()
			if err == nil {
				err = failure("NOT_FOUND", "Unknown run")
			}
			writeError(w, err)
			return
		}
		var receipt agentReceipt
		err = json.Unmarshal(raw, &receipt)
		if err != nil {
			s.mu.Unlock()
			writeError(w, err)
			return
		}
		result := s.resultLocked(receipt)
		execution := s.live[sessionID]
		if execution != nil && execution.receipt.CommandID != receipt.CommandID {
			execution = nil
		}
		s.mu.Unlock()
		if request.Method == "GET" && len(parts) == 1 {
			writeResponse(w, 200, result)
			return
		}
		if request.Method == "GET" && len(parts) == 2 && parts[1] == "events" {
			serveAgentEvents(w, request, execution, result)
			return
		}
		if request.Method == "POST" && len(parts) == 2 && parts[1] == "cancel" {
			if execution != nil && execution.run != nil {
				if _, err := execution.run.Abort(request.Context(), agent.AbortRequest{Reason: "Cancelled by caller"}); err != nil {
					writeError(w, err)
					return
				}
			}
			writeResponse(w, 200, result)
			return
		}
		if request.Method == "POST" && len(parts) == 4 && parts[1] == "interactions" && parts[3] == "responses" {
			if execution == nil || execution.run == nil {
				writeError(w, failure("RUNTIME_UNAVAILABLE", "Run is not active"))
				return
			}
			snapshot, err := execution.session.Snapshot(request.Context())
			if err != nil {
				writeError(w, err)
				return
			}
			allowed := false
			for _, interaction := range snapshot.PendingInteractions {
				if interaction.ID == parts[2] && interaction.Kind == agent.InteractionAsk {
					allowed = true
				}
			}
			if !allowed {
				writeError(w, failure("PERMISSION_DENIED", "Only ordinary questions can be answered through the consumer API"))
				return
			}
			var answer agent.InteractionResponse
			if err := readRequest(request, &answer); err != nil {
				writeError(w, err)
				return
			}
			if err := execution.run.Respond(request.Context(), parts[2], answer); err != nil {
				writeError(w, err)
				return
			}
			w.WriteHeader(204)
			return
		}
	}
	writeError(w, failure("NOT_FOUND", "Unknown Agent route"))
}

func serveAgentEvents(w http.ResponseWriter, request *http.Request, execution *agentExecution, result RunResult) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, failure("UNSUPPORTED", "Streaming is unavailable"))
		return
	}
	lastID := request.Header.Get("Last-Event-ID")
	var after uint64
	if execution != nil {
		execution.mu.Lock()
		if lastID != "" {
			generation, sequence, _ := strings.Cut(lastID, ":")
			after, _ = strconv.ParseUint(sequence, 10, 64)
			if generation != execution.generation || after > execution.next || (len(execution.events) > 0 && after+1 < execution.events[0].sequence) {
				execution.mu.Unlock()
				writeError(w, failure("CURSOR_EXPIRED", "Request a new snapshot"))
				return
			}
		}
		result = execution.receipt.Result
		after = execution.next
		execution.mu.Unlock()
	} else if lastID != "" {
		writeError(w, failure("CURSOR_EXPIRED", "Request a new snapshot after restart"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	send := func(kind, cursor string, data any) error {
		encoded, err := json.Marshal(data)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(w, "id: %s\nevent: %s\ndata: %s\n\n", cursor, kind, encoded)
		flusher.Flush()
		return err
	}
	cursor := ""
	if execution != nil {
		cursor = execution.generation + ":" + strconv.FormatUint(after, 10)
	}
	if send("snapshot", cursor, map[string]any{"snapshot": result, "cursor": cursor}) != nil || execution == nil {
		return
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-request.Context().Done():
			return
		case <-ticker.C:
		}
		execution.mu.Lock()
		events := slices.Clone(execution.events)
		next := execution.next
		execution.mu.Unlock()
		if len(events) > 0 && after+1 < events[0].sequence {
			_ = send("error", "", map[string]string{"code": "CURSOR_EXPIRED"})
			return
		}
		for _, event := range events {
			if event.sequence <= after {
				continue
			}
			if send(event.Kind, execution.generation+":"+strconv.FormatUint(event.sequence, 10), event.Data) != nil {
				return
			}
		}
		after = next
		select {
		case <-execution.done:
			return
		default:
		}
	}
}
