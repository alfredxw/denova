// Package statistics stores numerical observations for creator-facing charts.
// These records never participate in Session or Story recovery.
package statistics

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Usage follows the SDK convention: cached input is a subset of prompt tokens,
// and reasoning is already included in output. Nil Usage means unreported.
type Usage struct {
	Prompt int64 `json:"prompt"`
	Cached int64 `json:"cached"`
	Output int64 `json:"output"`
	Total  int64 `json:"total"`
}

type CallKind string

const (
	ModelCall   CallKind = "model_call"
	RuntimeTurn CallKind = "runtime_turn"
)

// Call is one provider boundary observation. An opaque external turn can contain
// several requests; it contributes tokens, never an invented model-call count.
// Model is the configured model, not a claim about provider-side routing.
type Call struct {
	ID         string    `json:"id"`
	At         time.Time `json:"at"`
	Kind       CallKind  `json:"kind"`
	Model      string    `json:"model"`
	Runtime    string    `json:"runtime"`
	Failed     bool      `json:"failed"`
	DurationMS int64     `json:"duration_ms"`
	TTFTMS     *int64    `json:"ttft_ms,omitempty"`
	Usage      *Usage    `json:"usage,omitempty"`
}

// Writing is a saved manuscript snapshot. Delta is absent at a new observation
// boundary (restart, resync, relink, or an unobserved file change).
type Writing struct {
	At    time.Time `json:"at"`
	Words int64     `json:"words"`
	Delta *int64    `json:"delta,omitempty"`
}

// The application holds the data-directory process lock. This mutex keeps
// readers from seeing an in-process partial append, across all Projects.
var storeMu sync.Mutex

func AppendCall(root string, call Call) error {
	if call.Kind != ModelCall && call.Kind != RuntimeTurn {
		return fmt.Errorf("invalid statistics call kind %q", call.Kind)
	}
	return appendRecord(root, "calls", call.At, call)
}

func AppendWriting(root string, observation Writing) error {
	return appendRecord(root, "writing", observation.At, observation)
}

func appendRecord(root, category string, at time.Time, record any) error {
	if root == "" || at.IsZero() {
		return errors.New("statistics require a Project Store and timestamp")
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	storeMu.Lock()
	defer storeMu.Unlock()
	dir := filepath.Join(root, "statistics", category)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(dir, at.UTC().Format("2006-01")+".jsonl"), os.O_CREATE|os.O_RDWR|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	// Preserve a torn tail for diagnosis, but keep it from swallowing the next
	// valid observation. Readers report the gap and continue with valid records.
	info, err := file.Stat()
	if err == nil && info.Size() > 0 {
		var last [1]byte
		_, err = file.ReadAt(last[:], info.Size()-1)
		if err == nil && last[0] != '\n' {
			data = append([]byte{'\n'}, data...)
		}
	}
	if err != nil {
		return errors.Join(err, file.Close())
	}
	_, writeErr := file.Write(append(data, '\n'))
	if writeErr == nil {
		writeErr = file.Sync()
	}
	return errors.Join(writeErr, file.Close())
}

// ReadCalls streams the requested UTC interval without a trace/detail limit.
// Corrupt or unsupported records fail visibly rather than becoming zero usage.
func ReadCalls(root string, from, until time.Time, visit func(Call)) error {
	return readRecords(root, "calls", from, until, func(data []byte) error {
		var call Call
		if err := json.Unmarshal(data, &call); err != nil {
			return err
		}
		if call.Kind != ModelCall && call.Kind != RuntimeTurn {
			return fmt.Errorf("invalid call kind %q", call.Kind)
		}
		if !call.At.Before(from) && call.At.Before(until) {
			visit(call)
		}
		return nil
	})
}

func ReadWriting(root string, from, until time.Time, visit func(Writing)) error {
	return readRecords(root, "writing", from, until, func(data []byte) error {
		var observation Writing
		if err := json.Unmarshal(data, &observation); err != nil {
			return err
		}
		if !observation.At.Before(from) && observation.At.Before(until) {
			visit(observation)
		}
		return nil
	})
}

func readRecords(root, category string, from, until time.Time, visit func([]byte) error) error {
	storeMu.Lock()
	defer storeMu.Unlock()
	month := time.Date(from.UTC().Year(), from.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
	var result error
	for month.Before(until) {
		path := filepath.Join(root, "statistics", category, month.Format("2006-01")+".jsonl")
		if err := readFile(path, visit); err != nil && result == nil {
			result = err
		}
		month = month.AddDate(0, 1, 0)
	}
	return result
}

func readFile(path string, visit func([]byte) error) error {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	var result error
	for scanner.Scan() {
		if err := visit(scanner.Bytes()); err != nil {
			if result == nil {
				result = fmt.Errorf("read statistics %s: %w", filepath.Base(path), err)
			}
		}
	}
	return errors.Join(result, scanner.Err())
}
