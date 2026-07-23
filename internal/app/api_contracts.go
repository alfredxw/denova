package app

import (
	"strings"

	"denova/internal/agent"
	runstate "denova/internal/agent/runtime"
	"denova/internal/agent/session"
	"denova/internal/agent/skills"
)

// These aliases are the deliberately shared contract values exposed by App.
// HTTP packages depend only on App; transport-specific projections still live
// in internal/api, while App owns use-case validation and orchestration. Keep
// runtime engines, registries, and persistence stores out of this surface.

type (
	AgentEvent       = agent.Event
	AgentChatRequest = agent.ChatRequest

	AgentCommandKind = agent.AgentCommandKind
	AgentOperationID = runstate.OperationID
	AgentCommandID   = runstate.CommandID

	AgentRuntimeStatus             = runstate.StatusSnapshot
	AgentRuntimeRecoveryActionKind = agent.RuntimeRecoveryActionKind
	AgentRuntimeRecoveryAction     = agent.RuntimeRecoveryAction

	AgentSessionHistoryEntry         = session.HistoryEntry
	AgentSessionUserMessageReference = session.UserMessageReference
	AgentSessionMeta                 = session.SessionMeta
	AgentSession                     = session.Session

	SkillScope               = skills.Scope
	SkillGitHubSource        = skills.GitHubSource
	SkillRemoteArchiveSource = skills.RemoteArchiveSource
)

const (
	AgentKindIDE = agent.AgentKindIDE

	AgentCommandSteer    = agent.AgentCommandSteer
	AgentCommandFollowUp = agent.AgentCommandFollowUp
	AgentCommandNextTurn = agent.AgentCommandNextTurn
	AgentCommandAbort    = agent.AgentCommandAbort

	AgentRuntimePhaseIdle = runstate.PhaseIdle

	AgentRuntimeRecoveryAttach           = agent.RuntimeRecoveryAttach
	AgentRuntimeRecoveryAbort            = agent.RuntimeRecoveryAbort
	AgentRuntimeRecoverySteer            = agent.RuntimeRecoverySteer
	AgentRuntimeRecoveryFollowUp         = agent.RuntimeRecoveryFollowUp
	AgentRuntimeRecoveryNextTurn         = agent.RuntimeRecoveryNextTurn
	AgentRuntimeRecoveryCompactContext   = agent.RuntimeRecoveryCompactContext
	AgentRuntimeRecoveryRemoveCompaction = agent.RuntimeRecoveryRemoveCompaction

	SkillScopeUser              = skills.ScopeUser
	MaxSkillInstallArchiveBytes = skills.MaxInstallArchiveBytes
)

var (
	ErrAgentRecoveryRequired             = agent.ErrRecoveryRequired
	ErrAgentRecoveryActionChanged        = agent.ErrRecoveryActionChanged
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
	return agent.RuntimeRecoveryActions(status)
}
