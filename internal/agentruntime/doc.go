// Package agentruntime coordinates durable Agent operations independently of
// any model provider, tool implementation, or product domain store.
//
// A BindingRef has exactly one journal-backed actor. Public commands become
// observable only after their event batch is durable, and command IDs remain
// idempotent across process restarts. Session and Story stores remain the
// canonical source of user content; the runtime journal stores coordination
// state and crosses those store boundaries through input/output intent and
// receipt barriers.
//
// Provider streams and unknown tool effects are deliberately not resumed after
// a crash. The host supplies compatible runtime dependencies from bounded
// restore descriptors, reconciles canonical receipts, and explicitly resumes a
// server-projected safe action. Display history is a separate projection and is
// never treated as future model context.
package agentruntime
