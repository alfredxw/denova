// Package agents composes Denova's writing, game, automation, image, and
// configuration Agents from the reusable public agent module.
//
// This package owns product prompts, tool policy, Conversation adapters,
// context compaction, display projection, and domain-commit coordination.
// Provider-neutral loop, runtime, context, session, and standard-tool
// contracts belong to github.com/alfredxw/denova/agent and its subpackages.
// Product-local subpackages are kept only where they form a real dependency
// seam; closely coupled orchestration stays here instead of being split into
// forwarding packages.
package agents
