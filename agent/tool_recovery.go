package agent

import "github.com/alfredxw/denova/agent/internal/protocol"

// UnknownToolEffectResult is the provider-neutral synthetic result for a tool
// whose external effect may have completed before durable settlement. It tells
// the next model call to inspect canonical state and never retry blindly.
//
// The value is public because context normalizers and custom Agent hosts need
// to recognize the same recovery protocol without importing lifecycle runtime
// implementation types.
const UnknownToolEffectResult = protocol.UnknownToolEffectResult
