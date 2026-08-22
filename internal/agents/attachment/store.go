// Package attachment persists user-supplied chat files as application-owned
// copies. It deliberately performs no content parsing: models receive the copy
// path, while protocol adapters additionally send images as native input.
package attachment

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

const (
	MaxFiles       = 10
	MaxFileBytes   = 20 * 1024 * 1024
	MaxTotalBytes  = 50 * 1024 * 1024
	maxFilenameLen = 240
)

// Upload is the transport-only file payload. It is discarded after the copy is
// persisted and never enters Agent history or idempotency metadata.
type Upload struct {
	Name      string `json:"name"`
	MediaType string `json:"media_type,omitempty"`
	DataURL   string `json:"data_url"`
}

// Scope binds copies to the user-owned conversation that can reference them.
type Scope struct {
	Kind string
	ID   string
}

func SessionScope(sessionID string) Scope { return Scope{Kind: "session", ID: sessionID} }
func StoryScope(storyID string) Scope     { return Scope{Kind: "story", ID: storyID} }

// Materialize writes deterministic per-command copies. A conflicting retry
// cannot overwrite files referenced by the already accepted command.
func Materialize(stateRoot string, scope Scope, commandID string, uploads []Upload) ([]agent.Attachment, error) {
	stateRoot = strings.TrimSpace(stateRoot)
	scope.Kind = strings.TrimSpace(scope.Kind)
	scope.ID = strings.TrimSpace(scope.ID)
	commandID = strings.TrimSpace(commandID)
	if len(uploads) == 0 {
		return nil, nil
	}
	if stateRoot == "" || scope.Kind == "" || scope.ID == "" || commandID == "" {
		return nil, errors.New("attachment storage requires state root, scope, and command id")
	}
	if len(uploads) > MaxFiles {
		return nil, fmt.Errorf("too many attachments: %d > %d", len(uploads), MaxFiles)
	}
	type decodedUpload struct {
		attachment agent.Attachment
		data       []byte
	}
	decoded := make([]decodedUpload, 0, len(uploads))
	var total int64
	for index, upload := range uploads {
		attachment, data, err := decodeUpload(upload, commandID, index)
		if err != nil {
			return nil, fmt.Errorf("attachment %d: %w", index+1, err)
		}
		total += int64(len(data))
		if total > MaxTotalBytes {
			return nil, fmt.Errorf("attachments exceed total limit of %d bytes", MaxTotalBytes)
		}
		decoded = append(decoded, decodedUpload{attachment: attachment, data: data})
	}
	dir := filepath.Join(stateRoot, "attachments", "v1", scopeKey(scope), hashText(commandID))
	_, statErr := os.Stat(dir)
	dirCreated := errors.Is(statErr, os.ErrNotExist)
	if statErr != nil && !dirCreated {
		return nil, fmt.Errorf("inspect attachment directory: %w", statErr)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create attachment directory: %w", err)
	}
	var result []agent.Attachment
	if dirCreated {
		defer func() {
			if result == nil {
				_ = os.RemoveAll(dir)
			}
		}()
	}
	result = make([]agent.Attachment, 0, len(decoded))
	for _, upload := range decoded {
		attachment := upload.attachment
		attachment.Path = filepath.Join(dir, attachment.ID+safeExtension(attachment.Name))
		if err := writeCopy(attachment.Path, upload.data); err != nil {
			result = nil
			return nil, fmt.Errorf("persist attachment %q: %w", attachment.Name, err)
		}
		result = append(result, attachment)
	}
	return result, nil
}

// RemoveScope removes every copy owned by a deleted conversation.
func RemoveScope(stateRoot string, scope Scope) error {
	if strings.TrimSpace(stateRoot) == "" || strings.TrimSpace(scope.Kind) == "" || strings.TrimSpace(scope.ID) == "" {
		return nil
	}
	return os.RemoveAll(filepath.Join(stateRoot, "attachments", "v1", scopeKey(scope)))
}

func decodeUpload(upload Upload, commandID string, index int) (agent.Attachment, []byte, error) {
	name := strings.TrimSpace(filepath.Base(strings.ReplaceAll(upload.Name, "\\", "/")))
	if name == "" || name == "." || name == ".." || len([]byte(name)) > maxFilenameLen {
		return agent.Attachment{}, nil, errors.New("invalid attachment filename")
	}
	header, encoded, ok := strings.Cut(strings.TrimSpace(upload.DataURL), ",")
	if !ok || !strings.HasPrefix(strings.ToLower(header), "data:") || !strings.HasSuffix(strings.ToLower(strings.TrimSpace(header)), ";base64") {
		return agent.Attachment{}, nil, errors.New("attachment data must be a base64 data URL")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return agent.Attachment{}, nil, errors.New("invalid base64 attachment data")
	}
	if len(data) == 0 {
		return agent.Attachment{}, nil, errors.New("attachment is empty")
	}
	if len(data) > MaxFileBytes {
		return agent.Attachment{}, nil, fmt.Errorf("attachment exceeds limit of %d bytes", MaxFileBytes)
	}
	mediaType := strings.TrimSpace(upload.MediaType)
	if parsed, _, err := mime.ParseMediaType(strings.TrimSuffix(strings.TrimPrefix(header, "data:"), ";base64")); err == nil && parsed != "" {
		mediaType = parsed
	}
	detectedType := http.DetectContentType(data)
	if agent.IsNativeImageMediaType(mediaType) {
		// Native image inputs must reflect the bytes the provider will receive;
		// never trust a transport MIME claim for arbitrary content.
		mediaType = detectedType
	} else if mediaType == "" || mediaType == "application/octet-stream" {
		mediaType = detectedType
	}
	digest := sha256.New()
	_, _ = digest.Write([]byte(commandID))
	_, _ = digest.Write([]byte{0, byte(index)})
	_, _ = digest.Write([]byte(name))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write(data)
	id := "att_" + hex.EncodeToString(digest.Sum(nil))[:32]
	return agent.Attachment{ID: id, Name: name, MediaType: mediaType, Size: int64(len(data))}, data, nil
}

func writeCopy(path string, data []byte) error {
	if _, err := os.Stat(path); err == nil {
		// The content-derived filename already proves this is the same original
		// upload. Preserve the working copy if a previous run edited it; an HTTP
		// retry must never overwrite user- or model-authored changes.
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".attachment-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func scopeKey(scope Scope) string {
	return scope.Kind + "_" + hashText(scope.ID)
}

func hashText(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])[:32]
}

func safeExtension(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	if len(ext) > 12 {
		return ""
	}
	for _, char := range ext {
		if char == '.' || char >= 'a' && char <= 'z' || char >= '0' && char <= '9' {
			continue
		}
		return ""
	}
	return ext
}
