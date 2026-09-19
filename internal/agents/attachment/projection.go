package attachment

import (
	"fmt"
	"path/filepath"
	"strings"

	"denova/internal/portablepath"
	agent "github.com/alfredxw/denova/agent"
)

// ProjectFiles resolves canonical copies only within their owning conversation.
// The returned paths are for the current host and are never serialized.
func ProjectFiles(stateRoot string, scope Scope, files []agent.Attachment) ([]agent.Attachment, error) {
	projected := append([]agent.Attachment(nil), files...)
	prefix := "attachments/v1/" + scopeKey(scope) + "/"
	for index := range projected {
		file := &projected[index]
		if err := portablepath.Validate(file.Path); err != nil {
			return nil, err
		}
		if stateRoot == "" || !strings.HasPrefix(file.Path, prefix) {
			return nil, fmt.Errorf("attachment %q is outside its conversation", file.Name)
		}
		file.RuntimePath = filepath.Join(stateRoot, filepath.FromSlash(file.Path))
	}
	return projected, nil
}
