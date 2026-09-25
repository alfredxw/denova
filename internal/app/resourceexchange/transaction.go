package resourceexchange

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"denova/internal/portablepath"
	"denova/internal/project"
	"denova/internal/revisionfile"
	"github.com/google/uuid"
)

// FileTarget persists a logical location; a Project's current host path is
// resolved only while applying or recovering a transaction.
type FileTarget struct {
	ProjectID string `json:"project_id,omitempty"`
	Path      string `json:"path"`
}

type fileChange struct {
	Target   FileTarget `json:"target"`
	Expected string     `json:"expected"`
	After    []byte     `json:"after"`
	Delete   bool       `json:"delete,omitempty"`
	Before   []byte     `json:"before,omitempty"`
	Existed  bool       `json:"existed"`
}

type transaction struct {
	CreatedAt time.Time    `json:"created_at"`
	ID        string       `json:"id"`
	State     string       `json:"state"`
	Changes   []fileChange `json:"changes"`
}

func resolveTarget(root string, registry *project.Registry, target FileTarget) (string, error) {
	if err := portablepath.Validate(target.Path); err != nil {
		return "", err
	}
	base := root
	if target.ProjectID != "" {
		if registry == nil {
			return "", fmt.Errorf("project registry is unavailable")
		}
		_, layout, err := registry.Resolve(target.ProjectID, true)
		if err != nil {
			return "", err
		}
		base = layout.ContentRoot
	}
	// Existing components must be ordinary directories/files. os.OpenRoot is
	// used by source readers; targets also reject symlink/case aliases.
	current := base
	for _, part := range strings.Split(target.Path, "/") {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			break
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("resource target contains a symbolic link")
		}
	}
	if err := portablepath.CheckNoCollision(base, target.Path); err != nil {
		return "", err
	}
	return filepath.Join(base, filepath.FromSlash(target.Path)), nil
}

func saveTransaction(ctx context.Context, root string, txn transaction) error {
	raw, err := json.Marshal(txn)
	if err != nil {
		return err
	}
	_, err = revisionfile.ReplaceIfRevision(ctx, filepath.Join(root, "resource-exchange", "transactions", txn.ID+".json"), "", raw, revisionfile.Options{})
	return err
}

func commitFiles(ctx context.Context, root string, registry *project.Registry, id string, changes []fileChange) error {
	paths := make([]string, len(changes))
	seen := map[string]bool{}
	for i, change := range changes {
		path, err := resolveTarget(root, registry, change.Target)
		if err != nil {
			return err
		}
		if seen[path] {
			return fmt.Errorf("duplicate transaction target")
		}
		seen[path], paths[i] = true, path
	}
	return revisionfile.WithFiles(ctx, paths, func(files *revisionfile.LockedFiles) error {
		for i := range changes {
			before, err := files.Snapshot(paths[i])
			if err != nil {
				return err
			}
			if before.Revision != changes[i].Expected {
				return &revisionfile.ConflictError{Path: paths[i], Expected: changes[i].Expected, Actual: before.Revision}
			}
			changes[i].Before, changes[i].Existed = before.Content, before.Exists
		}
		txn := transaction{ID: id, State: "applying", Changes: changes, CreatedAt: time.Now().UTC()}
		if err := saveTransaction(ctx, root, txn); err != nil {
			return err
		}
		// Once the intent is durable, client disconnect/cancellation cannot leave
		// a partially applied package. Finish or restore under the same locks.
		rollback := func(cause error) error {
			for i := len(changes) - 1; i >= 0; i-- {
				if err := files.Replace(paths[i], changes[i].Before, changes[i].Existed); err != nil {
					return errors.Join(cause, err)
				}
			}
			txn.State = "rolled_back"
			return errors.Join(cause, saveTransaction(context.Background(), root, txn))
		}
		for i, change := range changes {
			if err := files.Replace(paths[i], change.After, !change.Delete); err != nil {
				return rollback(err)
			}
		}
		txn.State = "committed"
		if err := saveTransaction(context.Background(), root, txn); err != nil {
			return rollback(err)
		}
		slog.InfoContext(ctx, "resource_exchange_committed", "operation", id, "files", len(changes))
		return nil
	})
}

// Recover must run before resource readers or runtimes start. Completed records
// retain before-images as backups; interrupted commits restore their old state.
func Recover(root string, registry *project.Registry) error {
	dir := filepath.Join(root, "resource-exchange", "transactions")
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") || entry.IsDir() {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return err
		}
		var txn transaction
		if err := json.Unmarshal(raw, &txn); err != nil {
			return err
		}
		if _, err := uuid.Parse(txn.ID); err != nil || entry.Name() != txn.ID+".json" {
			return fmt.Errorf("invalid exchange transaction identity")
		}
		switch txn.State {
		case "committed", "rolled_back":
			continue
		case "applying":
		default:
			return fmt.Errorf("unknown exchange transaction state")
		}
		paths := make([]string, len(txn.Changes))
		for i, change := range txn.Changes {
			paths[i], err = resolveTarget(root, registry, change.Target)
			if err != nil {
				return err
			}
		}
		err = revisionfile.WithFiles(context.Background(), paths, func(files *revisionfile.LockedFiles) error {
			for i, change := range txn.Changes {
				current, err := files.Snapshot(paths[i])
				if err != nil {
					return err
				}
				after := revisionfile.Revision(change.After)
				if change.Delete {
					after = revisionfile.MissingRevision
				}
				if current.Revision != change.Expected && current.Revision != after {
					return fmt.Errorf("recovery target changed outside interrupted transaction: %s", change.Target.Path)
				}
			}
			for i := len(txn.Changes) - 1; i >= 0; i-- {
				change := txn.Changes[i]
				if err := files.Replace(paths[i], change.Before, change.Existed); err != nil {
					return err
				}
			}
			txn.State = "rolled_back"
			return saveTransaction(context.Background(), root, txn)
		})
		if err != nil {
			return err
		}
		slog.Info("resource_exchange_recovered", "operation", txn.ID)
	}
	return nil
}
