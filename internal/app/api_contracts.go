package app

import (
	"strings"

	agents "denova/internal/agents"
	"denova/internal/agents/session"
	"denova/internal/agents/skills"
	runstate "github.com/alfredxw/denova/agent/runtime"
)

// These aliases are the deliberately shared contract values exposed by App.
// HTTP packages depend only on App; transport-specific projections still live
// in internal/api, while App owns use-case validation and orchestration. Keep
// runtime engines, registries, and persistence stores out of this surface.

type (
	AgentEvent       = agents.Event
	AgentChatRequest = agents.ChatRequest

	AgentCommandKind = agents.AgentCommandKind
	AgentOperationID = runstate.OperationID
	AgentCommandID   = runstate.CommandID

	AgentRuntimeStatus             = runstate.StatusSnapshot
	AgentRuntimeRecoveryActionKind = agents.RuntimeRecoveryActionKind
	AgentRuntimeRecoveryAction     = agents.RuntimeRecoveryAction

	AgentSessionHistoryEntry         = session.HistoryEntry
	AgentSessionUserMessageReference = session.UserMessageReference
	AgentSessionMeta                 = session.SessionMeta
	AgentSession                     = session.Session

	SkillScope               = skills.Scope
	SkillGitHubSource        = skills.GitHubSource
	SkillRemoteArchiveSource = skills.RemoteArchiveSource
)

const (
	AgentKindIDE = agents.AgentKindIDE

	AgentCommandSteer    = agents.AgentCommandSteer
	AgentCommandFollowUp = agents.AgentCommandFollowUp
	AgentCommandNextTurn = agents.AgentCommandNextTurn
	AgentCommandAbort    = agents.AgentCommandAbort

	AgentRuntimePhaseIdle = runstate.PhaseIdle

	AgentRuntimeRecoveryAttach           = agents.RuntimeRecoveryAttach
	AgentRuntimeRecoveryAbort            = agents.RuntimeRecoveryAbort
	AgentRuntimeRecoverySteer            = agents.RuntimeRecoverySteer
	AgentRuntimeRecoveryFollowUp         = agents.RuntimeRecoveryFollowUp
	AgentRuntimeRecoveryNextTurn         = agents.RuntimeRecoveryNextTurn
	AgentRuntimeRecoveryCompactContext   = agents.RuntimeRecoveryCompactContext
	AgentRuntimeRecoveryRemoveCompaction = agents.RuntimeRecoveryRemoveCompaction

	SkillScopeUser              = skills.ScopeUser
	MaxSkillInstallArchiveBytes = skills.MaxInstallArchiveBytes
)

var (
	ErrAgentRecoveryRequired             = agents.ErrRecoveryRequired
	ErrAgentRecoveryActionChanged        = agents.ErrRecoveryActionChanged
	ErrAgentRuntimeRecoveryActionChanged = runstate.ErrRecoveryActionChanged
	ErrInvalidAgentCommand               = runstate.ErrInvalidCommand
	ErrInvalidAgentBinding               = runstate.ErrInvalidBinding
	ErrStaleAgentOperation               = runstate.ErrStaleOperation
	ErrAgentQueueConflict                = runstate.ErrQueueConflict
	ErrAgentBusy                         = runstate.ErrBusy
	ErrAgentDomainCommitRejected         = runstate.ErrDomainCommitRejected
	ErrSkillRevisionConflict             = skills.ErrRevisionConflict
)

// ValidateAgentCommandID applies the exact durable command envelope used by
// the Agent runtime without exposing runtime configuration to HTTP handlers.
func ValidateAgentCommandID(commandID string) error {
	return runstate.ValidateCommandID(commandID, runstate.DefaultInputLimits())
}

// ValidateAgentRecoveryIdentity validates the caller-owned identity for one
// explicit recovery action. Kind validation remains at the transport seam so
// unsupported wire values can be rejected before constructing the request.
func ValidateAgentRecoveryIdentity(commandID, operationID string) error {
	limits := runstate.DefaultInputLimits()
	if err := runstate.ValidateCommandID(commandID, limits); err != nil {
		return err
	}
	operationID = strings.TrimSpace(operationID)
	if operationID == "" || len(operationID) > limits.MaxOperationIDBytes {
		return runstate.ErrInvalidCommand
	}
	return nil
}

// AgentRuntimeRecoveryActions projects only the recovery operations that are
// safe for an external caller to retry against the current durable status.
func AgentRuntimeRecoveryActions(status AgentRuntimeStatus) []AgentRuntimeRecoveryAction {
	return agents.RuntimeRecoveryActions(status)
}
