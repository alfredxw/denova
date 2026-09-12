package book

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"path/filepath"

	"denova/internal/revisionfile"
)

// ensureCreatorTemplate seeds missing instructions and replaces only the exact
// v0.4.4 default. Customized instructions remain author-owned. The adjacent
// backup preserves original bytes, including Windows line endings, for rollback.
func ensureCreatorTemplate(workspace string) error {
	path := filepath.Join(workspace, CreatorFileName)
	backupPath := path + ".v0.4.4.bak"
	migrated := false
	_, err := revisionfile.Mutate(context.Background(), path, revisionfile.Options{}, func(current revisionfile.Snapshot) ([]byte, error) {
		if !current.Exists {
			return []byte(CreatorTemplate), nil
		}
		canonical := bytes.ReplaceAll(current.Content, []byte("\r\n"), []byte("\n"))
		// A digest identifies the released template without retaining its retired
		// instructions in the executable. Release fixtures live only in testdata.
		if revisionfile.Revision(canonical) != "sha256:f7751f17a30b5819fc0ed2d88345dddb4e3af53ce808ef70617041666f3eaf04" {
			if bytes.Contains(canonical, []byte("#main rule#")) {
				slog.Warn("Preserved customized CREATOR.md with a legacy template marker; review creative instructions manually", "path", path)
			}
			return current.Content, nil
		}
		_, err := revisionfile.Mutate(context.Background(), backupPath, revisionfile.Options{FileMode: 0o600}, func(backup revisionfile.Snapshot) ([]byte, error) {
			if backup.Exists && !bytes.Equal(backup.Content, current.Content) {
				return nil, fmt.Errorf("creator migration backup differs from the original: %s", backupPath)
			}
			return current.Content, nil
		})
		if err != nil {
			return nil, fmt.Errorf("preserve released creator instructions: %w", err)
		}
		migrated = true
		return []byte(CreatorTemplate), nil
	})
	if err != nil {
		return fmt.Errorf("initialize creator instructions: %w", err)
	}
	if migrated {
		slog.Info("Replaced released default creator instructions after preserving the original", "path", path, "backup", backupPath)
	}
	return nil
}
