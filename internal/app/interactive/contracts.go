package interactiveapp

const (
	// DirectorTaskPlanUpdate refreshes the branch plan after a committed turn.
	DirectorTaskPlanUpdate = "director_plan_update"
	// DirectorTaskOpeningPlan initializes the plan before the first turn.
	DirectorTaskOpeningPlan = "opening_plan"
	// DirectorOpeningSourceID identifies the synthetic pre-opening maintenance source.
	DirectorOpeningSourceID = "story_opening"
)
