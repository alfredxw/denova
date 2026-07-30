package toolartifact

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"denova/internal/workspacepath"
)

// WorkspaceScopeOwner identifies one level in an artifact ownership hierarchy.
// Kind is a stable, code-owned label such as "story" or "branch". ID may be
// user-controlled; it is hashed before it becomes part of a filesystem path.
type WorkspaceScopeOwner struct {
	Kind string
	ID   string
}

// WorkspaceScope is an immutable, validated ownership path beneath the
// workspace artifact directory. Its fields remain private so removal callers
// cannot construct an unvalidated or overly broad filesystem target.
type WorkspaceScope struct {
	namespace string
	owners    []WorkspaceScopeOwner
}

// NewWorkspaceScope creates an owned artifact scope. At least one owner is
// required, which deliberately prevents a caller from removing an entire
// artifact namespace through RemoveWorkspaceScope.
func NewWorkspaceScope(namespace string, owners ...WorkspaceScopeOwner) (WorkspaceScope, error) {
	namespace = strings.TrimSpace(namespace)
	if !validScopeLabel(namespace) {
		return WorkspaceScope{}, fmt.Errorf("invalid artifact scope namespace %q", namespace)
	}
	if len(owners) == 0 {
		return WorkspaceScope{}, errors.New("artifact scope requires at least one owner")
	}
	validated := make([]WorkspaceScopeOwner, len(owners))
	for index, owner := range owners {
		owner.Kind = strings.TrimSpace(owner.Kind)
		owner.ID = strings.TrimSpace(owner.ID)
		if !validScopeLabel(owner.Kind) {
			return WorkspaceScope{}, fmt.Errorf("invalid artifact scope owner kind %q", owner.Kind)
		}
		if owner.ID == "" {
			return WorkspaceScope{}, fmt.Errorf("artifact scope owner %q requires an ID", owner.Kind)
		}
		validated[index] = owner
	}
	return WorkspaceScope{namespace: namespace, owners: validated}, nil
}

// NewWorkspaceScopeStore creates a lazy artifact store for one validated,
// workspace-owned scope.
func NewWorkspaceScopeStore(workspaceRoot string, scope WorkspaceScope) (*Store, error) {
	boundary, artifactRoot, err := resolveWorkspaceScope(workspaceRoot, scope)
	if err != nil {
		return nil, err
	}
	return NewBoundedStore(boundary, artifactRoot)
}

// WorkspaceScopeRoot resolves the absolute directory owned by scope. The
// directory remains lazy and is not created by this function.
func WorkspaceScopeRoot(workspaceRoot string, scope WorkspaceScope) (string, error) {
	_, artifactRoot, err := resolveWorkspaceScope(workspaceRoot, scope)
	return artifactRoot, err
}

// RemoveWorkspaceScope removes exactly one validated owned subtree. It first
// verifies that the target is a real directory and then performs the recursive
// removal through os.Root, preserving the canonical workspace boundary even in
// the presence of hostile names or symlinks.
func RemoveWorkspaceScope(workspaceRoot string, scope WorkspaceScope) error {
	boundary, artifactRoot, err := resolveWorkspaceScope(workspaceRoot, scope)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(boundary, artifactRoot)
	if err != nil || relative == "." || pathEscapesBoundary(relative) {
		return errors.New("artifact scope root is outside its workspace boundary")
	}
	relative = filepath.ToSlash(relative)
	root, err := os.OpenRoot(boundary)
	if err != nil {
		return fmt.Errorf("open artifact workspace: %w", err)
	}
	defer root.Close()
	info, err := root.Lstat(relative)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect artifact scope: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("artifact scope root is not a directory")
	}
	if err := root.RemoveAll(relative); err != nil {
		return fmt.Errorf("remove artifact scope: %w", err)
	}
	if err := syncArtifactDirectory(root, filepath.ToSlash(filepath.Dir(relative))); err != nil {
		return fmt.Errorf("sync artifact scope parent: %w", err)
	}
	return nil
}

func resolveWorkspaceScope(workspaceRoot string, scope WorkspaceScope) (string, string, error) {
	validated, err := NewWorkspaceScope(scope.namespace, scope.owners...)
	if err != nil {
		return "", "", err
	}
	boundary, err := canonicalWorkspaceRoot(workspaceRoot)
	if err != nil {
		return "", "", err
	}
	parts := []string{"artifacts", validated.namespace}
	for _, owner := range validated.owners {
		parts = append(parts, owner.Kind+"-"+scopeOwnerDigest(owner.ID))
	}
	return boundary, workspacepath.Path(boundary, parts...), nil
}

func canonicalWorkspaceRoot(workspaceRoot string) (string, error) {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if workspaceRoot == "" {
		return "", errors.New("artifact workspace is required")
	}
	absolute, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return "", fmt.Errorf("resolve artifact workspace: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("canonicalize artifact workspace: %w", err)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", fmt.Errorf("stat artifact workspace: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("artifact workspace is not a directory")
	}
	return canonical, nil
}

func validScopeLabel(value string) bool {
	if value == "" || len(value) > 32 {
		return false
	}
	for index, character := range value {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' {
			continue
		}
		if character == '-' && index > 0 && index < len(value)-1 {
			continue
		}
		return false
	}
	return true
}

func scopeOwnerDigest(ownerID string) string {
	digest := sha256.Sum256([]byte(ownerID))
	return hex.EncodeToString(digest[:16])
}
