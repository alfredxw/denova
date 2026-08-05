package configmanager

import (
	"context"
	"crypto/sha1"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"

	"denova/config"
	chatagent "denova/internal/agents/chat"
	agentcontext "denova/internal/agents/context"
	agentconversation "denova/internal/agents/conversation"
	agentharness "denova/internal/agents/harness"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	agenttool "denova/internal/agents/tool"
	appagentruntime "denova/internal/app/agentruntime"
	appsettings "denova/internal/app/settings"
	apptask "denova/internal/app/task"
)

type Service struct {
	host       Host
	admission  sync.Mutex
	starts     apptask.StartRegistry
	recoveries recoveryRegistry
}

func NewService(host Host) *Service {
	return &Service{
		host:   host,
		starts: apptask.NewStartRegistry(apptask.StartRegistryOptions{Label: "Config Manager"}),
	}
}

type taskRecord struct {
	CommandID string
	Task      *apptask.Task
}

func latestStartTask(registry *apptask.StartRegistry, scope, sessionID string) taskRecord {
	if registry == nil {
		return taskRecord{}
	}
	record := registry.Latest(scope, sessionID)
	return taskRecord{CommandID: record.Identity.CommandID, Task: record.Task}
}

const requestContextValueMaxBytes = 2048

func (service *Service) StartTask(ctx context.Context, request Request) *apptask.Task {
	task, err := service.StartTaskWithError(ctx, request)
	if err != nil {
		slog.ErrorContext(ctx, fmt.Sprintf("[app/configmanager] start failed command_id=%s err=%v", strings.TrimSpace(request.CommandID), err))
		return nil
	}
	return task
}

func (service *Service) StartTaskWithError(ctx context.Context, request Request) (*apptask.Task, error) {
	service.admission.Lock()
	defer service.admission.Unlock()
	request.CommandID = strings.TrimSpace(request.CommandID)
	if request.CommandID == "" {
		return nil, apptask.ErrCommandIDRequired
	}
	if err := agentrun.ValidateCommandID(request.CommandID); err != nil {
		return nil, err
	}
	if service == nil || service.host == nil {
		return nil, appagentruntime.ErrNoWorkspace
	}
	runtime, err := service.runtime(ctx, request)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(runtime.Workspace) == "" {
		return nil, appagentruntime.ErrNoWorkspace
	}
	sessionID, err := SessionID(request)
	if err != nil {
		return nil, err
	}
	message := BuildMessage(request)
	chatRequest := chatagent.CaptureChatRequestCallerInput(chatagent.ChatRequest{
		CommandID: request.CommandID, Message: message,
		LoreReferences: append([]string(nil), request.References...), Locale: request.Locale,
	})
	fingerprint := agentharness.RequestSemanticFingerprint(chatRequest)
	identity := apptask.StartIdentity{
		CommandID: request.CommandID, Scope: runtime.ProjectID,
		SessionID: sessionID, Fingerprint: fingerprint,
	}
	if replay, ok, err := service.starts.Replay(identity); err != nil {
		return nil, err
	} else if ok {
		return replay, nil
	}
	operation, err := service.host.AcquireProjectOperation(ctx, runtime.ProjectID)
	if err != nil {
		return nil, err
	}
	defer operation.Release()
	ctx = operation.Context()
	runtime, err = service.runtime(operation.Context(), request)
	if err != nil {
		return nil, err
	}
	if replay, matched, err := service.replayDurableStart(ctx, runtime, chatRequest, sessionID, fingerprint); err != nil {
		return nil, err
	} else if matched {
		return replay, nil
	}
	if !runtime.available() {
		return nil, appagentruntime.ErrNoWorkspace
	}

	bookWorkspaces := make([]string, 0)
	if runtime.ProjectRegistry != nil {
		books, listErr := runtime.ProjectRegistry.Books()
		if listErr != nil {
			slog.ErrorContext(ctx, fmt.Sprintf("[app/configmanager] list Book workspaces failed err=%v", listErr))
		} else {
			for _, record := range books {
				bookWorkspaces = append(bookWorkspaces, record.WorkspacePath)
			}
		}
	}
	runtimeConfig, err := appsettings.RefreshProject(
		runtime.Config, runtime.Workspace, runtime.Config.ProjectStateDir,
	)
	if err != nil {
		return nil, fmt.Errorf("load Config Manager project settings: %w", err)
	}
	runtimeConfig.AutomationWorkspaces = append(runtimeConfig.AutomationWorkspaces, bookWorkspaces...)
	appsettings.ApplyLocale(&runtimeConfig, request.Locale)
	resourceSkills, err := loadResourceSkills(ctx, &runtimeConfig, request)
	if err != nil {
		return nil, fmt.Errorf("load Config Manager resource Skills / 加载配置管理资源 Skills 失败: %w", err)
	}
	sess, _, err := agentconversation.GetOrCreateSession(
		runtime.SessionStore, sessionID, &runtimeConfig, config.AgentKindConfigManager,
	)
	if err != nil {
		return nil, err
	}
	if _, err := agentconversation.ApplySession(sess, &runtimeConfig, config.AgentKindConfigManager); err != nil {
		return nil, err
	}
	runner, systemPrompt, err := appagentruntime.BuildConfigManager(ctx, &runtimeConfig, runtime.State, resourceSkills...)
	if err != nil {
		return nil, err
	}
	conversation := agentconversation.NewSessionConversationForAgent(sess, &runtimeConfig, config.AgentKindConfigManager)
	task, err := apptask.NewDeferredWithContext(ctx, func(task *apptask.Task) error {
		return service.host.RegisterTask(task, runtime)
	})
	if err != nil {
		return nil, err
	}
	reservation, err := service.starts.Reserve(apptask.StartRecord{Identity: identity, Task: task})
	if err != nil {
		task.RejectStart(err)
		service.host.UnregisterTask(task)
		return nil, err
	}
	acceptCtx, releaseAcceptance := apptask.AcceptanceContext(ctx, task)
	accepted, err := runtime.ChatService.StartWithOptions(
		acceptCtx, runner, conversation, runtime.BookService, chatRequest,
		agentrun.Options{
			AgentKind: agentrun.AgentKindConfigManager, StateRoot: runtimeConfig.ProjectStateDir,
			ProjectID: runtime.ProjectID,
			TaskID:    task.ID(), SessionID: sess.ID, Workspace: runtime.Workspace,
			Mode: RuntimeMode, IdleTimeout: appagentruntime.IdleTimeout(runtimeConfig),
			ToolResultMaxBytes: appagentruntime.ToolResultMaxBytes(runtimeConfig), SystemPromptLog: systemPrompt,
			OnMutationsVerified: func(callbackCtx context.Context, mutations []agenttool.Mutation, verification agenttool.Verification) {
				service.host.OnVerifiedMutations(callbackCtx, "config_manager_post_run", runtime.VersionService, runtimeConfig, mutations, verification)
			},
		},
		task.Emit,
	)
	releaseAcceptance()
	if err != nil {
		reservation.Rollback()
		task.RejectStart(err)
		service.host.UnregisterTask(task)
		return nil, err
	}
	if err := task.Start(func(runCtx context.Context, task *apptask.Task, _ func(agentrun.Event)) {
		defer service.host.UnregisterTask(task)
		slog.InfoContext(runCtx, fmt.Sprintf(
			"[app/configmanager] run begin task_id=%s session_id=%s origin=%s resource_id=%s story_id=%s branch_id=%s message_len=%d",
			task.ID(), sess.ID, request.Origin, request.ResourceID, request.StoryID, request.BranchID, len(message),
		))
		accepted.Wait(runCtx)
		slog.InfoContext(runCtx, fmt.Sprintf("[app/configmanager] run end task_id=%s status=%s", task.ID(), task.Status()))
	}); err != nil {
		reservation.Rollback()
		task.Abort()
		_ = accepted.Wait(task.Context())
		task.Finish()
		service.host.UnregisterTask(task)
		return nil, err
	}
	reservation.Commit()
	return task, nil
}

func (runtime Runtime) available() bool {
	return runtime.Config.DataDir() != "" && runtime.SessionStore != nil &&
		runtime.ChatService != nil && strings.TrimSpace(runtime.Workspace) != ""
}

func (service *Service) runtime(ctx context.Context, request Request) (Runtime, error) {
	if service == nil || service.host == nil {
		return Runtime{}, appagentruntime.ErrNoWorkspace
	}
	projectID := strings.TrimSpace(request.ProjectID)
	if projectID == "" {
		return Runtime{}, fmt.Errorf("Config Manager Project ID is required")
	}
	return service.host.ProjectRuntime(ctx, projectID)
}

func (service *Service) Messages(request Request) ([]session.HistoryEntry, error) {
	sess, err := service.conversationSession(request)
	if err != nil {
		return nil, err
	}
	return sess.History(), nil
}

func (service *Service) MessagesPage(ctx context.Context, request Request, before, limit int) (session.HistoryPage, error) {
	sess, err := service.conversationSession(request)
	if err != nil {
		return session.HistoryPage{}, err
	}
	return sess.ReadHistoryPage(ctx, before, limit)
}

func (service *Service) Clear(request Request) error {
	sess, err := service.conversationSession(request)
	if err != nil {
		return err
	}
	return sess.Clear()
}

func (service *Service) conversationSession(request Request) (*session.Session, error) {
	if service == nil || service.host == nil {
		return nil, appagentruntime.ErrNoWorkspace
	}
	sessionID, err := SessionID(request)
	if err != nil {
		return nil, err
	}
	runtime, err := service.runtime(context.Background(), request)
	if err != nil {
		return nil, err
	}
	if runtime.SessionStore != nil && runtime.SessionStore.Exists(sessionID) {
		return runtime.SessionStore.Get(sessionID)
	}
	store, runtimeConfig, _, err := service.conversationRuntime(request)
	if err != nil {
		return nil, err
	}
	sess, _, err := agentconversation.GetOrCreateSession(
		store, sessionID, &runtimeConfig, config.AgentKindConfigManager,
	)
	return sess, err
}

func (service *Service) conversationRuntime(request Request) (*session.Store, config.Config, string, error) {
	if service == nil || service.host == nil {
		return nil, config.Config{}, "", appagentruntime.ErrNoWorkspace
	}
	sessionID, err := SessionID(request)
	if err != nil {
		return nil, config.Config{}, "", err
	}
	runtime, err := service.runtime(context.Background(), request)
	if err != nil {
		return nil, config.Config{}, "", err
	}
	if runtime.SessionStore == nil || strings.TrimSpace(runtime.Workspace) == "" {
		return nil, config.Config{}, "", appagentruntime.ErrNoWorkspace
	}
	runtimeConfig, err := appsettings.RefreshProject(
		runtime.Config, runtime.Workspace, runtime.Config.ProjectStateDir,
	)
	return runtime.SessionStore, runtimeConfig, sessionID, err
}

// BuildMessage materializes bounded request metadata with explicit provenance
// labels before it enters the model conversation.
func BuildMessage(request Request) string {
	instruction := strings.TrimSpace(request.Instruction)
	lines := []string{"[Module Context / 模块上下文]"}
	appendValue := func(key, value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		value, truncated := agentcontext.TrimUTF8Bytes(value, requestContextValueMaxBytes)
		if truncated {
			value += "\n  ... [truncated at request context limit / 已按请求上下文上限截断]"
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", key, strings.TrimSpace(value)))
	}
	appendValue("origin", request.Origin)
	appendValue("resource_id", request.ResourceID)
	appendValue("story_id", request.StoryID)
	appendValue("branch_id", request.BranchID)
	keys := make([]string, 0, len(request.Context))
	for key := range request.Context {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		appendValue(key, request.Context[key])
	}
	if len(request.References) > 0 {
		lines = append(lines, "- references: "+strings.Join(request.References, ", "))
	}
	lines = append(lines, "", "[User Instruction / 用户指令]", instruction)
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// SessionID derives the durable Config Manager scope. The hash prevents long
// or Unicode resource identifiers from becoming filesystem path authority.
func SessionID(request Request) (string, error) {
	base, ok := session.AgentSessionID(config.AgentKindConfigManager)
	if !ok {
		return "", fmt.Errorf("Config Manager Agent session is not configured / 未配置配置管理 Agent 会话")
	}
	scopeValues := []string{
		strings.TrimSpace(request.Origin), strings.TrimSpace(request.StoryID),
		strings.TrimSpace(request.BranchID), strings.TrimSpace(request.ResourceID),
	}
	hasScope := false
	for _, value := range scopeValues {
		if value != "" {
			hasScope = true
			break
		}
	}
	if !hasScope {
		return base, nil
	}
	segments := []string{base, sessionSegment(request.Origin)}
	if story := sessionSegment(request.StoryID); story != "" {
		segments = append(segments, "story", story)
	}
	if branch := sessionSegment(request.BranchID); branch != "" {
		segments = append(segments, "branch", branch)
	}
	if resource := sessionSegment(request.ResourceID); resource != "" {
		segments = append(segments, "resource", resource)
	}
	sum := sha1.Sum([]byte(strings.Join(scopeValues, "\x00")))
	segments = append(segments, fmt.Sprintf("%x", sum)[:12])
	return strings.Join(segments, "-"), nil
}

func sessionSegment(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	var builder strings.Builder
	lastDash := false
	for _, character := range value {
		valid := (character >= 'a' && character <= 'z') ||
			(character >= '0' && character <= '9') || character == '_'
		if valid {
			builder.WriteRune(character)
			lastDash = false
			continue
		}
		if builder.Len() > 0 && !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	segment := strings.Trim(builder.String(), "-")
	if segment == "" {
		return "scope"
	}
	const maxSegmentLength = 48
	if len(segment) > maxSegmentLength {
		return strings.Trim(segment[:maxSegmentLength], "-")
	}
	return segment
}
