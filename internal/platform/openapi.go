package platform

import (
	"encoding/json"
	"reflect"
	"strings"

	"github.com/invopop/jsonschema"
)

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
	// Public DTOs own field names and shapes. Protocol constraints stay next to
	// those fields; route descriptions below document behavior, not copies of DTOs.
	reflector := jsonschema.Reflector{Anonymous: true, DoNotReference: true, Mapper: func(t reflect.Type) *jsonschema.Schema {
		if t == reflect.TypeFor[json.RawMessage]() {
			return &jsonschema.Schema{}
		}
		return nil
	}}
	schemas := map[string]any{}
	for name, value := range map[string]any{
		"Error": Error{}, "PackageRef": PackageRef{}, "ReleaseRef": ReleaseRef{},
		"Scope": Scope{}, "Context": RuntimeContext{}, "AgentRef": AgentRef{},
		"AgentSession": AgentSession{}, "EnsureSession": EnsureAgentSession{},
		"RunRef": RunRef{}, "RunResult": RunResult{}, "RunInput": agentRunInput{},
		"File": FileSnapshot{}, "FileMutation": ContentDocument{}, "ToolResult": ToolResult{},
		"ConfigurationForm": ConfigurationForm{}, "ConfigurationDocument": ConfigurationDocument{},
		"ConfigurationInput": ConfigurationInput{}, "StoryInstanceInput": createStoryInput{},
		"StoryInstance": Instance{}, "AssetRef": AssetRef{}, "LibraryItem": LibraryItem{},
		"LibraryPage": LibraryPage{}, "LibraryWrite": LibraryWrite{}, "ContentSource": ContentSource{},
		"ImageRequest": ImageRequest{}, "GeneratedImage": GeneratedImage{}, "ImageResult": ImageResult{},
		"StoryStateField": StoryStateField{}, "StoryStateTemplate": StoryStateTemplate{},
		"StoryStateSchema": StoryStateSchema{}, "StoryStateChange": StoryStateChange{},
		"StoryState": StoryState{}, "StoryScene": StoryScene{}, "StoryModuleRefs": StoryModuleRefs{},
		"StoryInitialActor": StoryInitialActor{}, "StoryProtagonist": StoryProtagonist{},
		"StoryConfiguration": StoryConfiguration{}, "StoryPreset": StoryPreset{},
		"StoryPresetImport": StoryPresetImport{}, "StoryTurn": StoryTurn{},
		"StoryHistory": StoryHistory{}, "StorySnapshot": StorySnapshot{},
		"StoryCommand": StoryCommand{}, "StoryRecord": StoryRecord{}, "StoryRecordRequest": StoryRecordRequest{},
	} {
		schemas[name] = reflector.Reflect(value)
	}
	schemas["History"] = object(map[string]any{"items": array(object(map[string]any{"recordId": str, "role": str, "text": str, "createdAt": str}, "recordId", "role", "text", "createdAt")), "cursor": str}, "items", "cursor")
	integer := map[string]any{"type": "integer", "minimum": 0}
	boolean := map[string]any{"type": "boolean"}
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
	add("get", "/settings", "Read settings for the owning installed release", nil, ref("ConfigurationDocument"), "200", nil)
	add("put", "/settings", "Save owning extension preferences through native revision checks", ref("ConfigurationInput"), ref("ConfigurationDocument"), "200", nil)
	paths["/settings"].(map[string]any)["put"].(map[string]any)["description"] = "Requires settings.write on the owning installed view. Preview and host-only Agent runtimes cannot edit shared preferences. Preserves native schema validation, backups and release/revision conflict checks. The current runtime context stays frozen; a view may apply returned values locally."
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
	add("post", "/library/items", "Save an explicitly adopted item through the native library; requires library.write", ref("LibraryWrite"), ref("LibraryItem"), "200", nil)
	paths["/library/items"].(map[string]any)["post"].(map[string]any)["description"] = "Create uses an exact stable native ID (letters, digits, hyphen or underscore); identical input is replayed. Updating requires baseRevision from updatedAt. Existing content is backed up before native revision-checked replacement. Image binding is preserved, never replaced by this route. Project identity comes only from the runtime credential."
	add("get", "/assets/content", "Read authorized raster bytes using a portable asset reference", nil, nil, "200", []string{"kind", "path"})
	paths["/assets/content"].(map[string]any)["get"].(map[string]any)["responses"].(map[string]any)["200"].(map[string]any)["content"] = map[string]any{"image/png": map[string]any{}, "image/jpeg": map[string]any{}, "image/webp": map[string]any{}, "image/gif": map[string]any{}, "audio/wave": map[string]any{}, "audio/mpeg": map[string]any{}, "audio/ogg": map[string]any{}}
	add("post", "/assets/upload", "Adopt raster or audio bytes into this Project and extension's shared content; requires assets.write", str, ref("GeneratedImage"), "200", nil)
	paths["/assets/upload"].(map[string]any)["post"].(map[string]any)["requestBody"] = map[string]any{"required": true, "content": map[string]any{"application/octet-stream": map[string]any{"schema": map[string]any{"type": "string", "format": "binary", "maxLength": MaxAssetBytes}}}}
	add("get", "/assets/documents", "List this Project and extension's JSON authoring content; requires assets.read", nil, object(map[string]any{"items": array(str)}, "items"), "200", nil)
	add("get", "/assets/document", "Read a JSON authoring document and its revision; never an Agent journal", nil, ref("File"), "200", []string{"path"})
	add("put", "/assets/document", "Save a JSON authoring document with compare-and-swap; requires assets.write", ref("FileMutation"), object(map[string]any{"revision": str}, "revision"), "200", nil)
	add("post", "/assets/source", "Download a public GitHub source archive or a checksum-pinned release ZIP; does not install or execute content", ref("ContentSource"), nil, "200", nil)
	paths["/assets/source"].(map[string]any)["post"].(map[string]any)["responses"].(map[string]any)["200"].(map[string]any)["content"] = map[string]any{"application/zip": map[string]any{"schema": map[string]any{"type": "string", "format": "binary"}}}
	add("post", "/images/generations", "Start or recover an image request without replaying a paid operation", ref("ImageRequest"), ref("ImageResult"), "202", nil)
	paths["/images/generations"].(map[string]any)["post"].(map[string]any)["responses"].(map[string]any)["200"] = map[string]any{"description": "Previously settled request", "content": map[string]any{"application/json": map[string]any{"schema": ref("ImageResult")}}}
	add("get", "/images/generations/{commandId}", "Find this scope's image request after reconnection", nil, ref("ImageResult"), "200", nil)
	add("post", "/images/generations/{commandId}/cancel", "Cancel this scope's image request", nil, ref("ImageResult"), "200", nil)
	add("get", "/story/events", "Follow provisional prose for the observed Story operation", nil, str, "200", []string{"operationId"})
	paths["/story/events"].(map[string]any)["get"].(map[string]any)["description"] = "Data-only SSE frames contain kind reset, delta (with text), or settled. Reconnect replays current provisional prose from reset. Reload /story after settled or disconnect; only its turns are committed. With includeActivity=true, activity frames carry a content-free phase: thinking, reading, checking, writing or saving. Reset reason=reclassified retracts a provisional response. No reasoning or tool contents are exposed."
	paths["/story/events"].(map[string]any)["get"].(map[string]any)["parameters"] = []map[string]any{{"name": "operationId", "in": "query", "required": true, "schema": str}, {"name": "includeActivity", "in": "query", "schema": boolean}}
	add("get", "/story", "Read the linked Story's player-visible snapshot", nil, ref("StorySnapshot"), "200", nil)
	add("post", "/story/instances", "Create an independent play with this view's bound Project, engine release and models", ref("StoryInstanceInput"), ref("StoryInstance"), "201", nil)
	paths["/story/instances"].(map[string]any)["post"].(map[string]any)["responses"].(map[string]any)["200"] = map[string]any{"description": "Existing completed creation command", "content": map[string]any{"application/json": map[string]any{"schema": ref("StoryInstance")}}}
	paths["/story/instances"].(map[string]any)["post"].(map[string]any)["description"] = "Requires stories.write on an owning bound Story game view. Input cannot select a Project, game, release, Story or model. commandId is recorded in the originating Story journal: identical completed input returns the original instance; different input conflicts. An interrupted admitted creation conflicts without creating another play; inspect existing journeys before issuing a new command."
	add("get", "/story/state", "Read committed current state or a historical turn without moving the branch", nil, ref("StoryState"), "200", []string{"branchId", "turnId"})
	add("get", "/story/scene", "Inspect one exact recorded turn version without changing the current Story path", nil, ref("StoryScene"), "200", []string{"branchId", "turnId", "sourceRevision"})
	sceneOperation := paths["/story/scene"].(map[string]any)["get"].(map[string]any)
	sceneOperation["description"] = "Requires stories.read on the owning bound Story view. All three query parameters are required. A turn must belong to this branch or its actual fork ancestry; discarded regenerated versions remain inspectable. State and previousTurn follow that version's native parent chain. A changed narrative revision returns DOCUMENT_CONFLICT. Does not select a branch/version, repair a journal, or write reading/presentation records."
	for _, parameter := range sceneOperation["parameters"].([]map[string]any) {
		parameter["required"] = true
	}
	add("get", "/story/presets", "Discover portable content from the six native Story preset libraries", nil, array(ref("StoryPreset")), "200", nil)
	add("post", "/story/presets/import", "Create or verify a native content-addressed preset copy; owning bound Story view and stories.write required", ref("StoryPresetImport"), ref("StoryPreset"), "200", nil)
	add("put", "/story/pacing", "Update native target character count for future turns without changing Story state", object(map[string]any{"replyTargetChars": integer}, "replyTargetChars"), ref("StorySnapshot"), "200", nil)
	add("put", "/story/configuration", "Configure native modules and Story-local initial Actors before the first turn", ref("StoryConfiguration"), ref("StorySnapshot"), "200", nil)
	paths["/story/configuration"].(map[string]any)["put"].(map[string]any)["description"] = "Requires stories.write and a bound idle Story with zero turns and no additional branches. Uses existing native preset resolution, validation and journal persistence; initialActors requires fixed_template. Presets are never modified. Query /story after an uncertain response before starting play."
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
