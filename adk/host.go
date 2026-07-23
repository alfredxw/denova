package adk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// RunIdentity is the stable correlation key shared by a host, stream, and run.
type RunIdentity struct {
	RequestID string `json:"request_id"`
	RunID     string `json:"run_id"`
	SessionID string `json:"session_id"`
}

// Validate rejects partial identities that cannot be correlated reliably.
func (identity RunIdentity) Validate() error {
	if strings.TrimSpace(identity.RequestID) == "" {
		return errors.New("host identity: request ID is required")
	}
	if strings.TrimSpace(identity.RunID) == "" {
		return errors.New("host identity: run ID is required")
	}
	if strings.TrimSpace(identity.SessionID) == "" {
		return errors.New("host identity: session ID is required")
	}
	return nil
}

// ModelVisibleSummary is the only HostContext field intended for model input.
// Source, purpose, and an explicit byte ceiling make injection auditable.
type ModelVisibleSummary struct {
	Source   string `json:"source"`
	Purpose  string `json:"purpose"`
	Content  string `json:"content"`
	MaxBytes int    `json:"max_bytes"`
}

// Validate enforces the caller-selected hard byte ceiling.
func (summary ModelVisibleSummary) Validate() error {
	if strings.TrimSpace(summary.Source) == "" {
		return errors.New("model-visible summary: source is required")
	}
	if strings.TrimSpace(summary.Purpose) == "" {
		return errors.New("model-visible summary: purpose is required")
	}
	if summary.MaxBytes <= 0 {
		return errors.New("model-visible summary: MaxBytes must be positive")
	}
	if len(summary.Content) > summary.MaxBytes {
		return fmt.Errorf("model-visible summary: %d bytes exceeds limit %d", len(summary.Content), summary.MaxBytes)
	}
	return nil
}

// HostDetails contains recovery/debug data that must not be injected into a
// model merely because it accompanies a summary.
type HostDetails struct {
	Source string          `json:"source"`
	Data   json.RawMessage `json:"data"`
}

// HostContext keeps bounded model input separate from unrestricted host data.
type HostContext struct {
	Summary ModelVisibleSummary `json:"summary"`
	Details []HostDetails       `json:"details,omitempty"`
}

// Validate checks the model-visible portion without interpreting host details.
func (hostContext HostContext) Validate() error {
	return hostContext.Summary.Validate()
}

// HostContextRequest describes an explicitly bounded context lookup.
type HostContextRequest struct {
	Identity        RunIdentity
	Sources         []string
	SummaryMaxBytes int
}

// Validate rejects context lookups without a stable identity or byte ceiling.
func (request HostContextRequest) Validate() error {
	if err := request.Identity.Validate(); err != nil {
		return err
	}
	if request.SummaryMaxBytes <= 0 {
		return errors.New("host context request: SummaryMaxBytes must be positive")
	}
	return nil
}

// HostEvent is one source-ordered event returned by an external Host.
type HostEvent struct {
	Identity RunIdentity
	Sequence uint64
	Event    *AgentEvent
	Details  []HostDetails
}

// StartRequest starts one externally owned Agent task. TaskMaxBytes is an
// explicit hard ceiling for the user-authored task injected into that host.
type StartRequest struct {
	Identity     RunIdentity
	Task         string
	TaskMaxBytes int
	Context      *HostContext
}

// Validate enforces stable correlation and bounded host input.
func (request StartRequest) Validate() error {
	if err := request.Identity.Validate(); err != nil {
		return err
	}
	if request.TaskMaxBytes <= 0 {
		return errors.New("host start request: TaskMaxBytes must be positive")
	}
	if strings.TrimSpace(request.Task) == "" {
		return errors.New("host start request: task is required")
	}
	if len(request.Task) > request.TaskMaxBytes {
		return fmt.Errorf("host start request: task is %d bytes, exceeds limit %d", len(request.Task), request.TaskMaxBytes)
	}
	if request.Context != nil {
		if err := request.Context.Validate(); err != nil {
			return fmt.Errorf("host start request context: %w", err)
		}
	}
	return nil
}

// ResumeRequest asks a host to restore its own durable domain state.
type ResumeRequest struct {
	Identity RunIdentity
	Token    json.RawMessage
}

// Validate rejects resume requests that cannot be correlated to a run.
func (request ResumeRequest) Validate() error {
	return request.Identity.Validate()
}

// CancelRequest routes a cancellation command to a host-owned active run.
type CancelRequest struct {
	Identity RunIdentity
	Mode     CancelMode
}

// Validate rejects cancellation requests that cannot be correlated to a run
// or do not name an exact supported cancellation state. CancelImmediate is the
// zero value; every non-zero value is an explicitly selected safe-point set.
func (request CancelRequest) Validate() error {
	if err := request.Identity.Validate(); err != nil {
		return err
	}
	switch request.Mode {
	case CancelImmediate,
		CancelAfterChatModel,
		CancelAfterToolCalls,
		CancelAfterChatModel | CancelAfterToolCalls:
		return nil
	default:
		return fmt.Errorf("host cancel request: unsupported mode %d", request.Mode)
	}
}

// Host owns the lifecycle of a Codex-, Claude-, or application-specific
// external Agent. Start and Resume return source-ordered event streams.
type Host interface {
	Start(ctx context.Context, request StartRequest) (*AsyncIterator[*HostEvent], error)
	Resume(ctx context.Context, request ResumeRequest) (*AsyncIterator[*HostEvent], error)
	Cancel(ctx context.Context, request CancelRequest) error
	Context(ctx context.Context, request HostContextRequest) (*HostContext, error)
}

// ResolveHostContext is the ADK-owned boundary for bounded model context. The
// caller's SummaryMaxBytes is authoritative: a Host cannot widen that ceiling
// by returning a different ModelVisibleSummary.MaxBytes value.
func ResolveHostContext(ctx context.Context, host Host, request HostContextRequest) (*HostContext, error) {
	if host == nil {
		return nil, errors.New("resolve host context: nil host")
	}
	if err := request.Validate(); err != nil {
		return nil, fmt.Errorf("resolve host context request: %w", err)
	}
	hostContext, err := host.Context(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("resolve host context: %w", err)
	}
	if hostContext == nil {
		return nil, errors.New("resolve host context: host returned nil context")
	}

	bounded := *hostContext
	bounded.Summary.MaxBytes = request.SummaryMaxBytes
	if err := bounded.Validate(); err != nil {
		return nil, fmt.Errorf("resolve host context response: %w", err)
	}
	return &bounded, nil
}

// HostRegistry resolves uniquely named host implementations.
type HostRegistry struct {
	mu    sync.RWMutex
	hosts map[string]Host
}

// NewHostRegistry registers an optional initial host set.
func NewHostRegistry(hosts map[string]Host) (*HostRegistry, error) {
	registry := &HostRegistry{hosts: make(map[string]Host, len(hosts))}
	for name, host := range hosts {
		if err := registry.Register(name, host); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

// Register adds a uniquely named host.
func (registry *HostRegistry) Register(name string, host Host) error {
	if registry == nil {
		return errors.New("register host: nil registry")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("register host: name is required")
	}
	if host == nil {
		return fmt.Errorf("register host %q: nil host", name)
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if registry.hosts == nil {
		registry.hosts = make(map[string]Host)
	}
	if _, exists := registry.hosts[name]; exists {
		return fmt.Errorf("register host %q: duplicate name", name)
	}
	registry.hosts[name] = host
	return nil
}

// Lookup returns a registered host.
func (registry *HostRegistry) Lookup(name string) (Host, bool) {
	if registry == nil {
		return nil, false
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	host, exists := registry.hosts[name]
	return host, exists
}
