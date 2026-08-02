package task

const defaultRegistryReplayByteLimit = 64 << 20

// EffectiveRegistryReplayByteLimit applies the shared bounded display replay
// budget used by product-level task identity registries.
func EffectiveRegistryReplayByteLimit(configured int) int {
	if configured > 0 {
		return configured
	}
	return defaultRegistryReplayByteLimit
}

// TouchReplayKey maintains least-recently-used identity order.
func TouchReplayKey(order []string, key string) []string {
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

// RemoveReplayKey removes one ordered identity without retaining its string.
func RemoveReplayKey(order []string, index int) []string {
	copy(order[index:], order[index+1:])
	order[len(order)-1] = ""
	return order[:len(order)-1]
}
