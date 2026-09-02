package trajectory

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"
)

const (
	OutcomePositive   = "positive"
	OutcomeNegative   = "negative"
	OutcomeCorrection = "correction"
)

type Outcome struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id,omitempty"`
	SessionID string    `json:"session_id,omitempty"`
	RunID     string    `json:"run_id,omitempty"`
	Signal    string    `json:"signal"`
	Comment   string    `json:"comment,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type OutcomeStore struct {
	path string
	lock *flock.Flock
}

func NewOutcomeStore(stateRoot string) (*OutcomeStore, error) {
	stateRoot = strings.TrimSpace(stateRoot)
	if stateRoot == "" {
		return nil, errors.New("trajectory outcome Project Store directory is required")
	}
	if err := os.MkdirAll(stateRoot, 0o700); err != nil {
		return nil, fmt.Errorf("create trajectory outcome directory: %w", err)
	}
	return &OutcomeStore{
		path: filepath.Join(stateRoot, "outcomes.jsonl"),
		lock: flock.New(filepath.Join(stateRoot, "outcomes.lock")),
	}, nil
}

func (store *OutcomeStore) Append(outcome Outcome) (Outcome, error) {
	if store == nil || store.lock == nil {
		return Outcome{}, errors.New("trajectory outcome store is unavailable")
	}
	outcome.ProjectID = strings.TrimSpace(outcome.ProjectID)
	outcome.SessionID = strings.TrimSpace(outcome.SessionID)
	outcome.RunID = strings.TrimSpace(outcome.RunID)
	outcome.Signal = strings.ToLower(strings.TrimSpace(outcome.Signal))
	outcome.Comment = strings.TrimSpace(outcome.Comment)
	if outcome.RunID == "" && outcome.SessionID == "" {
		return Outcome{}, errors.New("trajectory outcome requires run_id or session_id")
	}
	if len(outcome.ProjectID) > 4096 || len(outcome.SessionID) > 4096 || len(outcome.RunID) > 4096 {
		return Outcome{}, errors.New("trajectory outcome identifier exceeds 4096 bytes")
	}
	switch outcome.Signal {
	case OutcomePositive, OutcomeNegative, OutcomeCorrection:
	default:
		return Outcome{}, fmt.Errorf("unsupported trajectory outcome signal %q", outcome.Signal)
	}
	if len(outcome.Comment) > 16*1024 {
		return Outcome{}, errors.New("trajectory outcome comment exceeds 16 KiB")
	}
	outcome.CreatedAt = time.Now().UTC()
	identity := make([]byte, 16)
	if _, err := rand.Read(identity); err != nil {
		return Outcome{}, fmt.Errorf("create trajectory outcome identity: %w", err)
	}
	outcome.ID = "outcome-" + hex.EncodeToString(identity)
	encoded, err := json.Marshal(outcome)
	if err != nil {
		return Outcome{}, err
	}
	if err := store.lock.Lock(); err != nil {
		return Outcome{}, err
	}
	defer store.lock.Unlock()
	file, err := os.OpenFile(store.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return Outcome{}, err
	}
	if _, err = file.Write(append(encoded, '\n')); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	return outcome, err
}

func (store *OutcomeStore) List(limit int) ([]Outcome, error) {
	if store == nil || store.lock == nil {
		return nil, errors.New("trajectory outcome store is unavailable")
	}
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	if err := store.lock.RLock(); err != nil {
		return nil, err
	}
	defer store.lock.Unlock()
	file, err := os.Open(store.path)
	if errors.Is(err, fs.ErrNotExist) {
		return []Outcome{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	items := make([]Outcome, 0, limit)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 256*1024)
	line := 0
	for scanner.Scan() {
		line++
		if len(strings.TrimSpace(scanner.Text())) == 0 {
			continue
		}
		var outcome Outcome
		if err := json.Unmarshal(scanner.Bytes(), &outcome); err != nil {
			return nil, fmt.Errorf("decode trajectory outcome line %d: %w", line, err)
		}
		items = append(items, outcome)
		if len(items) > limit {
			items = items[len(items)-limit:]
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
		items[left], items[right] = items[right], items[left]
	}
	return items, nil
}
