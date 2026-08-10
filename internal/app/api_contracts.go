package app

import (
	agentchat "denova/internal/agents/chat"
	agentexecution "denova/internal/agents/execution"
	"strings"

	agentcontext "denova/internal/agents/context"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	"denova/internal/agents/skills"
	projectdomain "denova/internal/project"
)

// These aliases are the deliberately shared contract values exposed by App.
// HTTP packages depend only on App; transport-specific projections still live
// in internal/api, while App owns use-case validation and orchestration. Keep
// runtime engines, registries, and persistence stores out of this surface.

type (
	AgentEvent       = agentrun.Event
	AgentChatRequest = agentchat.ChatRequest

	CommandKind      = agentexecution.CommandKind
	AgentOperationID = agentrun.OperationID
	AgentCommandID   = agentrun.CommandID

	AgentRuntimeStatus             = agentrun.RuntimeStatus
	AgentRuntimeRecoveryActionKind = agentexecution.RuntimeRecoveryActionKind
	AgentRuntimeRecoveryAction     = agentexecution.RuntimeRecoveryAction

	AgentSessionHistoryEntry         = session.HistoryEntry
	AgentSessionUserMessageReference = agentcontext.UserReference
	AgentSessionMeta                 = session.SessionMeta
	AgentSession                     = session.Session

	SkillScope               = skills.Scope
	SkillCreateMetadata      = skills.CreateMetadata
	SkillGitHubSource        = skills.GitHubSource
	SkillRemoteArchiveSource = skills.RemoteArchiveSource
	ProjectRecord            = projectdomain.Record
)

const (
	AgentKindIDE = agentrun.AgentKindIDE

	CommandSteer        = agentexecution.CommandSteer
	CommandFollowUp     = agentexecution.CommandFollowUp
	CommandNextTurn     = agentexecution.CommandNextTurn
	CommandSteerQueued  = agentexecution.CommandSteerQueued
	CommandCancelQueued = agentexecution.CommandCancelQueued
	CommandAbort        = agentexecution.CommandAbort

	AgentRuntimePhaseIdle = agentrun.RunPhaseIdle

	AgentRuntimeRecoveryAttach           = agentexecution.RuntimeRecoveryAttach
	AgentRuntimeRecoveryAbort            = agentexecution.RuntimeRecoveryAbort
	AgentRuntimeRecoverySteer            = agentexecution.RuntimeRecoverySteer
	AgentRuntimeRecoveryFollowUp         = agentexecution.RuntimeRecoveryFollowUp
	AgentRuntimeRecoveryNextTurn         = agentexecution.RuntimeRecoveryNextTurn
	AgentRuntimeRecoveryCompactContext   = agentexecution.RuntimeRecoveryCompactContext
	AgentRuntimeRecoveryRemoveCompaction = agentexecution.RuntimeRecoveryRemoveCompaction

	SkillScopeUser              = skills.ScopeUser
	MaxSkillInstallArchiveBytes = skills.MaxInstallArchiveBytes
)

var (
	ErrAgentRecoveryRequired             = agentexecution.ErrRecoveryRequired
	ErrAgentRecoveryActionChanged        = agentexecution.ErrRecoveryActionChanged
	ErrAgentRuntimeRecoveryActionChanged = agentexecution.ErrRecoveryActionChanged
	ErrInvalidAgentCommand               = agentrun.ErrInvalidCommand
	ErrInvalidAgentBinding               = agentrun.ErrInvalidBinding
	ErrStaleAgentOperation               = agentrun.ErrStaleOperation
	ErrAgentQueueConflict                = agentrun.ErrQueueConflict
	ErrAgentBusy                         = agentrun.ErrBusy
	ErrAgentDomainCommitRejected         = agentrun.ErrDomainCommitRejected
	ErrSkillRevisionConflict             = skills.ErrRevisionConflict
)

// ValidateAgentCommandID applies the exact durable command envelope used by
// the Agent runtime without exposing runtime configuration to HTTP handlers.
func ValidateAgentCommandID(commandID string) error {
	return agentrun.ValidateCommandID(commandID)
}

// ValidateAgentRecoveryIdentity validates the caller-owned identity for one
// explicit recovery action. Kind validation remains at the transport seam so
// unsupported wire values can be rejected before constructing the request.
func ValidateAgentRecoveryIdentity(commandID, operationID string) error {
	return agentrun.ValidateRecoveryIdentity(commandID, strings.TrimSpace(operationID))
}

// AgentRuntimeRecoveryActions projects only the recovery operations that are
// safe for an external caller to retry against the current durable status.
func AgentRuntimeRecoveryActions(status AgentRuntimeStatus) []AgentRuntimeRecoveryAction {
	return agentexecution.RuntimeRecoveryActions(status)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
