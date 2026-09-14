package platform

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
)

func (r *Runtime) serveStoryStream(w http.ResponseWriter, request *http.Request, scope Scope) {
	started := false
	err := r.manager.stories.Stream(request.Context(), scope, request.URL.Query().Get("operationId"), func(event StoryStreamEvent) error {
		if !started {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			started = true
		}
		data, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if _, err = fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
			return err
		}
		return http.NewResponseController(w).Flush()
	})
	if err != nil {
		slog.WarnContext(request.Context(), "platform_story_stream_failed", "story", scope.StoryID, "error", err)
		if !started {
			writeError(w, err)
		}
	}
}
