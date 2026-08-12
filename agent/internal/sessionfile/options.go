package sessionfile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultCheckpointTailBytes   = 64 << 20
	defaultCheckpointTailRecords = 4096
)

// Options controls rebuildable acceleration sidecars. The canonical record
// stream and its correctness are independent of these thresholds.
type Options struct {
	// CheckpointTailBytes triggers a checkpoint at the next reducer-safe
	// boundary after this many canonical tail bytes. Zero selects 64 MiB.
	CheckpointTailBytes int64
	// CheckpointTailRecords triggers at the next reducer-safe boundary after
	// this many canonical transactions. Zero selects 4096.
	CheckpointTailRecords int64
}

func (options Options) normalized() Options {
	if options.CheckpointTailBytes <= 0 {
		options.CheckpointTailBytes = defaultCheckpointTailBytes
	}
	if options.CheckpointTailRecords <= 0 {
		options.CheckpointTailRecords = defaultCheckpointTailRecords
	}
	return options
}

func New(root string) (*Store, error) {
	return NewWithOptions(root, Options{})
}

func NewWithOptions(root string, options Options) (*Store, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("agent Session file Store root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve agent Session file Store root: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return nil, fmt.Errorf("create agent Session file Store root: %w", err)
	}
	return &Store{root: filepath.Clean(absolute), options: options.normalized()}, nil
}
