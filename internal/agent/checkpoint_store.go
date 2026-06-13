package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const defaultCheckpointDirectory = ".nova/checkpoints"

type checkpointRecord struct {
	Key       string    `json:"key"`
	UpdatedAt time.Time `json:"updated_at"`
	Value     []byte    `json:"value"`
}

// fileCheckpointStore persists Eino checkpoints under the workspace so Agent
// loops can survive backend restarts better than an in-memory store.
type fileCheckpointStore struct {
	dir string
	mu  sync.Mutex
}

func newCheckpointStore(workspace, agentKind string) interface {
	Set(context.Context, string, []byte) error
	Get(context.Context, string) ([]byte, bool, error)
} {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return &inMemoryStore{mem: map[string][]byte{}}
	}
	agentKind = sanitizeCheckpointSegment(agentKind)
	if agentKind == "" {
		agentKind = AgentKindUnknown
	}
	return &fileCheckpointStore{
		dir: filepath.Join(workspace, filepath.FromSlash(defaultCheckpointDirectory), agentKind),
	}
}

func (s *fileCheckpointStore) Set(_ context.Context, key string, value []byte) error {
	if s == nil {
		return nil
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("checkpoint key is empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	record := checkpointRecord{
		Key:       key,
		UpdatedAt: time.Now().UTC(),
		Value:     append([]byte(nil), value...),
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return os.WriteFile(s.pathForKey(key), append(data, '\n'), 0o644)
}

func (s *fileCheckpointStore) Get(_ context.Context, key string) ([]byte, bool, error) {
	if s == nil {
		return nil, false, nil
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, false, fmt.Errorf("checkpoint key is empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.pathForKey(key))
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var record checkpointRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, false, err
	}
	return append([]byte(nil), record.Value...), true, nil
}

func (s *fileCheckpointStore) pathForKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(s.dir, hex.EncodeToString(sum[:])+".json")
}

func sanitizeCheckpointSegment(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_' || r == '-':
			b.WriteRune(r)
		}
	}
	return b.String()
}
