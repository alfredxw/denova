package handlers

import appsvc "denova/internal/app"

// HTTP projection DTOs intentionally do not mirror runtime structs. This
// keeps internal journal/context fields out of the shared writing/game API as
// the runtime evolves.
type agentRuntimeProjectionDTO struct {
	Cursor             uint64                          `json:"cursor"`
	Phase              string                          `json:"phase"`
	RecoveryPaused     bool                            `json:"recovery_paused,omitempty"`
	ActiveOperationID  string                          `json:"active_operation_id"`
	ActiveCycle        int                             `json:"active_cycle"`
	ActiveOutput       agentRuntimeActiveOutputDTO     `json:"active_output"`
	Queue              []agentRuntimeQueueDTO          `json:"queue"`
	OpenTools          []agentRuntimeOpenToolDTO       `json:"open_tools"`
	LastOperation      *agentRuntimeOperationDTO       `json:"last_operation,omitempty"`
	RuntimeRecoverable bool                            `json:"runtime_recoverable"`
	StreamAttached     bool                            `json:"stream_attached"`
	RecoveryActions    []agentRuntimeRecoveryActionDTO `json:"recovery_actions"`
}

type agentRuntimeRecoveryActionDTO struct {
	Kind        string `json:"kind"`
	CommandID   string `json:"command_id"`
	OperationID string `json:"operation_id"`
}

type agentRuntimeActiveOutputDTO struct {
	OperationID       string `json:"operation_id"`
	Cycle             int    `json:"cycle"`
	Content           string `json:"content"`
	Thinking          string `json:"thinking"`
	ContentTruncated  bool   `json:"content_truncated,omitempty"`
	ThinkingTruncated bool   `json:"thinking_truncated,omitempty"`
}

type agentRuntimeQueueDTO struct {
	CommandID        string `json:"command_id"`
	OperationID      string `json:"operation_id"`
	Delivery         string `json:"delivery"`
	Message          string `json:"message"`
	MessageTruncated bool   `json:"message_truncated,omitempty"`
}

type agentRuntimeOpenToolDTO struct {
	CallID      string `json:"call_id"`
	Name        string `json:"name"`
	OperationID string `json:"operation_id"`
	Cycle       int    `json:"cycle"`
}

type agentRuntimeOperationDTO struct {
	OperationID     string `json:"operation_id"`
	CommandID       string `json:"command_id"`
	Status          string `json:"status"`
	Reason          string `json:"reason,omitempty"`
	ReasonTruncated bool   `json:"reason_truncated,omitempty"`
}

const agentRuntimeProjectionTextMaxBytes = 1 << 20

type agentRuntimeProjectionOptions struct {
	Available       bool
	StreamAttached  bool
	RecoveryActions []appsvc.AgentRuntimeRecoveryAction
}

func addAgentRuntimeProjection(
	response map[string]interface{},
	snapshot appsvc.AgentRuntimeStatus,
	options agentRuntimeProjectionOptions,
) {
	if !options.Available {
		return
	}
	dto := newAgentRuntimeProjectionDTO(snapshot)
	dto.StreamAttached = options.StreamAttached
	if len(options.RecoveryActions) > 0 {
		// Service-owned refresh actions are emitted only after the durable
		// structural operation has settled. If the actor projection itself is
		// temporarily unavailable, keep the public protocol exhaustive instead
		// of exposing an empty phase alongside an actionable recovery fence.
		if dto.Phase == "" {
			dto.Phase = string(appsvc.AgentRuntimePhaseIdle)
		}
		dto.RecoveryPaused = true
		dto.RuntimeRecoverable = true
		dto.RecoveryActions = make([]agentRuntimeRecoveryActionDTO, 0, len(options.RecoveryActions))
		for _, action := range options.RecoveryActions {
			dto.RecoveryActions = append(dto.RecoveryActions, agentRuntimeRecoveryActionDTO{
				Kind: string(action.Kind), CommandID: string(action.CommandID), OperationID: string(action.OperationID),
			})
		}
	}
	response["cursor"] = dto.Cursor
	response["phase"] = dto.Phase
	response["recovery_paused"] = dto.RecoveryPaused
	response["active_operation_id"] = dto.ActiveOperationID
	response["active_cycle"] = dto.ActiveCycle
	response["active_output"] = dto.ActiveOutput
	response["queue"] = dto.Queue
	response["open_tools"] = dto.OpenTools
	response["runtime_recoverable"] = dto.RuntimeRecoverable
	response["stream_attached"] = dto.StreamAttached
	response["recovery_actions"] = dto.RecoveryActions
	if dto.LastOperation != nil {
		response["last_operation"] = dto.LastOperation
	}
}

func newAgentRuntimeProjectionDTO(snapshot appsvc.AgentRuntimeStatus) agentRuntimeProjectionDTO {
	content, contentTruncated := boundedRuntimeProjectionText(snapshot.ActiveOutput.Content)
	thinking, thinkingTruncated := boundedRuntimeProjectionText(snapshot.ActiveOutput.Thinking)
	queue := make([]agentRuntimeQueueDTO, 0, len(snapshot.Queue))
	for _, item := range snapshot.Queue {
		message, truncated := boundedRuntimeProjectionText(item.Message)
		queue = append(queue, agentRuntimeQueueDTO{
			CommandID:        string(item.CommandID),
			OperationID:      string(item.OperationID),
			Delivery:         string(item.Delivery),
			Message:          message,
			MessageTruncated: item.MessageTruncated || truncated,
		})
	}
	openTools := make([]agentRuntimeOpenToolDTO, 0, len(snapshot.OpenToolCalls))
	for _, call := range snapshot.OpenToolCalls {
		openTools = append(openTools, agentRuntimeOpenToolDTO{
			CallID:      call.CallID,
			Name:        call.Name,
			OperationID: string(call.OperationID),
			Cycle:       call.Cycle,
		})
	}
	var lastOperation *agentRuntimeOperationDTO
	if snapshot.LastOperation != nil {
		reason, truncated := boundedRuntimeProjectionText(snapshot.LastOperation.Reason)
		lastOperation = &agentRuntimeOperationDTO{
			OperationID:     string(snapshot.LastOperation.OperationID),
			CommandID:       string(snapshot.LastOperation.CommandID),
			Status:          string(snapshot.LastOperation.Status),
			Reason:          reason,
			ReasonTruncated: snapshot.LastOperation.ReasonTruncated || truncated,
		}
	}
	recoveryPlan := appsvc.AgentRuntimeRecoveryActions(snapshot)
	recoveryActions := make([]agentRuntimeRecoveryActionDTO, 0, len(recoveryPlan))
	for _, action := range recoveryPlan {
		recoveryActions = append(recoveryActions, agentRuntimeRecoveryActionDTO{
			Kind: string(action.Kind), CommandID: string(action.CommandID), OperationID: string(action.OperationID),
		})
	}
	return agentRuntimeProjectionDTO{
		Cursor:            uint64(snapshot.Cursor),
		Phase:             string(snapshot.Phase),
		RecoveryPaused:    snapshot.RecoveryPaused,
		ActiveOperationID: string(snapshot.ActiveOperation),
		ActiveCycle:       snapshot.ActiveCycle,
		ActiveOutput: agentRuntimeActiveOutputDTO{
			OperationID:       string(snapshot.ActiveOutput.OperationID),
			Cycle:             snapshot.ActiveOutput.Cycle,
			Content:           content,
			Thinking:          thinking,
			ContentTruncated:  snapshot.ActiveOutput.ContentTruncated || contentTruncated,
			ThinkingTruncated: snapshot.ActiveOutput.ThinkingTruncated || thinkingTruncated,
		},
		Queue:              queue,
		OpenTools:          openTools,
		LastOperation:      lastOperation,
		RuntimeRecoverable: len(recoveryActions) > 0,
		RecoveryActions:    recoveryActions,
	}
}

func boundedRuntimeProjectionText(value string) (string, bool) {
	if len(value) <= agentRuntimeProjectionTextMaxBytes {
		return value, false
	}
	limit := agentRuntimeProjectionTextMaxBytes
	for limit > 0 && (value[limit]&0xc0) == 0x80 {
		limit--
	}
	return value[:limit], true
}
