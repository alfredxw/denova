package interactive

func builtinGamePlanningTemplates() []GamePlanningTemplate {
	return []GamePlanningTemplate{
		planningTemplate("default", "Balanced story planning", "A general-purpose outline that balances direction, arcs, near-term beats, characters, threads, branches, and continuity.", []GamePlanningSection{
			{ID: "long-term-direction", Title: "Long-term direction", Description: "Define likely end states, irreversible transformations, the central dramatic question, and what must remain open to player choice. Plan possibilities rather than a fixed script."},
			{ID: "mid-term-arcs", Title: "Mid-term arcs", Description: "Track the next major arcs, their prerequisites, escalation, turning points, and possible exits."},
			{ID: "near-term-beats", Title: "Near-term beats", Description: "Plan the next few meaningful beats with purpose, pressure, information, consequences, and a return point for player agency."},
			{ID: "character-deployment", Title: "Character deployment", Description: "Track each important character's current motive, agency, relationship movement, and next useful collision."},
			{ID: "threads-and-payoffs", Title: "Threads and payoffs", Description: "Track open promises, clues, mysteries, threats, debts, setups, and expected payoff windows."},
			{ID: "branch-possibilities", Title: "Branch possibilities", Description: "Preserve distinct responses to plausible player directions. State what can change, what remains invariant, and how preparation can be recycled without forcing a route."},
			{ID: "continuity-and-replanning", Title: "Continuity and replanning", Description: "Record constraints from committed history, Actor State, Lore, and user intent. State the triggers for local adjustment or full replanning."},
		}),
		planningTemplate("mystery-investigation", "Mystery and investigation", "An evidence-led outline for fair mysteries, investigations, and revelation pacing.", []GamePlanningSection{
			{ID: "truth-and-stakes", Title: "Underlying truth and stakes", Description: "State the hidden truth, who benefits from concealment, and what changes if the player never intervenes."},
			{ID: "evidence-map", Title: "Evidence map", Description: "Track discovered and undiscovered evidence, what each clue supports, alternative access paths, and prerequisites."},
			{ID: "suspects-and-pressure", Title: "Suspects and pressure", Description: "Track motives, alibis, vulnerabilities, countermoves, and how pressure changes each suspect's behavior."},
			{ID: "revelation-pacing", Title: "Revelation pacing", Description: "Plan the next realizations and reversals while ensuring decisive conclusions have perceptible preparation."},
			{ID: "false-leads", Title: "False leads and corrections", Description: "Record plausible misreadings, the fair signals that expose them, and recovery routes that avoid dead ends."},
			{ID: "investigation-branches", Title: "Investigation branches", Description: "Preserve multiple viable approaches and show how consequences differ without making one route mandatory."},
		}),
		planningTemplate("character-relationships", "Character and relationships", "A character-led outline for motives, relationships, emotional turns, and ensemble pacing.", []GamePlanningSection{
			{ID: "character-trajectories", Title: "Character trajectories", Description: "Track each central character's desire, fear, contradiction, agency, and plausible transformation."},
			{ID: "relationship-map", Title: "Relationship movement", Description: "Track bonds, fractures, leverage, misunderstandings, and the next meaningful change in each important relationship."},
			{ID: "emotional-beats", Title: "Emotional beats", Description: "Plan earned emotional turns with setup, pressure, choice, reaction, and space for the player to respond."},
			{ID: "ensemble-deployment", Title: "Ensemble deployment", Description: "Balance active, resting, entering, and exiting characters so the same cast does not crowd every scene."},
			{ID: "secrets-and-boundaries", Title: "Secrets and boundaries", Description: "Track what each character knows, conceals, assumes, and refuses, including triggers that change those boundaries."},
			{ID: "relationship-branches", Title: "Relationship branches", Description: "Preserve distinct relationship outcomes driven by player choices without precommitting affection, conflict, or reconciliation."},
		}),
		planningTemplate("sandbox-exploration", "Sandbox and exploration", "A flexible outline for open worlds, factions, locations, opportunities, and player-led direction.", []GamePlanningSection{
			{ID: "world-momentum", Title: "World momentum", Description: "Track important developments that continue without the player and the signals that make those changes legible."},
			{ID: "regions-and-routes", Title: "Regions and routes", Description: "Track reachable locations, discoveries, hazards, travel choices, and what opens or closes each route."},
			{ID: "factions-and-fronts", Title: "Factions and fronts", Description: "Track each faction's goal, resources, active move, conflicts, and response to player interference."},
			{ID: "opportunity-board", Title: "Opportunities and hooks", Description: "Maintain varied optional goals with visible stakes, time sensitivity, dependencies, and consequences for ignoring them."},
			{ID: "nearby-beats", Title: "Nearby beats", Description: "Prepare only the next useful encounters and discoveries in enough detail to respond without forcing a destination."},
			{ID: "consequences-and-replanning", Title: "Consequences and replanning", Description: "Record committed world changes and the triggers that retire, transform, or generate opportunities."},
		}),
	}
}

func planningTemplate(id, name, description string, sections []GamePlanningSection) GamePlanningTemplate {
	return normalizeGamePlanningTemplate(GamePlanningTemplate{
		Version:     gamePlanningTemplateVersion,
		ID:          id,
		Name:        name,
		Description: description,
		Sections:    sections,
		Custom:      false,
	})
}

func builtinGamePlanningTemplateByID(id string) (GamePlanningTemplate, bool) {
	for _, item := range builtinGamePlanningTemplates() {
		if item.ID == id {
			return item, true
		}
	}
	return GamePlanningTemplate{}, false
}
