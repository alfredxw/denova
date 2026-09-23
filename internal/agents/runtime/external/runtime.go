package external

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"denova/config"
	agent "github.com/alfredxw/denova/agent"
)

// ErrSessionUnavailable is valid only before a provider admits new work.
// Reconstruction must never retry a possibly executed tool effect.
var ErrSessionUnavailable = errors.New("runtime session cache is unavailable")

// Application controls preserve pending questions, drafts and tool receipts.
var ErrSuspended = errors.New("external execution suspended")
var ErrSteered = errors.New("external execution redirected")

// Runtime owns disposable provider sessions at the Denova application boundary.
// It neither constructs nor runs a Native Agent. The product controller owns
// admission, tools, controls and canonical acceptance through Host.
type Runtime struct {
	Selection     config.RuntimeSelection
	Acquire       func(context.Context) (Adapter, func(), error)
	CacheRoot     string
	ConnectionKey string
}

// SessionRequest binds a provider cache to an exact canonical product boundary.
// Key contains stable Project and product Session/branch identity, never paths.
// Boundary is the existing journal source cursor or Game branch head/version.
// Input.History is needed only on a cache miss; Input.Text is the new input.
type SessionRequest struct {
	Key      string
	Boundary string
	Input    Input
	// Prepare bounds and projects only the input actually sent to the provider.
	// History is empty on an aligned resume; source summaries run on cache misses.
	Prepare func(context.Context, Input, Adapter) (Input, error)
}

type runtimeCache struct {
	Version         uint16            `json:"version"`
	SessionID       string            `json:"session_id"`
	Boundary        string            `json:"boundary"`
	PrefixKey       string            `json:"prefix_key"`
	CumulativeUsage *agent.TokenUsage `json:"cumulative_usage,omitempty"`
}

type SessionResult struct {
	Result
	Session *RuntimeSession
}

// RuntimeSession owns a provider lease through canonical commit and optional
// read-only Goal evaluation. Accept is called only after the product commits;
// every other exit leaves the cache invalid for safe reconstruction.
type RuntimeSession struct {
	EvaluationUsage func(context.Context, *agent.TokenUsage) error
	adapter         Adapter
	release         func()
	once            sync.Once
	input           Input
	cache           runtimeCache
	file            string
	prepareInput    func(context.Context, Input, Adapter) (Input, error)
	rebuildAllowed  bool
}

func (runtime *Runtime) prepare(ctx context.Context, request SessionRequest) (*RuntimeSession, error) {
	if runtime.Acquire == nil || request.Key == "" {
		return nil, errors.New("runtime requires a connection source and product identity")
	}
	root := runtime.CacheRoot
	if root == "" {
		var err error
		root, err = os.UserCacheDir()
		if err != nil {
			return nil, err
		}
		root = filepath.Join(root, "denova", "runtime-sessions")
	}
	key, err := json.Marshal(struct {
		Key        string
		Selection  config.RuntimeSelection
		Connection string
	}{request.Key, runtime.Selection, runtime.ConnectionKey})
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(key)
	dir := filepath.Join(root, hex.EncodeToString(digest[:]))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	prefix, err := json.Marshal(struct {
		Instructions string
		Tools        []Tool
	}{request.Input.Instructions, request.Input.Tools})
	if err != nil {
		return nil, err
	}
	prefixHash := sha256.Sum256(prefix)
	lease := &RuntimeSession{file: filepath.Join(dir, "binding.json"), input: request.Input, prepareInput: request.Prepare, rebuildAllowed: true,
		cache: runtimeCache{Version: 1, Boundary: request.Boundary, PrefixKey: hex.EncodeToString(prefixHash[:])}}
	lease.input.Selection, lease.input.Directory = runtime.Selection, dir
	data, readErr := os.ReadFile(lease.file)
	var cached runtimeCache
	if readErr == nil && json.Unmarshal(data, &cached) == nil && cached.Version == 1 &&
		cached.Boundary == request.Boundary && cached.PrefixKey == lease.cache.PrefixKey {
		lease.input.SessionID, lease.input.CumulativeUsage = cached.SessionID, cached.CumulativeUsage
	}
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return nil, readErr
	}
	if err := os.Remove(lease.file); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	lease.adapter, lease.release, err = runtime.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	return lease, nil
}

func (runtime *Runtime) Run(ctx context.Context, request SessionRequest, host Host) (SessionResult, error) {
	lease, err := runtime.prepare(ctx, request)
	if err != nil {
		return SessionResult{}, err
	}
	result, err := lease.run(ctx, host)
	return SessionResult{Result: result, Session: lease}, err
}

func (lease *RuntimeSession) run(ctx context.Context, host Host) (Result, error) {
	input := lease.input
	if input.SessionID != "" {
		input.History = nil
	}
	var err error
	if lease.prepareInput != nil {
		input, err = lease.prepareInput(ctx, input, lease.adapter)
		if err != nil {
			return Result{}, err
		}
	}
	result, err := lease.adapter.Run(ctx, input, host)
	if errors.Is(err, ErrSessionUnavailable) && input.SessionID != "" && lease.rebuildAllowed {
		slog.InfoContext(ctx, "Rebuilding runtime cache from canonical conversation")
		input = lease.input
		input.SessionID, input.CumulativeUsage = "", nil
		if lease.prepareInput != nil {
			input, err = lease.prepareInput(ctx, input, lease.adapter)
			if err != nil {
				return Result{}, err
			}
		}
		result, err = lease.adapter.Run(ctx, input, host)
	}
	// A product may deliberately interrupt a settled turn after its terminal
	// tool commits (Game). Keep that handle available; only the product's later
	// Accept call can bind it to canonical history. Cancellation alone never
	// makes an unconfirmed provider continuation reusable.
	if err == nil || result.Settled {
		lease.input.SessionID, lease.cache.SessionID = result.SessionID, result.SessionID
		lease.input.CumulativeUsage, lease.cache.CumulativeUsage = result.CumulativeUsage, result.CumulativeUsage
		if result.Plan != nil {
			lease.input.Plan = result.Plan.Items
		}
		lease.input.History = nil
		lease.rebuildAllowed = false
	}
	return result, err
}

// Continue keeps the same provider session for Game validation or appended input.
// The caller supplies the next tool set explicitly, including none after final
// structured submission. History cannot be replayed into an aligned session.
func (lease *RuntimeSession) Continue(ctx context.Context, input Input, host Host) (Result, error) {
	lease.input.Text, lease.input.Attachments, lease.input.Tools = input.Text, input.Attachments, input.Tools
	return lease.run(ctx, host)
}

func (lease *RuntimeSession) Accept(_ context.Context, boundary string) error {
	if lease.cache.SessionID == "" {
		return nil
	}
	lease.cache.Boundary = boundary
	body, err := json.Marshal(lease.cache)
	if err != nil {
		return err
	}
	return os.WriteFile(lease.file, body, 0600)
}

func (lease *RuntimeSession) Close() error {
	lease.once.Do(lease.release)
	return nil
}

func (lease *RuntimeSession) Evaluate(ctx context.Context, prompt string) (Result, error) {
	if lease.input.SessionID == "" {
		return Result{}, errors.New("runtime cannot fork without a session handle")
	}
	input := lease.input
	input.Mode, input.Text, input.Tools, input.History, input.Attachments = OperationEvaluate, prompt, nil, nil, nil
	result, err := lease.adapter.Run(ctx, input, maintenanceHost{})
	if lease.EvaluationUsage != nil && result.Usage != nil {
		err = errors.Join(err, lease.EvaluationUsage(context.WithoutCancel(ctx), result.Usage))
	}
	return result, err
}

func (runtime *Runtime) Compact(ctx context.Context, request SessionRequest, host Host) (Result, error) {
	request.Input.Mode = OperationCompact
	result, err := runtime.Run(ctx, request, host)
	if result.Session != nil {
		defer result.Session.Close()
	}
	if err != nil {
		return result.Result, err
	}
	return result.Result, result.Session.Accept(ctx, request.Boundary)
}
