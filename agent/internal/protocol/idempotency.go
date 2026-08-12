package protocol

// MaxIdempotencyKeyBytes is the durable command-identity bound shared by the
// public Agent API and its private runtime admission layer.
const MaxIdempotencyKeyBytes = 4 << 10
