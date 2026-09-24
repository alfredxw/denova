package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	agent "github.com/alfredxw/denova/agent"
)

// Match the product's upload limit. Image bytes have their own bound and never
// consume the text result limit; visual tokens are counted by InputEstimator.
const maxReadImageBytes = 20 << 20

func readLocalImage(ctx context.Context, target resolvedReadPath, file *os.File) (ReadResult, error) {
	if target.info.Size() > maxReadImageBytes {
		return ReadResult{}, fmt.Errorf("image %s exceeds the %d-byte read limit", target.display, maxReadImageBytes)
	}
	store := agent.ToolArtifactStoreFromContext(ctx)
	resolver, ok := store.(agent.ToolArtifactPathResolver)
	if store == nil || !ok {
		return ReadResult{}, errors.New("image reading requires an artifact store with local path resolution")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxReadImageBytes+1))
	if err != nil {
		return ReadResult{}, fmt.Errorf("read image %s: %w", target.display, err)
	}
	if len(data) > maxReadImageBytes {
		return ReadResult{}, fmt.Errorf("image %s exceeds the %d-byte read limit", target.display, maxReadImageBytes)
	}
	dimensions, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || dimensions.Width <= 0 || dimensions.Height <= 0 || !agent.IsNativeImageMediaType("image/"+format) {
		return ReadResult{}, fmt.Errorf("image %s has invalid or unsupported image data", target.display)
	}
	if err := contextError(ctx); err != nil {
		return ReadResult{}, err
	}
	writer, err := store.BeginToolArtifact(ctx, agent.ToolArtifactRequest{
		ToolName: "read", Purpose: agent.ToolArtifactPurposeAttachment,
		MIMEType: "image/" + format, Extension: format, Description: "Immutable image observed by a read tool call",
	})
	if err != nil {
		return ReadResult{}, fmt.Errorf("begin image snapshot: %w", err)
	}
	if _, err := writer.Write(data); err != nil {
		_ = writer.Abort()
		return ReadResult{}, fmt.Errorf("write image snapshot: %w", err)
	}
	if err := contextError(ctx); err != nil {
		_ = writer.Abort()
		return ReadResult{}, err
	}
	reference, err := writer.Commit()
	if err != nil {
		_ = writer.Abort()
		return ReadResult{}, fmt.Errorf("commit image snapshot: %w", err)
	}
	// A compressed image's byte count cannot estimate its visual token cost.
	reference.EstimatedTokens = 0
	runtimePath, err := resolver.ResolveToolArtifactPath(ctx, reference.ReadablePath)
	if err != nil {
		return ReadResult{}, fmt.Errorf("resolve image snapshot: %w", err)
	}
	attachment := agent.Attachment{
		ID: reference.ID, Name: filepath.Base(target.absolute),
		MediaType: reference.ContentType, Size: reference.EstimatedBytes,
		Path: reference.ReadablePath, RuntimePath: runtimePath, SHA256: reference.SHA256,
	}
	slog.InfoContext(ctx, "read image snapshot", "path", target.display, "width", dimensions.Width, "height", dimensions.Height, "bytes", len(data))
	return ReadResult{
		Path: target.display, Kind: "local_image",
		Content:     fmt.Sprintf("Image loaded for visual inspection (%d x %d). Immutable copy: %s", dimensions.Width, dimensions.Height, reference.ReadablePath),
		Attachments: []agent.Attachment{attachment}, Artifacts: []agent.ToolArtifactRef{reference},
	}, nil
}
