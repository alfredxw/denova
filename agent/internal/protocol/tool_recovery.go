// Package protocol contains provider-neutral wire constants shared by the
// public Agent interface and its private lifecycle implementation.
package protocol

const UnknownToolEffectResult = `{"schema":"agent.tool_result.recovery.v1","status":"effect_unknown","automatic_retry":false,"message":"The previous tool effect may have completed before the runtime stopped. Verify canonical state before deciding any next action; do not automatically retry."}`
