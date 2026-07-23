package app

const defaultTaskRegistryReplayByteLimit = 64 << 20

func effectiveTaskRegistryReplayByteLimit(configured int) int {
	if configured > 0 {
		return configured
	}
	return defaultTaskRegistryReplayByteLimit
}

// touchTaskReplayKey maintains least-recently-used order without exposing
// registry-specific fingerprints or receipts to the display-retention module.
func touchTaskReplayKey(order []string, key string) []string {
	for index, candidate := range order {
		if candidate != key {
			continue
		}
		copy(order[index:], order[index+1:])
		order[len(order)-1] = key
		return order
	}
	return append(order, key)
}

func removeTaskReplayKey(order []string, index int) []string {
	copy(order[index:], order[index+1:])
	order[len(order)-1] = ""
	return order[:len(order)-1]
}
