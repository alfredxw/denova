package interactive

import (
	producttools "denova/internal/agents/tools"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"denova/config"
	"denova/internal/agents/session"
)

const (
	directorPlanHiddenNotice = "chapter_body_hidden"
	directorPlanHiddenReason = "director_plan_body"
	directorPlanProgressStep = 100
)

func (c *DirectorConversation) AppendDisplayEvent(event session.DisplayEvent) error {
	if c == nil {
		return nil
	}
	event = decorateDirectorDisplayEvent(event)
	if c.shouldCompactDirectorTool(event.Name) {
		event = c.recordDirectorToolEvent(event)
	}
	return c.forwardDisplayEvent(event)
}

func (c *DirectorConversation) AppendDisplayToolArgs(id, name, delta string) error {
	if c == nil || delta == "" {
		return nil
	}
	if c.shouldCompactDirectorToolArgs(id, name) {
		event, ok := c.recordDirectorToolArgs(id, name, delta)
		if !ok {
			return nil
		}
		return c.forwardDisplayEvent(event)
	}
	if appender, ok := c.display.(displayToolArgsAppender); ok {
		return appender.AppendDisplayToolArgs(id, name, delta)
	}
	return nil
}

func (c *DirectorConversation) AppendDisplayEventContent(id, role, delta string) error {
	if c == nil || delta == "" {
		return nil
	}
	if appender, ok := c.display.(displayEventContentAppender); ok {
		return appender.AppendDisplayEventContent(id, role, delta)
	}
	return nil
}

func (c *DirectorConversation) FlushDisplayEventContent(id, role string) error {
	if c == nil {
		return nil
	}
	if flusher, ok := c.display.(displayEventContentFlusher); ok {
		return flusher.FlushDisplayEventContent(id, role)
	}
	return nil
}

func (c *DirectorConversation) FinalizeDisplayAssistantRun(runID, finalSegmentID, terminalPhase string) error {
	if c == nil {
		return nil
	}
	if finalizer, ok := c.display.(displayAssistantRunFinalizer); ok {
		return finalizer.FinalizeDisplayAssistantRun(runID, finalSegmentID, terminalPhase)
	}
	return nil
}

func (c *DirectorConversation) UpdateDisplayToolStatus(id, name, status string) error {
	if c == nil {
		return nil
	}
	if c.shouldCompactDirectorToolArgs(id, name) {
		if event, ok := c.finishDirectorToolEvent(id, name, status, ""); ok {
			if err := c.forwardDisplayEvent(event); err != nil {
				return err
			}
		}
	}
	if updater, ok := c.display.(displayEventAppender); ok {
		return updater.UpdateDisplayToolStatus(id, name, status)
	}
	return nil
}

func (c *DirectorConversation) UpdateDisplayToolResult(id, name, status, result string) error {
	if c == nil {
		return nil
	}
	if c.shouldCompactDirectorToolArgs(id, name) {
		if event, ok := c.finishDirectorToolEvent(id, name, status, result); ok {
			if err := c.forwardDisplayEvent(event); err != nil {
				return err
			}
		}
	}
	if updater, ok := c.display.(displayToolResultUpdater); ok {
		return updater.UpdateDisplayToolResult(id, name, status, result)
	}
	if updater, ok := c.display.(displayEventAppender); ok {
		return updater.UpdateDisplayToolStatus(id, name, status)
	}
	return nil
}

func (c *DirectorConversation) forwardDisplayEvent(event session.DisplayEvent) error {
	if appender, ok := c.display.(displayEventAppender); ok {
		return appender.AppendDisplayEvent(event)
	}
	return nil
}

func (c *DirectorConversation) recordDirectorToolEvent(event session.DisplayEvent) session.DisplayEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	state := c.directorToolStateLocked(event.ID, event.Name)
	if state == nil {
		return event
	}
	state.event = event
	state.appendArgs(event.Args)
	projected, ok := state.projectEvent(event)
	if !ok {
		projected.Args = ""
		return projected
	}
	state.sentChars = state.generatedChars
	return projected
}

func (c *DirectorConversation) shouldCompactDirectorTool(name string) bool {
	// Director writes are internal control payloads that may contain large
	// Markdown patches. Always compact them independently of chapter display
	// settings, including invalid write/edit calls before policy rejects them.
	return directorPlanWriteTool(name)
}

func (c *DirectorConversation) shouldCompactDirectorToolArgs(id, name string) bool {
	if c.shouldCompactDirectorTool(name) {
		return true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	state := c.findDirectorToolStateLocked(id, name)
	return state != nil && c.shouldCompactDirectorTool(state.name)
}

func (c *DirectorConversation) recordDirectorToolArgs(id, name, delta string) (session.DisplayEvent, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	state := c.directorToolStateLocked(id, name)
	if state == nil {
		return session.DisplayEvent{}, false
	}
	if strings.TrimSpace(state.event.Role) == "" {
		state.event = decorateDirectorDisplayEvent(session.DisplayEvent{
			ID:      strings.TrimSpace(id),
			Role:    "tool_call",
			Content: strings.TrimSpace(state.name),
			Name:    strings.TrimSpace(state.name),
			Status:  "running",
		})
	}
	state.appendArgs(delta)
	event, ok := state.progressEvent()
	if !ok {
		return session.DisplayEvent{}, false
	}
	return event, true
}

func (c *DirectorConversation) finishDirectorToolEvent(id, name, status, result string) (session.DisplayEvent, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	state := c.findDirectorToolStateLocked(id, name)
	if state == nil {
		return session.DisplayEvent{}, false
	}
	state.syncDecodedGeneratedChars()
	event, ok := state.projectEvent(state.event)
	if !ok {
		delete(c.directorTools, directorToolStateKey(state.id, state.name))
		return session.DisplayEvent{}, false
	}
	if strings.TrimSpace(status) != "" {
		event.Status = strings.TrimSpace(status)
	}
	event.Result = result
	delete(c.directorTools, directorToolStateKey(state.id, state.name))
	return event, true
}

func (c *DirectorConversation) directorToolStateLocked(id, name string) *directorToolDisplayState {
	if c.directorTools == nil {
		c.directorTools = map[string]*directorToolDisplayState{}
	}
	if existing := c.findDirectorToolStateLocked(id, name); existing != nil {
		if strings.TrimSpace(name) != "" {
			existing.name = strings.TrimSpace(name)
		}
		return existing
	}
	name = strings.TrimSpace(name)
	if !directorPlanWriteTool(name) {
		return nil
	}
	id = strings.TrimSpace(id)
	key := directorToolStateKey(id, name)
	state := &directorToolDisplayState{id: id, name: name}
	c.directorTools[key] = state
	return state
}

func (c *DirectorConversation) findDirectorToolStateLocked(id, name string) *directorToolDisplayState {
	if len(c.directorTools) == 0 {
		return nil
	}
	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	if id != "" {
		for _, state := range c.directorTools {
			if state.id == id {
				return state
			}
		}
	}
	if name == "" {
		return nil
	}
	for _, state := range c.directorTools {
		if state.name == name {
			return state
		}
	}
	return nil
}

type directorToolDisplayState struct {
	id             string
	name           string
	rawArgs        string
	displayArgs    string
	generatedChars int
	sentChars      int
	event          session.DisplayEvent
	counter        directorToolTextCounter
}

func (s *directorToolDisplayState) appendArgs(delta string) {
	if s == nil || delta == "" {
		return
	}
	s.rawArgs += delta
	s.generatedChars += s.counter.countDelta(delta, directorToolGeneratedTextKeys(s.name))
}

func (s *directorToolDisplayState) projectEvent(event session.DisplayEvent) (session.DisplayEvent, bool) {
	if s == nil {
		return event, false
	}
	s.syncDecodedGeneratedChars()
	displayArgs, ok := s.projectDisplayArgs()
	if !ok {
		return event, false
	}
	event.Args = displayArgs
	markDirectorPlanInputHidden(&event, s.generatedChars)
	return event, true
}

func (s *directorToolDisplayState) progressEvent() (session.DisplayEvent, bool) {
	if s == nil {
		return session.DisplayEvent{}, false
	}
	event, ok := s.projectEvent(s.event)
	if !ok {
		return session.DisplayEvent{}, false
	}
	charsChanged := s.generatedChars-s.sentChars >= directorPlanProgressStep
	if event.Args == s.displayArgs && !charsChanged {
		return session.DisplayEvent{}, false
	}
	s.displayArgs = event.Args
	s.sentChars = s.generatedChars
	return event, true
}

func (s *directorToolDisplayState) projectDisplayArgs() (string, bool) {
	if strings.TrimSpace(s.name) == producttools.SubmitDirectorPlanUpdateToolName {
		input := producttools.SubmitDirectorPlanUpdateInput{}
		if err := json.Unmarshal([]byte(strings.TrimSpace(s.rawArgs)), &input); err != nil {
			encoded, _ := json.Marshal(map[string]string{"mode": "pending"})
			return string(encoded), true
		}
		mode := strings.TrimSpace(string(input.Decision.Mode))
		if mode == "" {
			mode = "pending"
		}
		encoded, _ := json.Marshal(map[string]any{
			"mode":      mode,
			"documents": len(input.Updates),
			"finalize":  input.Finalize,
		})
		return string(encoded), true
	}
	preview, ok := directorToolPathArgPreviewFromArgs(s.rawArgs)
	if !ok || !isDirectorPlanPath(preview.path) {
		return "", false
	}
	return `{"path":"director.md"}`, true
}

func (s *directorToolDisplayState) syncDecodedGeneratedChars() {
	if s == nil {
		return
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(strings.TrimSpace(s.rawArgs)), &payload); err != nil {
		return
	}
	if strings.TrimSpace(s.name) == producttools.SubmitDirectorPlanUpdateToolName {
		var input producttools.SubmitDirectorPlanUpdateInput
		if err := json.Unmarshal([]byte(strings.TrimSpace(s.rawArgs)), &input); err == nil {
			total := 0
			for _, update := range input.Updates {
				for _, edit := range update.Edits {
					total += utf8.RuneCountInString(edit.Content)
				}
			}
			s.generatedChars = total
		}
		return
	}
	if strings.TrimSpace(s.name) == "edit" {
		var input struct {
			Edits []struct {
				NewString string `json:"new_string"`
			} `json:"edits"`
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(s.rawArgs)), &input); err == nil {
			total := 0
			for _, edit := range input.Edits {
				total += utf8.RuneCountInString(edit.NewString)
			}
			s.generatedChars = total
		}
		return
	}
	for _, key := range directorToolGeneratedTextKeys(s.name) {
		raw, ok := payload[key]
		if !ok {
			continue
		}
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			continue
		}
		s.generatedChars = utf8.RuneCountInString(text)
		return
	}
}

func decorateDirectorDisplayEvent(event session.DisplayEvent) session.DisplayEvent {
	if strings.TrimSpace(event.AgentKind) == "" {
		event.AgentKind = config.AgentKindInteractiveDirector
	}
	if strings.TrimSpace(event.AgentName) == "" {
		event.AgentName = "interactive_director"
	}
	if strings.TrimSpace(event.RootAgentName) == "" {
		event.RootAgentName = event.AgentName
	}
	if strings.TrimSpace(event.Content) == "" && strings.TrimSpace(event.Name) != "" {
		event.Content = strings.TrimSpace(event.Name)
	}
	return event
}

func markDirectorPlanInputHidden(event *session.DisplayEvent, generatedChars int) {
	if event == nil {
		return
	}
	event.SSEHiddenFields = []string{"content", "new_string", "old_string", "plan", "agent_brief", "lore_context"}
	event.SSEHiddenReason = directorPlanHiddenReason
	event.SSEDisplayNotice = directorPlanHiddenNotice
	if generatedChars > 0 {
		event.SSEGeneratedChars = generatedChars
	}
}

func directorPlanWriteTool(name string) bool {
	switch strings.TrimSpace(name) {
	case "write", "edit", producttools.SubmitDirectorPlanUpdateToolName:
		return true
	default:
		return false
	}
}

func isDirectorPlanPath(path string) bool {
	normalized := strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
	for strings.HasSuffix(normalized, "/") {
		normalized = strings.TrimSuffix(normalized, "/")
	}
	for _, name := range []string{"director.md", "agent-brief.md", "lore-context.md"} {
		if normalized == name || strings.HasSuffix(normalized, "/"+name) {
			return true
		}
	}
	return false
}
