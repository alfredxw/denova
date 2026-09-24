package external

import (
	"context"
	"fmt"

	"denova/internal/agents/attachment"
	"denova/internal/agents/session"
	agent "github.com/alfredxw/denova/agent"
)

// MediaProjection binds immutable user copies and tool artifacts to one product.
// Resolve returns a copy with host-local paths; it never changes canonical text
// or portable references. Preparation and live steering share this boundary.
type MediaProjection struct {
	Root      string
	Scope     attachment.Scope
	Artifacts agent.ToolArtifactPathResolver
}

func (media MediaProjection) Resolve(ctx context.Context, input Input) (Input, error) {
	project := func(files []agent.Attachment) ([]agent.Attachment, error) {
		if len(files) == 0 {
			return nil, nil
		}
		return attachment.ProjectFiles(media.Root, media.Scope, files)
	}
	var err error
	input.Attachments, err = project(input.Attachments)
	if err != nil {
		return Input{}, err
	}
	input.History = append([]Message(nil), input.History...)
	for index := range input.History {
		message := &input.History[index]
		message.Attachments, err = project(message.Attachments)
		if err != nil {
			return Input{}, err
		}
		message.ToolImages, err = media.ResolveToolImages(ctx, message.ToolImages)
		if err != nil {
			return Input{}, err
		}
	}
	return input, nil
}

func (media MediaProjection) ResolveToolImages(ctx context.Context, images []agent.Attachment) ([]agent.Attachment, error) {
	result := append([]agent.Attachment(nil), images...)
	if len(result) == 0 {
		return result, nil
	}
	if media.Artifacts == nil {
		return nil, fmt.Errorf("external tool images require the product artifact resolver")
	}
	for index := range result {
		path, err := media.Artifacts.ResolveToolArtifactPath(ctx, result[index].Path)
		if err != nil {
			return nil, err
		}
		result[index].RuntimePath = path
	}
	return result, nil
}

func (media MediaProjection) Prepare(ctx context.Context, input Input, limit int) (Input, error) {
	resolved, err := media.Resolve(ctx, input)
	if err != nil {
		return Input{}, err
	}
	return prepareInput(resolved, limit)
}

func (operation *Operation) media() MediaProjection {
	resolver, _ := operation.request.Session.ToolArtifactStore().(agent.ToolArtifactPathResolver)
	return MediaProjection{Root: operation.request.AttachmentRoot, Scope: attachment.SessionScope(operation.request.Session.ID), Artifacts: resolver}
}

func (operation *Operation) projectMedia(ctx context.Context, input Input) (Input, error) {
	return operation.media().Prepare(ctx, input, operation.request.ProviderInputMaxBytes)
}

func projectToolImages(ctx context.Context, sess *session.Session, images []agent.Attachment) ([]agent.Attachment, error) {
	resolver, _ := sess.ToolArtifactStore().(agent.ToolArtifactPathResolver)
	return (MediaProjection{Artifacts: resolver}).ResolveToolImages(ctx, images)
}
