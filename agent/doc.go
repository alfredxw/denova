// Package agent provides a reusable, provider-neutral agent core.
//
// The package is intentionally layered around stable seams:
//
//   - Message and StreamReader define persisted and streaming wire data.
//   - BaseChatModel and Tool isolate provider and tool implementations.
//   - ToolDefinition and Registry form one validated schema/descriptor catalog.
//   - Agent owns the native model/tool loop; Runner is a convenience entry point.
//   - Middleware and Host expose integration seams without importing a
//     provider SDK, workflow engine, or application package.
//
// The context, session, tools, and runtime subpackages add bounded context
// assembly, append-only transcripts, complete tool definitions, and durable
// orchestration. Optional provider adapters depend on this module; the core
// never depends on them.
package agent
