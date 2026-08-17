// Package agent provides a provider-neutral, composable Agent core.
//
// The public lifecycle is Agent -> Session -> Run. Agent owns admission,
// model/tool execution, permission fences, canonical commits, interactions,
// compaction, and event publication. Definition and Source expose
// the stable composition seams for models, tools, context, goals, compaction,
// permissions, interactions, product canonical state, and middleware.
//
// The session package provides in-memory and file-backed Store implementations;
// tools provides independently selectable common coding tools; optional
// network and browser integrations live under plugins. Provider adapters may
// depend on this module, while the Agent core never depends on an application
// or presentation framework.
package agent
