package runtime

// UnknownToolEffectResult is the provider-neutral result paired with a tool
// call whose durable start exists but whose completion receipt was absent at
// crash recovery. It is deliberately constant: the call ID and tool name live
// in the surrounding protocol message, while this body remains stable across
// journal replay, context compaction, and provider adapters.
const UnknownToolEffectResult = `{"schema":"agent.tool_result.recovery.v1","status":"effect_unknown","automatic_retry":false,"message":"The previous tool effect may have completed before the runtime stopped. Verify canonical state before deciding any next action; do not automatically retry."}`
