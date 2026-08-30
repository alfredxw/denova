export function stateEntries(state?: Record<string, unknown>) {
  if (!state) return []
  return Object.entries(state).filter(([, value]) => value !== undefined && value !== null)
}
