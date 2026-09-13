package platform

import (
	"context"
	"crypto/sha256"
	"denova/config"
	agentlifecycle "denova/internal/agents/lifecycle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"denova/internal/agents/canonicalstore"
	agentrun "denova/internal/agents/run"
	productsession "denova/internal/agents/session"
	agent "github.com/alfredxw/denova/agent"
)

// ModelResolver resolves an explicitly selected profile without exposing keys.
// Its stable identity excludes credentials and must change with model behavior.
type ModelResolver func(context.Context, string) (agent.BaseChatModel, agent.CapabilityIdentity, error)

type AgentService struct {
	manager  *Manager
	store    *canonicalstore.Store
	model    ModelResolver
	mu       sync.Mutex
	projects map[string]*productsession.Store
	live     map[string]*agentExecution
}

type agentConfig struct {
	Caller        PackageRef               `json:"caller"`
	Scope         Scope                    `json:"scope"`
	Key           string                   `json:"key"`
	Definition    string                   `json:"definition"`
	Provider      ReleaseRef               `json:"provider"`
	Content       AgentDefinition          `json:"content"`
	ModelProfile  string                   `json:"modelProfile"`
	ModelIdentity agent.CapabilityIdentity `json:"modelIdentity"`
}

type AgentRef struct {
	Owner struct {
		Kind      string `json:"kind"`
		ProjectID string `json:"projectId"`
		SessionID string `json:"sessionId"`
	} `json:"owner"`
	SessionID string `json:"sessionId"`
}

type AgentSession struct {
	Ref        AgentRef `json:"ref"`
	Definition string   `json:"definition"`
	Key        string   `json:"key"`
}

type RunRef struct {
	Agent AgentRef `json:"agent"`
	RunID string   `json:"runId"`
}
type RunResult struct {
	Run        RunRef `json:"run"`
	Status     string `json:"status"`
	Text       string `json:"text"`
	Error      *Error `json:"error,omitempty"`
	Completion *struct {
		Agent    AgentRef `json:"agent"`
		RecordID string   `json:"recordId"`
	} `json:"completion,omitempty"`
}

type agentReceipt struct {
	CommandID string    `json:"commandId"`
	InputHash string    `json:"inputHash"`
	Result    RunResult `json:"result"`
}

type agentExecution struct {
	runtime  *Runtime
	owner    *agent.Agent
	session  *agent.Session
	product  *productsession.Session
	run      *agent.Run
	receipt  agentReceipt
	revision uint64
	done     chan struct{}
	// Event cursors are process-local. The sequence includes the random process
	// generation so a cursor from before a restart cannot look current.
	generation string
	events     []streamEvent
	next       uint64
	mu         sync.Mutex
}

func (m *Manager) ConfigureAgents(store *canonicalstore.Store, resolver ModelResolver) {
	m.agents = &AgentService{manager: m, store: store, model: resolver, projects: map[string]*productsession.Store{}, live: map[string]*agentExecution{}}
}

func stableID(parts ...string) string {
	raw, _ := json.Marshal(parts)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func agentReference(projectID, sessionID string) AgentRef {
	ref := AgentRef{SessionID: sessionID}
	ref.Owner.Kind = "session"
	ref.Owner.ProjectID = projectID
	ref.Owner.SessionID = sessionID
	return ref
}

func (s *AgentService) product(projectID, sessionID string, create bool) (*productsession.Session, error) {
	_, layout, err := s.manager.registry.Resolve(projectID, true)
	if err != nil {
		return nil, err
	}
	store := s.projects[projectID]
	if store == nil {
		store, err = productsession.NewStore(layout.SessionsDir())
		if err != nil {
			return nil, err
		}
		s.projects[projectID] = store
	}
	if !create && !store.Exists(sessionID) {
		return nil, failure("NOT_FOUND", "Session is unavailable")
	}
	return store.GetOrCreate(sessionID)
}

func (s *AgentService) definition(runtime *Runtime, caller *activation, id string) (agentConfig, error) {
	config := agentConfig{Caller: caller.release.Ref.Package, Scope: caller.context.Scope, Definition: id}
	if id == "builtin/assistant" {
		if caller.release.Manifest.Game != nil && !slices.Contains(caller.release.Manifest.Game.Uses.Agents, id) {
			return config, failure("PERMISSION_DENIED", "Built-in assistant is not selected by this game")
		}
		config.Content = AgentDefinition{Instructions: "Help the user with their requested creative task.", ModelSlot: "assistant"}
		config.Provider = ReleaseRef{Package: PackageRef{Kind: Plugin, ID: "builtin"}, ReleaseID: "v1"}
		config.ModelProfile = runtime.models["builtin/assistant"]
		return config, nil
	}
	if caller.release.Ref.Package.Kind != Game || !strings.HasPrefix(id, "local:") {
		return config, failure("PERMISSION_DENIED", "Only the calling game's private Agent definitions are available")
	}
	localID := strings.TrimPrefix(id, "local:")
	for _, item := range caller.release.Manifest.privateAgents() {
		if item.ID != localID {
			continue
		}
		if err := readJSON(filepath.Join(s.manager.releasePath(caller.release.Ref), filepath.FromSlash(item.Definition)), &config.Content); err != nil {
			return config, err
		}
		config.Provider = caller.release.Ref
		config.ModelProfile = runtime.models["local:"+config.Content.ModelSlot]
		return config, nil
	}
	return config, failure("NOT_FOUND", "Agent definition %s is unavailable", id)
}

type EnsureAgentSession struct {
	ProjectID  string `json:"projectId"`
	Definition string `json:"definition"`
	Key        string `json:"key"`
}

func (s *AgentService) ensure(ctx context.Context, runtime *Runtime, caller *activation, request EnsureAgentSession) (AgentSession, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if request.ProjectID == "" || request.ProjectID != caller.context.Scope.ProjectID {
		return AgentSession{}, false, failure("PERMISSION_DENIED", "Select and authorize a Project when starting this scope")
	}
	if strings.TrimSpace(request.Key) == "" || len(request.Key) > 256 {
		return AgentSession{}, false, failure("INVALID_ARGUMENT", "Session key must contain 1..256 bytes")
	}
	config, err := s.definition(runtime, caller, request.Definition)
	if err != nil {
		return AgentSession{}, false, err
	}
	config.Key = request.Key
	if config.ModelProfile == "" {
		return AgentSession{}, false, failure("NOT_CONFIGURED", "Select a model for %s", request.Definition)
	}
	_, config.ModelIdentity, err = s.model(ctx, config.ModelProfile)
	if err != nil {
		return AgentSession{}, false, failure("NOT_CONFIGURED", "Resolve model profile %s: %v", config.ModelProfile, err)
	}
	id := "platform-" + stableID(string(caller.release.Ref.Package.Kind), caller.release.Manifest.ID, config.Scope.Kind, config.Scope.InstanceID, config.Scope.ProjectID, config.Scope.SessionID, request.Key)
	product, err := s.product(request.ProjectID, id, true)
	if err != nil {
		return AgentSession{}, false, err
	}
	existing, revision, err := product.PlatformRecord(ctx, "configuration")
	if err != nil {
		return AgentSession{}, false, err
	}
	raw, _ := json.Marshal(config)
	if revision == 0 {
		if _, err := product.SetPlatformRecord(ctx, "configuration", 0, raw); err != nil {
			return AgentSession{}, false, err
		}
	} else {
		var frozen agentConfig
		if err := json.Unmarshal(existing, &frozen); err != nil {
			return AgentSession{}, false, err
		}
		canonical, _ := json.Marshal(frozen)
		if string(canonical) != string(raw) {
			return AgentSession{}, false, failure("IDEMPOTENCY_CONFLICT", "Session key %s already has another frozen definition or model selection", request.Key)
		}
	}
	return AgentSession{Ref: agentReference(request.ProjectID, id), Definition: config.Definition, Key: config.Key}, revision == 0, nil
}

func (s *AgentService) authorizedSession(ctx context.Context, caller *activation, id string) (*productsession.Session, agentConfig, error) {
	if !strings.HasPrefix(id, "platform-") || len(id) != len("platform-")+64 {
		return nil, agentConfig{}, failure("NOT_FOUND", "Session is unavailable")
	}
	product, err := s.product(caller.context.Scope.ProjectID, id, false)
	if err != nil {
		return nil, agentConfig{}, err
	}
	raw, _, err := product.PlatformRecord(ctx, "configuration")
	if err != nil {
		return nil, agentConfig{}, err
	}
	var config agentConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		return nil, config, err
	}
	if config.Caller != caller.release.Ref.Package || config.Scope != caller.context.Scope {
		return nil, config, failure("PERMISSION_DENIED", "Session belongs to another product or scope")
	}
	return product, config, nil
}

func (s *AgentService) start(ctx context.Context, runtime *Runtime, caller *activation, id, commandID, text string) (RunResult, error) {
	if commandID == "" || len(commandID) > 256 || strings.TrimSpace(text) == "" || len(text) > 256<<10 {
		return RunResult{}, failure("INVALID_ARGUMENT", "commandId and bounded input text are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	product, frozen, err := s.authorizedSession(ctx, caller, id)
	if err != nil {
		return RunResult{}, err
	}
	requestKey := stableID(commandID)
	inputHash := stableID(text)
	existing, _, err := product.PlatformRecord(ctx, "request/"+requestKey)
	if err != nil {
		return RunResult{}, err
	}
	if len(existing) != 0 {
		var receipt agentReceipt
		if err := json.Unmarshal(existing, &receipt); err != nil {
			return RunResult{}, err
		}
		if receipt.InputHash != inputHash {
			return RunResult{}, failure("IDEMPOTENCY_CONFLICT", "commandId %s already has another input", commandID)
		}
		return s.resultLocked(receipt), nil
	}
	if current := s.live[id]; current != nil {
		select {
		case <-current.done:
			_ = current.owner.Close(context.Background())
			delete(s.live, id)
		default:
			return RunResult{}, failure("SESSION_BUSY", "Session %s already has an active run", id)
		}
	}
	current, err := s.definition(runtime, caller, frozen.Definition)
	if err != nil {
		return RunResult{}, err
	}
	if current.Provider != frozen.Provider {
		return RunResult{}, failure("API_INCOMPATIBLE", "Session requires its frozen definition release")
	}
	model, modelIdentity, err := s.model(runtime.ctx, frozen.ModelProfile)
	if err != nil {
		return RunResult{}, failure("NOT_CONFIGURED", "Resolve model profile %s: %v", frozen.ModelProfile, err)
	}
	if modelIdentity != frozen.ModelIdentity {
		return RunResult{}, failure("DOCUMENT_CONFLICT", "The saved session's model configuration changed; use a new session key")
	}
	tools, err := s.agentTools(runtime, caller, frozen.Content)
	if err != nil {
		return RunResult{}, err
	}
	key, err := (agentrun.RuntimeBinding{AgentKind: agentrun.AgentKindGeneral, Mode: agentrun.ModeAgentChat, ProjectID: frozen.Scope.ProjectID, SessionID: id}).AgentSessionKey()
	if err != nil {
		return RunResult{}, err
	}
	mode := config.AgentApprovalAsk
	if slices.Contains(caller.grants, "tools.write") {
		mode = config.AgentApprovalFullAccess
	}
	permission, err := agentlifecycle.NewPermissionPolicy(agentlifecycle.PermissionConfig{Mode: mode, ProjectID: frozen.Scope.ProjectID, NonInteractive: true})
	if err != nil {
		return RunResult{}, err
	}
	definition := agent.Definition{Key: "platform/" + frozen.Definition, Name: frozen.Definition, Model: model, ModelIdentity: modelIdentity, Instructions: frozen.Content.Instructions, Tools: tools, Canonical: productCanonical{product: product}, Permission: permission}

	owner, err := agent.New(runtime.ctx, definition, agent.WithSessionStore(s.store))
	if err != nil {
		return RunResult{}, err
	}
	session, err := owner.Session(runtime.ctx, key)
	if err != nil {
		_ = owner.Close(context.Background())
		return RunResult{}, err
	}
	messages, err := product.ReadCanonicalMessages(runtime.ctx)
	if err == nil {
		err = session.LoadCanonicalMessages(runtime.ctx, messages)
	}
	if err != nil {
		_ = owner.Close(context.Background())
		return RunResult{}, err
	}
	publicRunID := id + "." + requestKey
	receipt := agentReceipt{CommandID: commandID, InputHash: inputHash, Result: RunResult{Run: RunRef{Agent: agentReference(frozen.Scope.ProjectID, id), RunID: publicRunID}, Status: "accepted"}}
	raw, _ := json.Marshal(receipt)
	revision, err := product.SetPlatformRecord(ctx, "request/"+requestKey, 0, raw)
	if err != nil {
		_ = owner.Close(context.Background())
		return RunResult{}, err
	}
	execution := &agentExecution{runtime: runtime, owner: owner, session: session, product: product, receipt: receipt, revision: revision, done: make(chan struct{}), generation: randomToken()[:16]}
	s.live[id] = execution
	run, err := session.Run(runtime.ctx, agent.Input{Text: text, IdempotencyKey: commandID})
	if err != nil {
		execution.finish("failed", err)
		return execution.receipt.Result, nil
	}
	execution.run = run
	launch("platform_agent_events", func() { execution.finish("incomplete", fmt.Errorf("Agent event worker panicked")) }, func() {
		for event := range run.Events() {
			execution.consume(event)
		}
		result, runErr := run.Wait(context.Background())
		status := string(result.Status)
		if result.Status == agent.ResultBlocked {
			status = "failed"
		}
		execution.finish(status, runErr)
	})
	slog.Info("platform_agent_run_accepted", "session", id, "run", publicRunID, "scope", frozen.Scope.Kind)
	return receipt.Result, nil
}

func (s *AgentService) resultLocked(receipt agentReceipt) RunResult {
	if current := s.live[receipt.Result.Run.Agent.SessionID]; current != nil {
		current.mu.Lock()
		defer current.mu.Unlock()
		if current.receipt.CommandID == receipt.CommandID {
			return current.receipt.Result
		}
	}
	if receipt.Result.Status == "accepted" || receipt.Result.Status == "running" || receipt.Result.Status == "waiting" {
		receipt.Result.Status = "incomplete"
	}
	return receipt.Result
}

func (s *AgentService) StopRuntime(ctx context.Context, runtime *Runtime) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, live := range s.live {
		if live.runtime != runtime {
			continue
		}
		if live.run != nil {
			_, _ = live.run.Abort(ctx, agent.AbortRequest{Reason: "Owning runtime stopped"})
		}
		select {
		case <-live.done:
		case <-ctx.Done():
			return ctx.Err()
		}
		if err := live.owner.Close(ctx); err != nil {
			return err
		}
		delete(s.live, id)
	}
	return nil
}

func (s *AgentService) Close(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, live := range s.live {
		if err := live.owner.Close(ctx); err != nil {
			return err
		}
	}
	for _, store := range s.projects {
		if err := store.Close(); err != nil {
			return err
		}
	}
	return nil
}
