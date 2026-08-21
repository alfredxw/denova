export function nextProfileIDAfterRemoval<T>(
  profiles: T[],
  removedIndex: number,
  profileID: (profile: T) => string,
): string {
  for (let index = removedIndex + 1; index < profiles.length; index += 1) {
    const id = profileID(profiles[index])
    if (id) return id
  }
  for (let index = removedIndex - 1; index >= 0; index -= 1) {
    const id = profileID(profiles[index])
    if (id) return id
  }
  return ''
}
