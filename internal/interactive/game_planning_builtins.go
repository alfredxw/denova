package interactive

func builtinGamePlanningTemplates() []GamePlanningTemplate {
	return []GamePlanningTemplate{
		planningTemplate("default", "Classic adventure", "A broadly applicable plan with a clear long line, flexible arcs, prepared near-term beats, and room for player-led change.", []GamePlanningSection{
			{ID: "long-term-direction", Title: "Long-term direction", Description: "Define likely end states, irreversible transformations, the central dramatic question, and what must remain open to player choice. Plan possibilities rather than a fixed script."},
			{ID: "mid-term-arcs", Title: "Mid-term arcs", Description: "Design the next major arcs, their prerequisites, escalation, turning points, and possible exits."},
			{ID: "near-term-beats", Title: "Near-term beats", Description: "Plan the next few meaningful beats with purpose, pressure, information, consequences, and a return point for player agency."},
			{ID: "character-deployment", Title: "Character deployment", Description: "Plan each important character's next useful entrance, exit, intervention, relationship movement, and independent action."},
			{ID: "threads-and-payoffs", Title: "Threads and payoffs", Description: "Schedule open promises, clues, mysteries, threats, debts, setups, and their plausible payoff windows."},
			{ID: "branch-possibilities", Title: "Branch possibilities", Description: "Preserve distinct responses to plausible player directions. State what can change, what remains invariant, and how preparation can be recycled without forcing a route."},
			{ID: "continuity-and-replanning", Title: "Continuity and replanning", Description: "Use committed history, Actor State, Lore, and user intent only as constraints. Define triggers for local adjustment or full replanning, and remove material that is completed or no longer viable."},
		}),
		planningTemplate("directed-longform", "Directed long-form", "A detailed, stable-mainline plan for progression stories, epic plots, and other narratives that need deliberate long-range structure.", []GamePlanningSection{
			{ID: "story-promise-and-endgame", Title: "Story promise and endgame", Description: "Define the experience this story should ultimately deliver, the central dramatic question, likely end states, and transformations that require long preparation."},
			{ID: "major-phase-map", Title: "Major phase map", Description: "Lay out the major phases or volumes in order, with the purpose, prerequisite, escalation, turning point, and exit condition of each. Keep distant phases concise until they approach."},
			{ID: "current-arc-blueprint", Title: "Current arc blueprint", Description: "Detail the active arc's objective, opposition, discoveries, reversals, midpoint, climax, aftermath, and the bridges into later arcs."},
			{ID: "near-term-scene-plan", Title: "Near-term scene plan", Description: "Prepare the next two to four candidate scenes with their dramatic job, location, active cast, pressure, information change, and handoff back to player agency."},
			{ID: "character-entrances-and-exits", Title: "Character entrances and exits", Description: "Schedule important introductions, returns, spotlight turns, temporary absences, betrayals, deaths, and departures only when their setup and story function are ready."},
			{ID: "setup-and-payoff-chain", Title: "Setup and payoff chain", Description: "Connect promises, abilities, rivals, mysteries, objects, costs, and foreshadowing to suitable reinforcement and payoff windows across phases."},
			{ID: "deviation-and-recovery", Title: "Deviation and recovery", Description: "Preserve player agency by defining which milestones are movable, which functions can transfer to another scene or character, and what signals require restructuring the main line."},
		}),
		planningTemplate("character-relationships", "Character and relationships", "A character-led outline for motives, relationships, emotional turns, and ensemble pacing.", []GamePlanningSection{
			{ID: "emotional-premise", Title: "Emotional premise", Description: "Define the central emotional question, the relationship experiences the story promises, and the boundaries that must remain responsive to player choice."},
			{ID: "character-trajectories", Title: "Character trajectories", Description: "Plan each central character's desire, fear, contradiction, independent agenda, pressure points, and several plausible transformations."},
			{ID: "relationship-trajectories", Title: "Relationship trajectories", Description: "Design the next meaningful movement in each important bond or rivalry, including setup, pressure, choice, reaction, and consequences rather than a predetermined outcome."},
			{ID: "current-emotional-arc", Title: "Current emotional arc", Description: "Detail the active emotional arc's immediate tension, unspoken need, scene opportunities, turning point, and a satisfying temporary landing point."},
			{ID: "emotional-beats", Title: "Emotional beats", Description: "Plan earned emotional turns with setup, pressure, choice, reaction, and space for the player to respond."},
			{ID: "ensemble-deployment", Title: "Ensemble deployment", Description: "Schedule entrances, exits, reunions, absences, and spotlight rotation so relationships can breathe and the same cast does not crowd every scene."},
			{ID: "secrets-and-boundaries", Title: "Secrets and boundaries", Description: "Plan when knowledge, concealment, mistaken assumptions, refusals, and personal boundaries may change, including the preparation each revelation requires."},
			{ID: "relationship-branches", Title: "Relationship branches", Description: "Preserve distinct relationship outcomes driven by player choices without precommitting affection, conflict, or reconciliation."},
		}),
		planningTemplate("mystery-dread", "Mystery and dread", "A fair-clue, pressure-aware plan for mysteries, investigations, horror, and concealed-rule stories.", []GamePlanningSection{
			{ID: "hidden-truth-and-logic", Title: "Hidden truth and logic", Description: "Define the concealed truth, cause, timeline, responsible forces, stakes, and any supernatural or procedural rules. Keep this as future reveal structure, not a recap of discovered facts."},
			{ID: "revelation-ladder", Title: "Revelation ladder", Description: "Order the realizations needed to move from surface questions to the core truth, with prerequisites, reversals, and the decision each revelation enables."},
			{ID: "clue-network", Title: "Clue network", Description: "Prepare multiple perceptible clues for every decisive inference, alternative access routes, clue combinations, and ways to relocate essential information without making one route mandatory."},
			{ID: "pressure-and-dread", Title: "Pressure and dread", Description: "Plan how danger, uncertainty, isolation, time pressure, cost, and moments of relief escalate without relying on arbitrary punishment or withholding all actionable information."},
			{ID: "suspects-and-threats", Title: "Suspects and threats", Description: "Give suspects, witnesses, factions, and hostile forces motives, vulnerabilities, countermoves, breaking points, and independent next actions."},
			{ID: "current-investigation-window", Title: "Current investigation window", Description: "Prepare the next few viable locations, conversations, tests, discoveries, and complications, including what each can reveal or put at risk."},
			{ID: "failure-forward", Title: "Failure-forward routes", Description: "Design costs, partial discoveries, escalating danger, and recovery routes so failed approaches change the situation without ending the investigation in a dead end."},
			{ID: "confrontations-and-aftermath", Title: "Confrontations and aftermath", Description: "Prepare several plausible confrontation, escape, exposure, containment, or survival outcomes and the evidence and choices required to earn them."},
		}),
		planningTemplate("episodic-emergent", "Episodic and emergent", "A light, adaptive plan for episodic adventures, infinite-flow structures, slice-of-life stories, and player-led open play.", []GamePlanningSection{
			{ID: "enduring-premise", Title: "Enduring premise", Description: "Define the repeatable story engine, emotional or thematic center, persistent questions, and continuity promises that make varied episodes feel like one story."},
			{ID: "season-momentum", Title: "Season momentum", Description: "Keep one or two loose medium-term movements that can accumulate across episodes without forcing every episode to serve a single main quest."},
			{ID: "active-opportunities", Title: "Active opportunities", Description: "Prepare a small, varied pool of optional goals, invitations, destinations, problems, and relationship moments with clear stakes and expiry or transformation conditions."},
			{ID: "next-episode-candidates", Title: "Next episode candidates", Description: "Detail only the next few likely episodes or scenarios enough to improvise: premise, active cast, opening pressure, possible turn, and reusable exit."},
			{ID: "cast-rotation", Title: "Cast rotation", Description: "Plan recurring-character returns, guest entrances, temporary exits, pairings, and spotlight rotation so the world feels inhabited without overcrowding every episode."},
			{ID: "world-clocks", Title: "World clocks", Description: "Advance a few background forces, recurring rivals, routines, or location changes independently, and choose signals that let the player notice their momentum."},
			{ID: "continuity-seeds", Title: "Continuity seeds", Description: "Plant lightweight callbacks, promises, relationship changes, mysteries, and objects that may grow into later episodes without turning every seed into an obligation."},
			{ID: "regrouping-rules", Title: "Regrouping and replanning", Description: "Define when to retire stale opportunities, promote an emergent thread into a main arc, return to the baseline rhythm, or rebuild the plan after a major player-led change."},
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
