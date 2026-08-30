package automation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"denova/internal/localfs"
)

const (
	hostEffectObligationVersion = 1
	hostEffectObligationDir     = "host-effects"
	maxHostEffectIDBytes        = 512
	maxHostEffectKindBytes      = 4 << 10
	maxHostEffectPayloadBytes   = 8 << 20
)

var ErrHostEffectConflict = errors.New("automation host effect obligation conflict")

// HostEffectObligation is the application-owned durable admission receipt for
// one Runtime host effect. Payload remains opaque here: the app adapter owns
// interpretation, while this module owns exact idempotency and crash recovery.
type HostEffectObligation struct {
	ID         string          `json:"id"`
	Kind       string          `json:"kind"`
	ProjectID  string          `json:"project_id,omitempty"`
	Workspace  string          `json:"workspace,omitempty"`
	Payload    json.RawMessage `json:"payload"`
	IntentHash string          `json:"intent_hash"`
	AdmittedAt time.Time       `json:"admitted_at"`
}

type hostEffectObligationFile struct {
	Version int                  `json:"version"`
	Effect  HostEffectObligation `json:"effect"`
}

// AdmitHostEffect durably claims one exact EffectID before the Runtime may ack
// its own outbox. Same-ID/same-payload retries are idempotent; a different
// payload is a hard conflict. The per-effect lease works across Store instances
// and processes without serializing unrelated effects.
func (s *Store) AdmitHostEffect(ctx context.Context, effect HostEffectObligation) (HostEffectObligation, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	effect, err := normalizeHostEffectObligation(effect)
	if err != nil {
		return HostEffectObligation{}, err
	}
	// Application host effects are user-scoped obligations even when their
	// payload targets a workspace. Keeping the hot index in DataDir means a
	// closed, moved, or temporarily unavailable workspace cannot hide work that
	// the Runtime has already transferred to the host.
	destination := NewStore(s.userDir, "")
	path, err := destination.hostEffectObligationPath(effect.ID)
	if err != nil {
		return HostEffectObligation{}, err
	}
	return withTaskStoreWriteLease(ctx, path, func() (HostEffectObligation, error) {
		existing, found, readErr := readHostEffectObligation(path)
		if readErr != nil {
			return HostEffectObligation{}, readErr
		}
		if found {
			if existing.IntentHash != effect.IntentHash || existing.Kind != effect.Kind || existing.ProjectID != effect.ProjectID || existing.Workspace != effect.Workspace {
				return HostEffectObligation{}, fmt.Errorf("%w: effect_id=%s", ErrHostEffectConflict, effect.ID)
			}
			return existing, nil
		}
		encoded, encodeErr := json.MarshalIndent(hostEffectObligationFile{Version: hostEffectObligationVersion, Effect: effect}, "", "  ")
		if encodeErr != nil {
			return HostEffectObligation{}, fmt.Errorf("encode host effect obligation %s: %w", effect.ID, encodeErr)
		}
		if writeErr := durableWriteJSON(path, append(encoded, '\n'), 0o644); writeErr != nil {
			return HostEffectObligation{}, fmt.Errorf("persist host effect obligation %s: %w", effect.ID, writeErr)
		}
		return effect, nil
	})
}

// ListHostEffects scans only the hot obligation directories. Obligations are
// independent of task definitions and therefore survive task archival/deletion.
func (s *Store) ListHostEffects() ([]HostEffectObligation, error) {
	result := make([]HostEffectObligation, 0)
	seen := make(map[string]HostEffectObligation)
	for _, location := range []*Store{NewStore(s.userDir, "")} {
		dir, err := location.hostEffectObligationDirectory()
		if err != nil {
			return nil, err
		}
		entries, err := os.ReadDir(dir)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("list host effect obligations %s: %w", dir, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
				continue
			}
			effect, found, readErr := readHostEffectObligation(filepath.Join(dir, entry.Name()))
			if readErr != nil {
				return nil, readErr
			}
			if !found {
				continue
			}
			if existing, duplicate := seen[effect.ID]; duplicate {
				if existing.IntentHash != effect.IntentHash {
					return nil, fmt.Errorf("%w: duplicate effect_id=%s", ErrHostEffectConflict, effect.ID)
				}
				continue
			}
			seen[effect.ID] = effect
			result = append(result, effect)
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].AdmittedAt.Equal(result[j].AdmittedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].AdmittedAt.Before(result[j].AdmittedAt)
	})
	return result, nil
}

// AcknowledgeHostEffect removes an application obligation only after its
// downstream durable action owns the same deterministic EffectID.
func (s *Store) AcknowledgeHostEffect(ctx context.Context, effect HostEffectObligation) error {
	if ctx == nil {
		ctx = context.Background()
	}
	effect, err := normalizeHostEffectObligation(effect)
	if err != nil {
		return err
	}
	destination := NewStore(s.userDir, "")
	path, err := destination.hostEffectObligationPath(effect.ID)
	if err != nil {
		return err
	}
	_, err = withTaskStoreWriteLease(ctx, path, func() (struct{}, error) {
		existing, found, readErr := readHostEffectObligation(path)
		if readErr != nil {
			return struct{}{}, readErr
		}
		if !found {
			return struct{}{}, nil
		}
		if existing.IntentHash != effect.IntentHash || existing.Kind != effect.Kind || existing.ProjectID != effect.ProjectID || existing.Workspace != effect.Workspace {
			return struct{}{}, fmt.Errorf("%w: acknowledge effect_id=%s", ErrHostEffectConflict, effect.ID)
		}
		if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return struct{}{}, fmt.Errorf("remove host effect obligation %s: %w", effect.ID, removeErr)
		}
		if syncErr := localfs.SyncDirectory(filepath.Dir(path)); syncErr != nil {
			return struct{}{}, fmt.Errorf("sync host effect obligation directory: %w", syncErr)
		}
		return struct{}{}, nil
	})
	return err
}

func normalizeHostEffectObligation(effect HostEffectObligation) (HostEffectObligation, error) {
	effect.ID = strings.TrimSpace(effect.ID)
	effect.Kind = strings.TrimSpace(effect.Kind)
	effect.ProjectID = strings.TrimSpace(effect.ProjectID)
	effect.Workspace = canonicalStoreRoot(effect.Workspace)
	if effect.ProjectID != "" {
		effect.Workspace = ""
	}
	if effect.ID == "" || len(effect.ID) > maxHostEffectIDBytes || effect.Kind == "" || len(effect.Kind) > maxHostEffectKindBytes {
		return HostEffectObligation{}, fmt.Errorf("host effect identity is invalid")
	}
	if len(effect.Payload) == 0 || len(effect.Payload) > maxHostEffectPayloadBytes || !json.Valid(effect.Payload) {
		return HostEffectObligation{}, fmt.Errorf("host effect payload is invalid or exceeds %d bytes", maxHostEffectPayloadBytes)
	}
	// MarshalIndent is allowed to rewrite whitespace inside RawMessage values.
	// Canonicalize before hashing so a durable read computes the same intent as
	// the original admission regardless of JSON presentation.
	var canonicalPayload bytes.Buffer
	canonicalPayload.Grow(len(effect.Payload))
	if err := json.Compact(&canonicalPayload, effect.Payload); err != nil {
		return HostEffectObligation{}, fmt.Errorf("canonicalize host effect payload: %w", err)
	}
	effect.Payload = append(json.RawMessage(nil), canonicalPayload.Bytes()...)
	wantHash := hostEffectIntentHash(effect.Kind, firstNonEmpty(effect.ProjectID, effect.Workspace), effect.Payload)
	if effect.IntentHash != "" && effect.IntentHash != wantHash {
		return HostEffectObligation{}, fmt.Errorf("%w: effect_id=%s intent hash mismatch", ErrHostEffectConflict, effect.ID)
	}
	effect.IntentHash = wantHash
	if effect.AdmittedAt.IsZero() {
		effect.AdmittedAt = time.Now().UTC()
	} else {
		effect.AdmittedAt = effect.AdmittedAt.UTC()
	}
	return effect, nil
}

func hostEffectIntentHash(kind, target string, payload json.RawMessage) string {
	hash := sha256.New()
	for _, part := range [][]byte{[]byte(kind), []byte(target), payload} {
		_, _ = hash.Write(part)
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (s *Store) hostEffectObligationDirectory() (string, error) {
	if strings.TrimSpace(s.userDir) == "" {
		return "", fmt.Errorf("user automation directory is required")
	}
	return filepath.Join(s.userDir, "automations", hostEffectObligationDir), nil
}

func (s *Store) hostEffectObligationPath(effectID string) (string, error) {
	dir, err := s.hostEffectObligationDirectory()
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(strings.TrimSpace(effectID)))
	return filepath.Join(dir, hex.EncodeToString(digest[:])+".json"), nil
}

func readHostEffectObligation(path string) (HostEffectObligation, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return HostEffectObligation{}, false, nil
	}
	if err != nil {
		return HostEffectObligation{}, false, fmt.Errorf("read host effect obligation %s: %w", path, err)
	}
	var file hostEffectObligationFile
	if err := json.Unmarshal(data, &file); err != nil {
		return HostEffectObligation{}, false, fmt.Errorf("decode host effect obligation %s: %w", path, err)
	}
	if file.Version != hostEffectObligationVersion {
		return HostEffectObligation{}, false, fmt.Errorf("unsupported host effect obligation version %d in %s", file.Version, path)
	}
	effect, err := normalizeHostEffectObligation(file.Effect)
	if err != nil {
		return HostEffectObligation{}, false, fmt.Errorf("validate host effect obligation %s: %w", path, err)
	}
	return effect, true, nil
}
