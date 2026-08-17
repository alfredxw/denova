// Package agents is Denova's Agent composition boundary. It wires product
// builders while exposing the stable message facade used by the app layer.
//
// Chat execution, durable coordination, one-shot model tasks, context,
// conversations, prompts, tool policy, and interactive protocols live in
// focused subpackages. Those subpackages never import this root package,
// keeping dependencies directed from product composition toward reusable
// implementation.
package agents
