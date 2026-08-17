package interaction

import agent "github.com/alfredxw/denova/agent"

// Standard validates the built-in ask and permission vocabulary.
func Standard() agent.InteractionPolicy { return agent.StandardInteraction() }
