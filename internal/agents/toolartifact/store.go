// Package toolartifact provides the workspace-bounded implementation behind
// Agent's provider-neutral ToolArtifactStore contract.
package toolartifact

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	agent "github.com/alfredxw/denova/agent"

	workspacelayout "denova/internal/workspace"
)

// Store publishes immutable artifacts beneath one already-existing boundary.
// Every filesystem operation is resolved through os.Root, so relative paths
// and symlinks cannot escape that boundary.
type Store struct {
	boundaryRoot string
	artifactRoot string
}

// NewWorkspaceStore creates a lazy store at the active Denova data directory
// for one opaque session/story scope. The scope is hashed so user-controlled
// names never become filesystem paths or model-visible metadata.
func NewWorkspaceStore(workspaceRoot, scopeID string) (*Store, error) {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	scopeID = strings.TrimSpace(scopeID)
	if workspaceRoot == "" {
		return nil, errors.New("artifact workspace is required")
	}
	if scopeID == "" {
		return nil, errors.New("artifact scope ID is required")
	}
	absolute, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve artifact workspace: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, fmt.Errorf("canonicalize artifact workspace: %w", err)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return nil, fmt.Errorf("stat artifact workspace: %w", err)
	}
	if !info.IsDir() {
		return nil, errors.New("artifact workspace is not a directory")
	}
	scopeDigest := sha256.Sum256([]byte(scopeID))
	artifactRoot := workspacelayout.Path(canonical, "artifacts", "scope-"+hex.EncodeToString(scopeDigest[:16]))
	return NewBoundedStore(canonical, artifactRoot)
}

// NewBoundedStore creates a lazy store whose artifact directory is a strict
// descendant of boundaryRoot. This constructor is used by canonical session
// storage and by NewWorkspaceStore; callers cannot widen access after creation.
func NewBoundedStore(boundaryRoot, artifactRoot string) (*Store, error) {
	boundaryRoot = strings.TrimSpace(boundaryRoot)
	artifactRoot = strings.TrimSpace(artifactRoot)
	if !filepath.IsAbs(boundaryRoot) || !filepath.IsAbs(artifactRoot) {
		return nil, errors.New("artifact boundary and root must be absolute")
	}
	boundaryRoot = filepath.Clean(boundaryRoot)
	artifactRoot = filepath.Clean(artifactRoot)
	relative, err := filepath.Rel(boundaryRoot, artifactRoot)
	if err != nil || relative == "." || pathEscapesBoundary(relative) {
		return nil, errors.New("tool artifact root is outside its boundary")
	}
	canonicalBoundary, err := filepath.EvalSymlinks(boundaryRoot)
	if err != nil {
		return nil, fmt.Errorf("canonicalize tool artifact boundary: %w", err)
	}
	info, err := os.Stat(canonicalBoundary)
	if err != nil {
		return nil, fmt.Errorf("stat tool artifact boundary: %w", err)
	}
	if !info.IsDir() {
		return nil, errors.New("tool artifact boundary is not a directory")
	}
	return &Store{boundaryRoot: canonicalBoundary, artifactRoot: filepath.ToSlash(relative)}, nil
}

func (store *Store) BeginToolArtifact(ctx context.Context, request agent.ToolArtifactRequest) (agent.ToolArtifactWriter, error) {
	if store == nil || store.boundaryRoot == "" || store.artifactRoot == "" {
		return nil, errors.New("tool artifact store is not configured")
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	contentType := strings.TrimSpace(request.MIMEType)
	if contentType == "" {
		return nil, errors.New("tool artifact content type is required")
	}
	purpose := request.Purpose
	if purpose == "" {
		purpose = agent.ToolArtifactPurposeAttachment
	}
	switch purpose {
	case agent.ToolArtifactPurposeCompleteModelOutput, agent.ToolArtifactPurposeCompleteToolOutput, agent.ToolArtifactPurposeAttachment:
	default:
		return nil, fmt.Errorf("unsupported tool artifact purpose %q", purpose)
	}
	root, err := os.OpenRoot(store.boundaryRoot)
	if err != nil {
		return nil, fmt.Errorf("open tool artifact boundary: %w", err)
	}
	if err := root.MkdirAll(store.artifactRoot, 0o700); err != nil {
		root.Close()
		return nil, fmt.Errorf("create tool artifact directory: %w", err)
	}
	// MkdirAll applies its mode only to directories it creates. Explicitly
	// secure the store-owned leaf so legacy scopes created with wider
	// permissions are repaired without changing the workspace boundary or any
	// unrelated parent directory.
	if err := root.Chmod(store.artifactRoot, 0o700); err != nil {
		root.Close()
		return nil, fmt.Errorf("secure tool artifact directory: %w", err)
	}

	callID := strings.TrimSpace(request.ToolCallID)
	if callID == "" {
		callID = strings.TrimSpace(agent.CurrentToolExecutionID(ctx))
	}
	if callID == "" {
		callID = strings.TrimSpace(agent.ToolCallID(ctx))
	}
	id, err := artifactID(callID, purpose)
	if err != nil {
		root.Close()
		return nil, err
	}
	tempName, file, err := createStagingFile(root, store.artifactRoot)
	if err != nil {
		root.Close()
		return nil, err
	}
	extension := normalizedExtension(request.Extension)
	return &writer{
		root: root, file: file, tempName: tempName,
		finalName: filepath.ToSlash(filepath.Join(store.artifactRoot, id+extension)),
		id:        id, purpose: purpose, contentType: contentType, digest: sha256.New(),
	}, nil
}

// VerifyToolArtifact proves that a tool-provided reference names a complete
// artifact published in this store for the expected execution identity. The
// check intentionally relies on bounded path ownership, immutable call/purpose
// identity, and file size; a content hash is optional diagnostic metadata, not
// part of the runtime recovery contract.
func (store *Store) VerifyToolArtifact(ctx context.Context, reference agent.ToolArtifactRef, expected agent.ToolArtifactRequest) error {
	if store == nil || store.boundaryRoot == "" || store.artifactRoot == "" {
		return errors.New("tool artifact store is not configured")
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	purpose := expected.Purpose
	if purpose == "" {
		purpose = agent.ToolArtifactPurposeAttachment
	}
	if reference.Purpose != purpose || !reference.Complete {
		return errors.New("tool artifact purpose or completeness does not match")
	}
	callID := strings.TrimSpace(expected.ToolCallID)
	if callID == "" {
		callID = strings.TrimSpace(agent.CurrentToolExecutionID(ctx))
	}
	if callID == "" {
		callID = strings.TrimSpace(agent.ToolCallID(ctx))
	}
	wantID, err := artifactID(callID, purpose)
	if err != nil {
		return err
	}
	if reference.ID != wantID {
		return errors.New("tool artifact execution identity does not match")
	}
	readablePath := strings.TrimSpace(reference.ReadablePath)
	absolutePath := filepath.Clean(readablePath)
	if !filepath.IsAbs(absolutePath) {
		absolutePath = filepath.Join(store.boundaryRoot, absolutePath)
	}
	expectedDirectory := filepath.Join(store.boundaryRoot, filepath.FromSlash(store.artifactRoot))
	relative, err := filepath.Rel(expectedDirectory, absolutePath)
	if err != nil || relative == "." || pathEscapesBoundary(relative) || filepath.Dir(relative) != "." ||
		!strings.HasPrefix(filepath.Base(relative), wantID+".") {
		return errors.New("tool artifact path is outside the expected execution scope")
	}
	root, err := os.OpenRoot(store.boundaryRoot)
	if err != nil {
		return fmt.Errorf("open tool artifact boundary: %w", err)
	}
	defer root.Close()
	name := filepath.ToSlash(filepath.Join(store.artifactRoot, relative))
	info, err := root.Lstat(name)
	if err != nil {
		return fmt.Errorf("inspect tool artifact: %w", err)
	}
	if !info.Mode().IsRegular() || reference.EstimatedBytes < 0 ||
		(reference.EstimatedBytes > 0 && info.Size() != reference.EstimatedBytes) {
		return errors.New("tool artifact file metadata does not match")
	}
	return nil
}

type writer struct {
	root        *os.Root
	file        *os.File
	tempName    string
	finalName   string
	id          string
	purpose     agent.ToolArtifactPurpose
	contentType string
	digest      hash.Hash
	byteSize    int64
	terminal    bool
}

func (w *writer) Write(data []byte) (int, error) {
	if w == nil || w.file == nil || w.root == nil || w.terminal {
		return 0, errors.New("tool artifact writer is closed")
	}
	written, err := w.file.Write(data)
	if written > 0 {
		_, _ = w.digest.Write(data[:written])
		w.byteSize += int64(written)
	}
	return written, err
}

func (w *writer) Commit() (agent.ToolArtifactRef, error) {
	if w == nil || w.file == nil || w.root == nil || w.terminal {
		return agent.ToolArtifactRef{}, errors.New("tool artifact writer is closed")
	}
	if err := w.file.Sync(); err != nil {
		_ = w.Abort()
		return agent.ToolArtifactRef{}, fmt.Errorf("sync tool artifact: %w", err)
	}
	if err := w.file.Close(); err != nil {
		w.file = nil
		_ = w.Abort()
		return agent.ToolArtifactRef{}, fmt.Errorf("close tool artifact: %w", err)
	}
	w.file = nil
	if err := w.root.Link(w.tempName, w.finalName); err != nil {
		if !errors.Is(err, os.ErrExist) {
			_ = w.Abort()
			return agent.ToolArtifactRef{}, fmt.Errorf("publish tool artifact: %w", err)
		}
		matches, compareErr := w.matchesPublishedArtifact()
		if compareErr != nil {
			_ = w.Abort()
			return agent.ToolArtifactRef{}, compareErr
		}
		if !matches {
			_ = w.Abort()
			return agent.ToolArtifactRef{}, fmt.Errorf("tool artifact call identity %q already has different content", w.id)
		}
		// A matching file may predate the private at-rest permission policy.
		// Repair it before returning the replayed reference.
		if err := w.root.Chmod(w.finalName, 0o600); err != nil {
			_ = w.Abort()
			return agent.ToolArtifactRef{}, fmt.Errorf("secure replayed tool artifact: %w", err)
		}
		if err := w.root.Remove(w.tempName); err != nil && !errors.Is(err, os.ErrNotExist) {
			_ = w.Abort()
			return agent.ToolArtifactRef{}, fmt.Errorf("remove replay artifact staging file: %w", err)
		}
		w.tempName = ""
		return w.finish(), nil
	}
	if err := w.root.Chmod(w.finalName, 0o600); err != nil {
		_ = w.root.Remove(w.finalName)
		_ = w.Abort()
		return agent.ToolArtifactRef{}, fmt.Errorf("secure tool artifact: %w", err)
	}
	if err := w.root.Remove(w.tempName); err != nil && !errors.Is(err, os.ErrNotExist) {
		_ = w.root.Remove(w.finalName)
		w.terminal = true
		_ = w.closeRoot()
		return agent.ToolArtifactRef{}, fmt.Errorf("remove tool artifact staging file: %w", err)
	}
	w.tempName = ""
	if err := syncArtifactDirectory(w.root, filepath.ToSlash(filepath.Dir(w.finalName))); err != nil {
		_ = w.root.Remove(w.finalName)
		w.terminal = true
		_ = w.closeRoot()
		return agent.ToolArtifactRef{}, fmt.Errorf("sync tool artifact directory: %w", err)
	}
	return w.finish(), nil
}

func (w *writer) Abort() error {
	if w == nil || w.terminal {
		return nil
	}
	w.terminal = true
	var result error
	if w.file != nil {
		result = errors.Join(result, w.file.Close())
		w.file = nil
	}
	if w.root != nil && w.tempName != "" {
		if err := w.root.Remove(w.tempName); err != nil && !errors.Is(err, os.ErrNotExist) {
			result = errors.Join(result, err)
		}
		w.tempName = ""
	}
	result = errors.Join(result, w.closeRoot())
	return result
}

func (w *writer) matchesPublishedArtifact() (bool, error) {
	info, err := w.root.Lstat(w.finalName)
	if err != nil {
		return false, fmt.Errorf("inspect published tool artifact: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() != w.byteSize {
		return false, nil
	}
	file, err := w.root.Open(w.finalName)
	if err != nil {
		return false, fmt.Errorf("open published tool artifact: %w", err)
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return false, fmt.Errorf("hash published tool artifact: %w", err)
	}
	return string(digest.Sum(nil)) == string(w.digest.Sum(nil)), nil
}

func (w *writer) finish() agent.ToolArtifactRef {
	w.terminal = true
	readablePath := filepath.ToSlash(filepath.Join(w.root.Name(), filepath.FromSlash(w.finalName)))
	reference := agent.ToolArtifactRef{
		ID: w.id, Purpose: w.purpose, ReadablePath: readablePath, ContentType: w.contentType,
		EstimatedBytes: w.byteSize, EstimatedTokens: estimatedTokens(w.byteSize), Complete: true,
		SHA256: hex.EncodeToString(w.digest.Sum(nil)),
	}
	_ = w.closeRoot()
	return reference
}

func (w *writer) closeRoot() error {
	if w == nil || w.root == nil {
		return nil
	}
	err := w.root.Close()
	w.root = nil
	return err
}

func createStagingFile(root *os.Root, directory string) (string, *os.File, error) {
	for attempt := 0; attempt < 8; attempt++ {
		id, err := randomID()
		if err != nil {
			return "", nil, err
		}
		name := filepath.ToSlash(filepath.Join(directory, ".pending-"+id))
		file, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
		if err == nil {
			return name, file, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return "", nil, fmt.Errorf("create tool artifact: %w", err)
		}
	}
	return "", nil, errors.New("create tool artifact: staging name collisions exhausted")
}

func artifactID(callID string, purpose agent.ToolArtifactPurpose) (string, error) {
	if callID != "" {
		// Purpose is part of the immutable identity. A tool may publish one
		// auxiliary attachment and one complete model-output stream under the same
		// execution ID; neither may relabel or overwrite the other on replay.
		digest := sha256.Sum256([]byte(callID + "\x00" + string(purpose)))
		return "call-" + hex.EncodeToString(digest[:16]), nil
	}
	id, err := randomID()
	if err != nil {
		return "", fmt.Errorf("generate tool artifact id: %w", err)
	}
	return id, nil
}

func randomID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func normalizedExtension(extension string) string {
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

func syncArtifactDirectory(root *os.Root, directory string) error {
	// Windows does not support syncing directory handles through os.File.Sync.
	// The artifact file itself is synced before publication, so retain the
	// directory durability barrier only on platforms that implement it.
	if runtime.GOOS == "windows" {
		return nil
	}
	handle, err := root.Open(directory)
	if err != nil {
		return err
	}
	defer handle.Close()
	return handle.Sync()
}

func pathEscapesBoundary(relative string) bool {
	return filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func estimatedTokens(byteSize int64) int {
	if byteSize <= 0 {
		return 0
	}
	estimate := byteSize / 4
	if byteSize%4 != 0 {
		estimate++
	}
	maxInt := int64(^uint(0) >> 1)
	if estimate > maxInt {
		return int(maxInt)
	}
	return int(estimate)
}
