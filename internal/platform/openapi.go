package platform

import "strings"

// OpenAPI describes implemented consumer routes. Management APIs and host
// internals are deliberately absent from scoped discovery.
func OpenAPI() map[string]any {
	ref := func(name string) map[string]any { return map[string]any{"$ref": "#/components/schemas/" + name} }
	str := map[string]any{"type": "string"}
	object := func(properties map[string]any, required ...string) map[string]any {
		return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
	}
	array := func(items any) map[string]any { return map[string]any{"type": "array", "items": items} }
	jsonObject := map[string]any{"type": "object", "additionalProperties": true}
	schemas := map[string]any{
		"Error":         object(map[string]any{"code": str, "messageKey": str, "diagnostic": str}, "code", "messageKey", "diagnostic"),
		"PackageRef":    object(map[string]any{"kind": map[string]any{"enum": []string{"plugin", "game"}}, "id": str}, "kind", "id"),
		"ReleaseRef":    object(map[string]any{"package": ref("PackageRef"), "releaseId": str}, "package", "releaseId"),
		"Scope":         object(map[string]any{"kind": map[string]any{"enum": []string{"project", "session", "story", "game-instance"}}, "projectId": str, "sessionId": str, "instanceId": str, "storyId": str, "branchId": str}, "kind"),
		"Context":       object(map[string]any{"source": ref("ReleaseRef"), "scope": ref("Scope"), "locale": str, "theme": str, "environment": str, "settings": jsonObject, "setup": jsonObject}, "source", "scope", "locale", "theme", "environment", "settings"),
		"AgentRef":      object(map[string]any{"owner": object(map[string]any{"kind": str, "projectId": str, "sessionId": str}, "kind", "projectId", "sessionId"), "sessionId": str}, "owner", "sessionId"),
		"AgentSession":  object(map[string]any{"ref": ref("AgentRef"), "definition": str, "key": str}, "ref", "definition", "key"),
		"EnsureSession": object(map[string]any{"projectId": str, "definition": str, "key": map[string]any{"type": "string", "minLength": 1, "maxLength": 256}}, "projectId", "definition", "key"),
		"RunRef":        object(map[string]any{"agent": ref("AgentRef"), "runId": str}, "agent", "runId"),
		"RunResult":     object(map[string]any{"run": ref("RunRef"), "status": map[string]any{"enum": []string{"accepted", "running", "waiting", "completed", "failed", "aborted", "incomplete"}}, "text": str, "error": ref("Error"), "completion": object(map[string]any{"agent": ref("AgentRef"), "recordId": str}, "agent", "recordId")}, "run", "status", "text"),
		"RunInput":      object(map[string]any{"commandId": map[string]any{"type": "string", "minLength": 1, "maxLength": 256}, "input": object(map[string]any{"text": str}, "text")}, "commandId", "input"),
		"File":          object(map[string]any{"content": str, "revision": str}, "content", "revision"),
		"FileMutation":  object(map[string]any{"path": str, "content": str, "expectedRevision": map[string]any{"type": []string{"string", "null"}, "description": "null creates only; a nonempty revision performs compare-and-swap"}}, "path"),
		"ToolResult":    object(map[string]any{"content": str, "data": map[string]any{}}, "content"),
		"History":       object(map[string]any{"items": array(object(map[string]any{"recordId": str, "role": str, "text": str, "createdAt": str}, "recordId", "role", "text", "createdAt")), "cursor": str}, "items", "cursor"),
	}
	integer := map[string]any{"type": "integer", "minimum": 0}
	boolean := map[string]any{"type": "boolean"}
	schemas["AssetRef"] = object(map[string]any{"kind": map[string]any{"enum": []string{"project", "generated"}}, "path": str}, "kind", "path")
	schemas["LibraryItem"] = object(map[string]any{"id": str, "type": str, "name": str, "tags": array(str), "briefDescription": str, "updatedAt": str, "enabled": boolean, "keywords": array(str), "content": str, "image": ref("AssetRef")}, "id", "type", "name", "tags", "briefDescription", "updatedAt", "enabled")
	schemas["LibraryPage"] = object(map[string]any{"items": array(ref("LibraryItem")), "total": integer, "nextOffset": integer}, "items", "total")
	schemas["ImageRequest"] = object(map[string]any{"commandId": str, "modelSlot": str, "prompt": map[string]any{"type": "string", "description": "English image prompt; 1..65536 UTF-8 bytes"}, "size": str, "aspectRatio": str, "quality": str}, "commandId", "modelSlot", "prompt")
	schemas["GeneratedImage"] = object(map[string]any{"asset": ref("AssetRef"), "mimeType": str, "sizeBytes": integer, "revisedPrompt": str}, "asset", "mimeType", "sizeBytes")
	schemas["ImageResult"] = object(map[string]any{"commandId": str, "status": map[string]any{"enum": []string{"running", "completed", "failed", "cancelled", "interrupted"}}, "images": array(ref("GeneratedImage")), "error": ref("Error")}, "commandId", "status", "images")
	schemas["StoryTurn"] = object(map[string]any{"id": str, "revision": str, "user": str, "narrative": str, "choices": array(str), "versions": array(str)}, "id", "revision", "user", "narrative", "choices", "versions")
	schemas["StoryHistory"] = object(map[string]any{"turns": array(ref("StoryTurn")), "beforeCursor": str, "hasMore": boolean}, "turns", "hasMore")
	schemas["StorySnapshot"] = object(map[string]any{"storyId": str, "branchId": str, "title": str, "turns": array(ref("StoryTurn")), "branches": array(object(map[string]any{"id": str, "title": str, "current": boolean}, "id", "title", "current")), "beforeCursor": str, "hasMore": boolean, "status": str, "operationId": str, "interruptionId": str}, "storyId", "branchId", "title", "turns", "branches", "hasMore", "status")
	schemas["StoryCommand"] = object(map[string]any{"kind": map[string]any{"enum": []StoryCommandKind{StoryAdvance, StoryResume, StoryRegenerate, StoryStop, StoryFork, StorySwitchBranch, StorySwitchVersion}}, "commandId": str, "message": str, "locale": str, "operationId": str, "interruptionId": str, "branchId": str, "turnId": str, "versionTurnId": str, "title": str}, "kind", "commandId")
	schemas["StoryRecord"] = object(map[string]any{"revision": integer, "schemaVersion": integer, "value": map[string]any{}}, "revision", "schemaVersion", "value")
	schemas["StoryRecordRequest"] = object(map[string]any{"key": str, "branchId": str, "turnId": str, "sourceRevision": str, "expectedRevision": integer, "schemaVersion": integer, "value": map[string]any{}}, "key", "expectedRevision", "schemaVersion", "value")
	paths := map[string]any{}
	add := func(method, path, summary string, requestSchema, responseSchema any, success string, query []string) {
		content := func(schema any) map[string]any {
			return map[string]any{"application/json": map[string]any{"schema": schema}}
		}
		response := map[string]any{"description": "Operation completed"}
		if responseSchema != nil {
			response["content"] = content(responseSchema)
		}
		operation := map[string]any{"summary": summary, "responses": map[string]any{success: response, "default": map[string]any{"description": "Localized error; never retry an uncertain mutation with a new commandId", "content": content(ref("Error"))}}}
		parameters := []map[string]any{}
		for _, segment := range strings.Split(path, "/") {
			if strings.HasPrefix(segment, "{") {
				parameters = append(parameters, map[string]any{"name": strings.Trim(segment, "{}"), "in": "path", "required": true, "schema": str})
			}
		}
		for _, name := range query {
			parameters = append(parameters, map[string]any{"name": name, "in": "query", "schema": str})
		}
		if len(parameters) > 0 {
			operation["parameters"] = parameters
		}
		if requestSchema != nil {
			operation["requestBody"] = map[string]any{"required": true, "content": content(requestSchema)}
		}
		if strings.HasSuffix(path, "/events") {
			response["content"] = map[string]any{"text/event-stream": map[string]any{"schema": str}}
			operation["description"] = "Starts with snapshot, then delta/state/interaction/result events. Cursor expiration returns CURSOR_EXPIRED; reconnect without Last-Event-ID to obtain a fresh snapshot."
			operation["parameters"] = append(parameters, map[string]any{"name": "Last-Event-ID", "in": "header", "schema": str})
		}
		item, _ := paths[path].(map[string]any)
		if item == nil {
			item = map[string]any{}
			paths[path] = item
		}
		item[method] = operation
	}
	add("get", "/context", "Read the bound runtime context", nil, ref("Context"), "200", nil)
	add("get", "/capabilities", "Read granted permissions and capacity limits", nil, jsonObject, "200", nil)
	add("get", "/openapi.json", "Read this API description", nil, jsonObject, "200", nil)
	add("get", "/contributions", "List visible pinned capabilities", nil, array(jsonObject), "200", nil)
	add("get", "/agents/definitions", "List allowed Agent definitions", nil, object(map[string]any{"items": array(str)}, "items"), "200", nil)
	add("post", "/agents/sessions", "Ensure an immutable Agent configuration under a stable key", ref("EnsureSession"), ref("AgentSession"), "201", nil)
	paths["/agents/sessions"].(map[string]any)["post"].(map[string]any)["responses"].(map[string]any)["200"] = map[string]any{"description": "Existing session", "content": map[string]any{"application/json": map[string]any{"schema": ref("AgentSession")}}}
	add("get", "/agents/sessions/{sessionId}", "Read a session binding", nil, ref("AgentSession"), "200", nil)
	add("get", "/agents/sessions/{sessionId}/history", "Read a bounded history page", nil, ref("History"), "200", []string{"cursor", "limit"})
	add("post", "/agents/sessions/{sessionId}/runs", "Durably admit or recover one commandId", ref("RunInput"), ref("RunResult"), "202", nil)
	add("get", "/agents/sessions/{sessionId}/runs", "Find a command after refresh or restart", nil, ref("RunResult"), "200", []string{"commandId"})
	add("get", "/agents/runs/{runId}", "Read a current or durable result", nil, ref("RunResult"), "200", nil)
	add("get", "/agents/runs/{runId}/events", "Subscribe to run events", nil, str, "200", nil)
	add("post", "/agents/runs/{runId}/cancel", "Request cancellation without deleting history", nil, ref("RunResult"), "200", nil)
	add("post", "/agents/runs/{runId}/interactions/{interactionId}/responses", "Answer an ordinary question; permission approval is forbidden", jsonObject, nil, "204", nil)
	add("post", "/tools/{providerId}/{toolId}/invoke", "Invoke a selected tool with its validated input", object(map[string]any{"input": map[string]any{}}, "input"), ref("ToolResult"), "200", nil)
	add("get", "/library/items", "Read a page from the bound Project library", nil, ref("LibraryPage"), "200", []string{"query", "offset", "limit"})
	add("get", "/library/items/{id}", "Read one library item including its full content", nil, ref("LibraryItem"), "200", nil)
	add("get", "/assets/content", "Read authorized raster bytes using a portable asset reference", nil, nil, "200", []string{"kind", "path"})
	paths["/assets/content"].(map[string]any)["get"].(map[string]any)["responses"].(map[string]any)["200"].(map[string]any)["content"] = map[string]any{"image/png": map[string]any{}, "image/jpeg": map[string]any{}, "image/webp": map[string]any{}, "image/gif": map[string]any{}}
	add("post", "/images/generations", "Start or recover an image request without replaying a paid operation", ref("ImageRequest"), ref("ImageResult"), "202", nil)
	paths["/images/generations"].(map[string]any)["post"].(map[string]any)["responses"].(map[string]any)["200"] = map[string]any{"description": "Previously settled request", "content": map[string]any{"application/json": map[string]any{"schema": ref("ImageResult")}}}
	add("get", "/images/generations/{commandId}", "Find this scope's image request after reconnection", nil, ref("ImageResult"), "200", nil)
	add("post", "/images/generations/{commandId}/cancel", "Cancel this scope's image request", nil, ref("ImageResult"), "200", nil)
	add("get", "/story/events", "Follow provisional prose for the observed Story operation", nil, str, "200", []string{"operationId"})
	paths["/story/events"].(map[string]any)["get"].(map[string]any)["description"] = "Data-only SSE frames contain kind reset, delta (with text), or settled. Reconnect replays current provisional prose from reset. Reload /story after settled or disconnect; only its turns are committed. No reasoning or tool events are exposed."
	paths["/story/events"].(map[string]any)["get"].(map[string]any)["parameters"] = []map[string]any{{"name": "operationId", "in": "query", "required": true, "schema": str}}
	add("get", "/story", "Read the linked Story's player-visible snapshot", nil, ref("StorySnapshot"), "200", nil)
	add("get", "/story/history", "Read earlier turns in an authorized Story branch", nil, ref("StoryHistory"), "200", []string{"branchId", "beforeCursor", "limit"})
	add("post", "/story/commands", "Perform an existing Story operation through its canonical command path", ref("StoryCommand"), ref("StorySnapshot"), "200", nil)
	add("get", "/story/records", "Read extension JSON bound to a Story or exact turn revision", nil, ref("StoryRecord"), "200", []string{"key", "branchId", "turnId", "sourceRevision"})
	add("put", "/story/records", "Commit extension JSON to the same Story journal with revision protection", ref("StoryRecordRequest"), ref("StoryRecord"), "200", nil)
	for _, prefix := range []string{"/game-data", "/plugin-data"} {
		add("get", prefix+"/files", "List this scope's portable files", nil, object(map[string]any{"items": array(jsonObject)}, "items"), "200", []string{"directory"})
		add("get", prefix+"/file", "Read a UTF-8 file and its revision", nil, ref("File"), "200", []string{"path"})
		add("put", prefix+"/file", "Write a file with compare-and-swap", ref("FileMutation"), object(map[string]any{"revision": str}, "revision"), "200", nil)
		add("delete", prefix+"/file", "Delete a file with compare-and-swap", ref("FileMutation"), nil, "204", nil)
	}
	return map[string]any{"openapi": "3.1.0", "info": map[string]string{"title": "Denova Platform", "version": "1.0.0"}, "servers": []map[string]string{{"url": "/api/platform/v1"}}, "security": []map[string]any{{"bearerAuth": []string{}}}, "components": map[string]any{"schemas": schemas, "securitySchemes": map[string]any{"bearerAuth": map[string]string{"type": "http", "scheme": "bearer"}}}, "paths": paths}
}
