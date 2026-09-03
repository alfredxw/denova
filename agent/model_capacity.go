package agent

// CapacityAwareTokenReserve returns the smallest planning reserve that keeps a
// configured response cap feasible by the maintenance trigger. Context space
// already left above that trigger is counted once instead of reserving the
// entire response cap again.
func CapacityAwareTokenReserve(baseReserve, maxOutputTokens, contextWindowTokens int, triggerRatio float64) int {
	baseReserve = max(0, baseReserve)
	if maxOutputTokens <= 0 || contextWindowTokens <= 0 || triggerRatio <= 0 || triggerRatio >= 1 {
		return baseReserve
	}
	triggerTokens := int(float64(contextWindowTokens) * triggerRatio)
	triggerHeadroom := max(0, contextWindowTokens-triggerTokens)
	return max(baseReserve, max(0, maxOutputTokens-triggerHeadroom))
}
