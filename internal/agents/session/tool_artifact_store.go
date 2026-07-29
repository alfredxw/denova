package session

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"os"
	"path/filepath"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

const sessionToolArtifactDirectorySuffix = ".artifacts"

// ToolArtifactStore returns the filesystem adapter bound to this canonical
// session. Artifact lifetime follows the session journal lifetime.
func (s *Session) ToolArtifactStore() agent.ToolArtifactStore {
	if s == nil || strings.TrimSpace(s.filePath) == "" {
		return nil
	}
	return &sessionToolArtifactStore{root: sessionToolArtifactDirectory(s.filePath)}
}

type sessionToolArtifactStore struct{ root string }

func (store *sessionToolArtifactStore) BeginToolArtifact(ctx context.Context, request agent.ToolArtifactRequest) (agent.ToolArtifactWriter, error) {
	if store == nil || !filepath.IsAbs(store.root) {
		return nil, fmt.Errorf("tool artifact directory is invalid")
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	mimeType := strings.TrimSpace(request.MIMEType)
	if mimeType == "" {
		return nil, fmt.Errorf("tool artifact MIME type is required")
	}
	if err := os.MkdirAll(store.root, 0o700); err != nil {
		return nil, fmt.Errorf("create tool artifact directory: %w", err)
	}
	id, err := newToolArtifactID()
	if err != nil {
		return nil, err
	}
	extension := normalizedToolArtifactExtension(request.Extension)
	file, err := os.CreateTemp(store.root, ".pending-*")
	if err != nil {
		return nil, fmt.Errorf("create tool artifact: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return nil, fmt.Errorf("protect tool artifact: %w", err)
	}
	return &sessionToolArtifactWriter{
		file: file, tempPath: file.Name(), finalPath: filepath.Join(store.root, id+extension),
		id: id, mimeType: mimeType, digest: sha256.New(),
	}, nil
}

type sessionToolArtifactWriter struct {
	file      *os.File
	tempPath  string
	finalPath string
	id        string
	mimeType  string
	digest    hash.Hash
	byteSize  int64
	terminal  bool
}

func (writer *sessionToolArtifactWriter) Write(data []byte) (int, error) {
	if writer == nil || writer.file == nil || writer.terminal {
		return 0, fmt.Errorf("tool artifact writer is closed")
	}
	written, err := writer.file.Write(data)
	if written > 0 {
		_, _ = writer.digest.Write(data[:written])
		writer.byteSize += int64(written)
	}
	return written, err
}

func (writer *sessionToolArtifactWriter) Commit() (agent.ToolArtifactRef, error) {
	if writer == nil || writer.file == nil || writer.terminal {
		return agent.ToolArtifactRef{}, fmt.Errorf("tool artifact writer is closed")
	}
	if err := writer.file.Sync(); err != nil {
		_ = writer.Abort()
		return agent.ToolArtifactRef{}, fmt.Errorf("sync tool artifact: %w", err)
	}
	if err := writer.file.Close(); err != nil {
		writer.file = nil
		_ = writer.Abort()
		return agent.ToolArtifactRef{}, fmt.Errorf("close tool artifact: %w", err)
	}
	writer.file = nil
	if err := os.Rename(writer.tempPath, writer.finalPath); err != nil {
		_ = writer.Abort()
		return agent.ToolArtifactRef{}, fmt.Errorf("publish tool artifact: %w", err)
	}
	if err := syncParentDirectory(writer.finalPath); err != nil {
		_ = os.Remove(writer.finalPath)
		writer.terminal = true
		return agent.ToolArtifactRef{}, fmt.Errorf("sync tool artifact directory: %w", err)
	}
	writer.terminal = true
	return agent.ToolArtifactRef{
		ID: writer.id, URI: filepath.ToSlash(writer.finalPath), MIMEType: writer.mimeType,
		ByteSize: writer.byteSize, SHA256: hex.EncodeToString(writer.digest.Sum(nil)),
	}, nil
}

func (writer *sessionToolArtifactWriter) Abort() error {
	if writer == nil || writer.terminal {
		return nil
	}
	writer.terminal = true
	var result error
	if writer.file != nil {
		result = errors.Join(result, writer.file.Close())
		writer.file = nil
	}
	if writer.tempPath != "" {
		if err := os.Remove(writer.tempPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			result = errors.Join(result, err)
		}
	}
	return result
}

func sessionToolArtifactDirectory(journalPath string) string {
	return filepath.Clean(journalPath) + sessionToolArtifactDirectorySuffix
}

func normalizedToolArtifactExtension(extension string) string {
	extension = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(extension, ".")))
	if extension == "" || len(extension) > 16 {
		return ".bin"
	}
	for _, value := range extension {
		if (value < 'a' || value > 'z') && (value < '0' || value > '9') {
			return ".bin"
		}
	}
	return "." + extension
}

func newToolArtifactID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate tool artifact id: %w", err)
	}
	return hex.EncodeToString(value[:]), nil
}
