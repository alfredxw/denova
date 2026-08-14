// Package tools provides provider-neutral model-visible tools and the host
// adapters used to implement them.
//
// File names make the module role explicit:
//   - *_tool.go and *_tools.go define model-visible tool interfaces.
//   - workspace_* files implement local workspace adapters and search behavior.
//   - shell_* files implement command execution and platform process handling.
//   - task_* files define delegation and its local executor adapter.
//   - toolset_* files construct and initialize composed toolsets.
//   - tool_* files contain shared definition, schema, and result contracts.
package tools
