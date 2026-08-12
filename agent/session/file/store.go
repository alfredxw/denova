// Package file provides Agent's built-in durable session.Store.
//
// Custom persistence adapters implement session.Store directly. The built-in
// implementation keeps one complete, checksummed canonical record stream per
// Session. Direct use also gets rebuildable runtime checkpoints and command
// indexes; decorators that expose only session.Log remain correct and may fall
// back to linear replay. Custom stores can choose their own acceleration
// without exposing Agent's reducer schema through the public contract.
package file

import (
	sessionfile "github.com/alfredxw/denova/agent/internal/sessionfile"
	"github.com/alfredxw/denova/agent/session"
)

type Store = sessionfile.Store
type Options = sessionfile.Options

func New(root string) (*Store, error) {
	return sessionfile.New(root)
}

func NewWithOptions(root string, options Options) (*Store, error) {
	return sessionfile.NewWithOptions(root, options)
}

var _ session.Store = (*Store)(nil)
