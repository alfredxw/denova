package platform

import (
	"encoding/json"
	"net/http"
	"reflect"
	"slices"
	"strings"
)

type createStoryInput struct {
	CommandID string         `json:"commandId" jsonschema:"minLength=1,maxLength=128"`
	Title     string         `json:"title"`
	Setup     map[string]any `json:"setup"`
}

type storyCreationRecord struct {
	Input    createStoryInput `json:"input"`
	Instance *Instance        `json:"instance,omitempty"`
}

// Creation receipts live in the originating Story's canonical journal. An
// interrupted admission remains visible and is never retried as a new creation:
// native Story creation has no transactional cross-Story command contract.
func (r *Runtime) serveCreateStory(w http.ResponseWriter, request *http.Request, caller *activation) {
	scope := caller.context.Scope
	if caller != r.owner || r.hostOnly || !slices.Contains(caller.grants, "stories.write") || scope.Kind != "game-instance" || scope.ProjectID == "" || scope.StoryID == "" || scope.InstanceID == "" || caller.release.Ref.Package.Kind != Game || caller.release.Manifest.Game == nil || caller.release.Manifest.Game.Storage.Kind != "story" {
		writeError(w, failure("PERMISSION_DENIED", "A bound owning Story view is required"))
		return
	}
	if r.manager.stories == nil {
		writeError(w, failure("UNSUPPORTED", "Story host is unavailable"))
		return
	}
	var input createStoryInput
	if err := readRequest(request, &input); err != nil {
		writeError(w, err)
		return
	}
	if input.CommandID == "" || len(input.CommandID) > 128 || strings.TrimSpace(input.CommandID) != input.CommandID {
		writeError(w, failure("INVALID_ARGUMENT", "A stable creation commandId is required"))
		return
	}
	if input.Setup == nil {
		input.Setup = map[string]any{}
	}
	// Serialize admission within this runtime, including simultaneous browser tabs.
	r.dataMu.Lock()
	defer r.dataMu.Unlock()
	// Extension record routes cannot choose this host-only namespace. Creation
	// receipts must not be forgeable or replaceable by a game's ordinary records.
	owner := "denova.platform.creations/" + caller.release.Manifest.ID
	address := StoryRecordRequest{Key: input.CommandID, SchemaVersion: 1}
	previous, err := r.manager.stories.ReadRecord(request.Context(), scope, owner, address)
	if err != nil {
		writeError(w, err)
		return
	}
	if previous.Revision != 0 {
		var receipt storyCreationRecord
		if err := json.Unmarshal(previous.Value, &receipt); err != nil {
			writeError(w, err)
			return
		}
		if !reflect.DeepEqual(receipt.Input, input) {
			writeError(w, failure("IDEMPOTENCY_CONFLICT", "Creation commandId belongs to different input"))
			return
		}
		if receipt.Instance == nil {
			writeError(w, failure("IDEMPOTENCY_CONFLICT", "Creation was interrupted; inspect existing stories before starting another play"))
			return
		}
		writeResponse(w, http.StatusOK, receipt.Instance)
		return
	}
	// Validate the view's chosen setup before admitting a creation receipt.
	if strings.TrimSpace(input.Title) == "" || len(input.Title) > 512 {
		writeError(w, failure("INVALID_ARGUMENT", "Instance title must contain 1..512 bytes"))
		return
	}
	if _, err := r.manager.gameSetup(caller.release, input.Setup); err != nil {
		writeError(w, err)
		return
	}
	receipt := storyCreationRecord{Input: input}
	address.Value, err = json.Marshal(receipt)
	if err != nil {
		writeError(w, err)
		return
	}
	admitted, err := r.manager.stories.WriteRecord(request.Context(), scope, owner, address)
	if err != nil {
		writeError(w, err)
		return
	}
	instance, err := r.manager.CreateInstance(CreateInstance{
		GameID: caller.release.Manifest.ID, ReleaseID: caller.release.Ref.ReleaseID,
		ProjectID: scope.ProjectID, Title: input.Title, Setup: input.Setup,
		Models: r.models,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	receipt.Instance = &instance
	address.ExpectedRevision = admitted.Revision
	address.Value, err = json.Marshal(receipt)
	if err == nil {
		_, err = r.manager.stories.WriteRecord(r.ctx, scope, owner, address)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	writeResponse(w, http.StatusCreated, instance)
}
