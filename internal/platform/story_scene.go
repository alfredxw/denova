package platform

import (
	"net/http"
	"strings"
)

// StoryScene is an inspection of one recorded native turn version. Its state
// and previous turn belong to that version's ancestry, not the current head.
type StoryScene struct {
	Turn         StoryTurn  `json:"turn"`
	PreviousTurn *StoryTurn `json:"previousTurn,omitempty"`
	State        StoryState `json:"state"`
}

func (r *Runtime) serveStoryScene(w http.ResponseWriter, request *http.Request, caller *activation, scope Scope) {
	if caller != r.owner || r.hostOnly {
		writeError(w, failure("PERMISSION_DENIED", "Scene inspection requires the owning Story view"))
		return
	}
	query := request.URL.Query()
	for _, field := range []string{"branchId", "turnId", "sourceRevision"} {
		value := query.Get(field)
		if value == "" || len(value) > 256 || strings.TrimSpace(value) != value || len(query[field]) != 1 {
			writeError(w, failure("INVALID_ARGUMENT", "Scene inspection requires one exact %s", field))
			return
		}
	}
	scope.BranchID = query.Get("branchId")
	scene, err := r.manager.stories.Scene(request.Context(), scope, query.Get("turnId"), query.Get("sourceRevision"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeResponse(w, http.StatusOK, scene)
}
