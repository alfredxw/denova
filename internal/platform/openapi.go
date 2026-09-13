package platform

import "strings"

// OpenAPI describes only implemented A-release consumer routes. Management and
// future contribution families are deliberately absent from scoped discovery.
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
		"Scope":         object(map[string]any{"kind": map[string]any{"enum": []string{"project", "session", "game-instance"}}, "projectId": str, "sessionId": str, "instanceId": str}, "kind"),
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
	for _, prefix := range []string{"/game-data", "/plugin-data"} {
		add("get", prefix+"/files", "List this scope's portable files", nil, object(map[string]any{"items": array(jsonObject)}, "items"), "200", []string{"directory"})
		add("get", prefix+"/file", "Read a UTF-8 file and its revision", nil, ref("File"), "200", []string{"path"})
		add("put", prefix+"/file", "Write a file with compare-and-swap", ref("FileMutation"), object(map[string]any{"revision": str}, "revision"), "200", nil)
		add("delete", prefix+"/file", "Delete a file with compare-and-swap", ref("FileMutation"), nil, "204", nil)
	}
	return map[string]any{"openapi": "3.1.0", "info": map[string]string{"title": "Denova Platform", "version": "1.0.0"}, "servers": []map[string]string{{"url": "/api/platform/v1"}}, "security": []map[string]any{{"bearerAuth": []string{}}}, "components": map[string]any{"schemas": schemas, "securitySchemes": map[string]any{"bearerAuth": map[string]string{"type": "http", "scheme": "bearer"}}}, "paths": paths}
}
