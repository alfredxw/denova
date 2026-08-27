package app

import (
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"

	agentattachment "denova/internal/agents/attachment"
	chatagent "denova/internal/agents/chat"
	"denova/internal/interactive"
)

// MaterializeWritingAttachments persists transport payloads in the selected
// Session's project state before the durable Agent command is admitted.
func (a *App) MaterializeWritingAttachments(sessionID, commandID string, request *chatagent.ChatRequest) error {
	if request == nil || len(request.AttachmentUploads) == 0 {
		return nil
	}
	a.mu.RLock()
	workspace := strings.TrimSpace(a.workspace)
	activeSessionID := ""
	if a.session != nil {
		activeSessionID = a.session.ID
	}
	a.mu.RUnlock()
	if workspace == "" || activeSessionID == "" || activeSessionID != strings.TrimSpace(sessionID) {
		return ErrAgentContextChanged
	}
	layout, err := a.projectLayoutForWorkspace(workspace)
	if err != nil {
		return err
	}
	return materializeChatAttachments(layout.StateRoot, agentattachment.SessionScope(activeSessionID), commandID, request)
}

// MaterializeInteractiveAttachments binds game copies to the Story so deleting
// the Story can remove every attachment in one scoped operation.
func (a *App) MaterializeInteractiveAttachments(storyID, commandID string, request *chatagent.ChatRequest) error {
	if request == nil || len(request.AttachmentUploads) == 0 {
		return nil
	}
	a.mu.RLock()
	workspace := strings.TrimSpace(a.workspace)
	a.mu.RUnlock()
	if workspace == "" || strings.TrimSpace(storyID) == "" {
		return ErrNoWorkspace
	}
	store := a.interactiveService().store()
	if store == nil {
		return ErrNoWorkspace
	}
	if _, err := store.Snapshot(storyID, ""); err != nil {
		return err
	}
	layout, err := a.projectLayoutForWorkspace(workspace)
	if err != nil {
		return err
	}
	return materializeChatAttachments(layout.StateRoot, agentattachment.StoryScope(storyID), commandID, request)
}

func attachmentDescriptors(files []agent.Attachment) []agent.Attachment {
	result := append([]agent.Attachment(nil), files...)
	for index := range result {
		result[index].Path = ""
		result[index].SHA256 = ""
	}
	return result
}

func redactInteractiveSnapshotAttachmentPaths(snapshot *interactive.Snapshot) {
	if snapshot == nil {
		return
	}
	for index := range snapshot.Turns {
		snapshot.Turns[index].Attachments = attachmentDescriptors(snapshot.Turns[index].Attachments)
	}
	for index := range snapshot.PendingPlayerInputs {
		snapshot.PendingPlayerInputs[index].Attachments = attachmentDescriptors(snapshot.PendingPlayerInputs[index].Attachments)
	}
	if snapshot.CurrentTurn != nil {
		current := *snapshot.CurrentTurn
		current.Attachments = attachmentDescriptors(current.Attachments)
		snapshot.CurrentTurn = &current
	}
}

func materializeChatAttachments(stateRoot string, scope agentattachment.Scope, commandID string, request *chatagent.ChatRequest) error {
	files, err := agentattachment.Materialize(stateRoot, scope, commandID, request.AttachmentUploads)
	if err != nil {
		return fmt.Errorf("materialize chat attachments: %w", err)
	}
	request.AttachmentUploads = nil
	request.AttachedFiles = append([]agent.Attachment(nil), files...)
	request.AttachmentIDs = make([]string, 0, len(files))
	for _, file := range files {
		request.AttachmentIDs = append(request.AttachmentIDs, file.ID)
	}
	return nil
}
