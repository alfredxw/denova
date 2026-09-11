package agent

// projectCompactionCleanup applies the transient overflow cleanup to the
// same raw coordinates used by the summary source. Results produced by the
// active tool loop are outside that source and remain in the final snapshot.
func projectCompactionCleanup(
	contextMessages, visible, beforeMiddleware, raw []*Message,
	initialLoopMessages int,
	replacements []CleanupReplacement,
) ([]*Message, error) {
	targets, err := freezeCleanupTargets(visible, beforeMiddleware, raw, initialLoopMessages, replacements)
	if err != nil {
		return nil, err
	}
	plan := CleanupPlan{Action: CleanupProject}
	for _, replacement := range targets.raw {
		if replacement.MessageIndex < len(contextMessages) {
			plan.Replacements = append(plan.Replacements, replacement)
		}
	}
	return applyCleanupPlan(contextMessages, plan)
}
