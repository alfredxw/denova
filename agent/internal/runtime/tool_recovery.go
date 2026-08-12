package runtime

import "github.com/alfredxw/denova/agent/internal/protocol"

// UnknownToolEffectResult is the provider-neutral result paired with a tool
// call whose durable start exists but whose completion receipt was absent at
// crash recovery. It is deliberately constant: the call ID and tool name live
// in the surrounding protocol message, while this body remains stable across
// journal replay, context compaction, and provider adapters.
const UnknownToolEffectResult = protocol.UnknownToolEffectResult
