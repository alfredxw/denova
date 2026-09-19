package external

import (
	"context"
	"encoding/json"
	"fmt"

	"denova/internal/agents/attachment"
	"denova/internal/agents/session"
	agent "github.com/alfredxw/denova/agent"
)

func projectToolImages(ctx context.Context, sess *session.Session, images []agent.Attachment) ([]agent.Attachment, error) {
	result := append([]agent.Attachment(nil), images...)
	if len(result) == 0 {
		return result, nil
	}
	resolver, ok := sess.ToolArtifactStore().(agent.ToolArtifactPathResolver)
	if !ok {
		return nil, fmt.Errorf("external tool images require the product artifact resolver")
	}
	for index := range result {
		path, err := resolver.ResolveToolArtifactPath(ctx, result[index].Path)
		if err != nil {
			return nil, err
		}
		result[index].RuntimePath = path
	}
	return result, nil
}

// Media paths are projected after checkpointing, so hashes and summaries use
// portable references. Binary data remains in the original immutable stores.
func (operation *Operation) projectMedia(ctx context.Context, input Input) (Input, error) {
	project := func(files []agent.Attachment) ([]agent.Attachment, error) {
		if len(files) == 0 {
			return nil, nil
		}
		return attachment.ProjectFiles(operation.request.AttachmentRoot, attachment.SessionScope(operation.request.Session.ID), files)
	}
	var err error
	input.Attachments, err = project(input.Attachments)
	if err != nil {
		return Input{}, err
	}
	input.Text = agent.ModelUserContent(&agent.Message{Content: input.Text, Attachments: input.Attachments})
	for index := range input.History {
		message := &input.History[index]
		message.Attachments, err = project(message.Attachments)
		if err != nil {
			return Input{}, err
		}
		message.Text = agent.ModelUserContent(&agent.Message{Content: message.Text, Attachments: message.Attachments})
		message.ToolImages, err = projectToolImages(ctx, operation.request.Session, message.ToolImages)
		if err != nil {
			return Input{}, err
		}
	}
	if limit := operation.request.ProviderInputMaxBytes; limit > 0 {
		body, err := json.Marshal(input)
		if err != nil {
			return Input{}, err
		}
		bytes := int64(len(body))
		count := func(files []agent.Attachment) {
			for _, file := range files {
				if agent.IsNativeImageMediaType(file.MediaType) {
					bytes += (file.Size + 2) / 3 * 4
				}
			}
		}
		count(input.Attachments)
		for _, message := range input.History {
			count(message.Attachments)
			count(message.ToolImages)
		}
		if bytes > int64(limit) {
			return Input{}, fmt.Errorf("external provider input exceeds shared byte budget: %d > %d", bytes, limit)
		}
	}
	return input, nil
}
