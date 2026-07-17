import { diffLines } from 'diff'

export interface LineDiffStats {
  additions: number
  deletions: number
}

/** Counts logical lines changed between two server-provided snapshots. */
export function lineDiffStats(before: string, after: string): LineDiffStats {
  let additions = 0
  let deletions = 0
  for (const part of diffLines(before, after)) {
    if (part.added) additions += part.count ?? logicalLineCount(part.value)
    if (part.removed) deletions += part.count ?? logicalLineCount(part.value)
  }
  return { additions, deletions }
}

function logicalLineCount(value: string): number {
  if (!value) return 0
  const count = value.split('\n').length
  return value.endsWith('\n') ? count - 1 : count
}
