package platform

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strconv"
)

// StoryHost adapts the existing interactive Story service. Callers are bound
// to one authorized Story; implementations must fence Project transitions and
// use that Story's canonical journal for all extension recovery facts.
type StoryHost interface {
	Instances(context.Context) ([]Instance, error)
	Bind(context.Context, Instance, StoryBindingOptions) (Instance, error)
	SaveBinding(context.Context, Instance) error
	RemoveBinding(context.Context, Instance) error
	Export(context.Context, Instance, io.Writer) error
	StopRuntime(context.Context, Scope) error
	Snapshot(context.Context, Scope) (StorySnapshot, error)
	// Stream sends provisional player prose only; completion must be reloaded from Snapshot.
	Stream(context.Context, Scope, string, func(StoryStreamEvent) error) error
	History(context.Context, Scope, string, int) (StoryHistory, error)
	Command(context.Context, Scope, StoryCommand) (StorySnapshot, error)
	ReadRecord(context.Context, Scope, string, StoryRecordRequest) (StoryRecord, error)
	WriteRecord(context.Context, Scope, string, StoryRecordRequest) (StoryRecord, error)
}

// StoryStreamEvent is display-only and never a recovery fact. Reset retracts
// provisional prose; the canonical Story snapshot decides what was committed.
type StoryStreamEvent struct {
	Kind string `json:"kind"`
	Text string `json:"text,omitempty"`
}

func (m *Manager) ConfigureStories(host StoryHost) { m.stories = host }

type StoryBindingOptions struct {
	Origin       string
	ModelProfile string
}

// StoryTurn contains player-visible prose only. Model messages, private plans,
// tool arguments, provider state and host filesystem paths never cross this API.
type StoryTurn struct {
	ID        string   `json:"id"`
	Revision  string   `json:"revision"`
	User      string   `json:"user"`
	Narrative string   `json:"narrative"`
	Choices   []string `json:"choices"`
	Versions  []string `json:"versions"`
}

type StoryBranch struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Current bool   `json:"current"`
}

type StorySnapshot struct {
	StoryID        string        `json:"storyId"`
	BranchID       string        `json:"branchId"`
	Title          string        `json:"title"`
	Turns          []StoryTurn   `json:"turns"`
	Branches       []StoryBranch `json:"branches"`
	BeforeCursor   string        `json:"beforeCursor,omitempty"`
	HasMore        bool          `json:"hasMore"`
	Status         string        `json:"status"`
	OperationID    string        `json:"operationId,omitempty"`
	InterruptionID string        `json:"interruptionId,omitempty"`
}

type StoryHistory struct {
	Turns        []StoryTurn `json:"turns"`
	BeforeCursor string      `json:"beforeCursor,omitempty"`
	HasMore      bool        `json:"hasMore"`
}

// StoryCommand is a closed set of existing Story operations. commandId makes
// model advancement replay-safe; stop addresses the exact observed operation.
type StoryCommand struct {
	Kind           StoryCommandKind `json:"kind"`
	CommandID      string           `json:"commandId"`
	Message        string           `json:"message,omitempty"`
	Locale         string           `json:"locale,omitempty"`
	OperationID    string           `json:"operationId,omitempty"`
	InterruptionID string           `json:"interruptionId,omitempty"`
	BranchID       string           `json:"branchId,omitempty"`
	TurnID         string           `json:"turnId,omitempty"`
	VersionTurnID  string           `json:"versionTurnId,omitempty"`
	Title          string           `json:"title,omitempty"`
}

type StoryCommandKind string

const (
	StoryAdvance       StoryCommandKind = "advance"
	StoryResume        StoryCommandKind = "resume"
	StoryRegenerate    StoryCommandKind = "regenerate"
	StoryStop          StoryCommandKind = "stop"
	StoryFork          StoryCommandKind = "fork"
	StorySwitchBranch  StoryCommandKind = "switchBranch"
	StorySwitchVersion StoryCommandKind = "switchVersion"
)

// StoryRecord is extension-owned JSON. A turn record is addressed by an exact
// prose revision; changed prose cannot silently reuse old presentation data.
type StoryRecord struct {
	Revision      uint64          `json:"revision"`
	SchemaVersion int             `json:"schemaVersion"`
	Value         json.RawMessage `json:"value"`
}

type StoryRecordRequest struct {
	Key              string          `json:"key"`
	BranchID         string          `json:"branchId,omitempty"`
	TurnID           string          `json:"turnId,omitempty"`
	SourceRevision   string          `json:"sourceRevision,omitempty"`
	ExpectedRevision uint64          `json:"expectedRevision"`
	SchemaVersion    int             `json:"schemaVersion"`
	Value            json.RawMessage `json:"value"`
}

func (r *Runtime) serveStory(w http.ResponseWriter, request *http.Request, caller *activation, route string) {
	permission := "stories.read"
	if request.Method != http.MethodGet {
		permission = "stories.write"
	}
	if !slices.Contains(caller.grants, permission) || caller.context.Scope.StoryID == "" || caller.context.Scope.Kind != "game-instance" || caller.context.Scope.InstanceID == "" {
		writeError(w, failure("PERMISSION_DENIED", "A bound Story and %s permission are required", permission))
		return
	}
	if r.manager.stories == nil {
		writeError(w, failure("UNSUPPORTED", "Story host is unavailable"))
		return
	}
	scope := caller.context.Scope
	// Binding authorizes the Story. Branch commands may choose its branches,
	// while no request can select a different Project or Story.
	scope.BranchID = ""
	var result any
	var err error
	switch {
	case route == "/story/events" && request.Method == http.MethodGet:
		r.serveStoryStream(w, request, scope)
		return
	case route == "/story" && request.Method == http.MethodGet:
		result, err = r.manager.stories.Snapshot(request.Context(), scope)
	case route == "/story/history" && request.Method == http.MethodGet:
		scope.BranchID = request.URL.Query().Get("branchId")
		limit, _ := strconv.Atoi(request.URL.Query().Get("limit"))
		result, err = r.manager.stories.History(request.Context(), scope, request.URL.Query().Get("beforeCursor"), limit)
	case route == "/story/commands" && request.Method == http.MethodPost:
		var input StoryCommand
		if err = readRequest(request, &input); err == nil {
			result, err = r.manager.stories.Command(r.ctx, scope, input)
		}
	case route == "/story/records" && request.Method == http.MethodGet:
		q := request.URL.Query()
		input := StoryRecordRequest{Key: q.Get("key"), BranchID: q.Get("branchId"), TurnID: q.Get("turnId"), SourceRevision: q.Get("sourceRevision")}
		result, err = r.manager.stories.ReadRecord(request.Context(), scope, string(caller.release.Ref.Package.Kind)+"/"+caller.release.Manifest.ID, input)
	case route == "/story/records" && request.Method == http.MethodPut:
		var input StoryRecordRequest
		if err = readRequest(request, &input); err == nil {
			result, err = r.manager.stories.WriteRecord(request.Context(), scope, string(caller.release.Ref.Package.Kind)+"/"+caller.release.Manifest.ID, input)
		}
	default:
		err = failure("NOT_FOUND", "Story route is unavailable")
	}
	if err != nil {
		writeError(w, err)
		return
	}
	writeResponse(w, http.StatusOK, result)
}
