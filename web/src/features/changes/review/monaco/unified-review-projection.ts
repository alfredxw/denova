import { diffLines, diffWordsWithSpace } from 'diff'

export type UnifiedReviewLineKind = 'unchanged' | 'added' | 'removed' | 'collapsed'

export interface UnifiedReviewWordDiff {
  startColumn: number
  endColumn: number
}

export interface UnifiedReviewLine {
  kind: UnifiedReviewLineKind
  content: string
  lineNumberLabel: string
  commentSide?: 'before' | 'after'
  sourceLineNumber?: number
  beforeLineNumber?: number
  afterLineNumber?: number
  wordDiffs?: UnifiedReviewWordDiff[]
  collapseID?: string
  collapsedLineCount?: number
}

export interface UnifiedReviewProjection {
  value: string
  lines: UnifiedReviewLine[]
}

interface BuildUnifiedReviewProjectionOptions {
  collapsedLabel: (count: number) => string
  expandedRegionIDs?: ReadonlySet<string>
  visibleSourceLines?: ReadonlySet<string>
  contextLineCount?: number
  minimumUnchangedLineCount?: number
}

const DEFAULT_CONTEXT_LINE_COUNT = 3
const DEFAULT_MINIMUM_UNCHANGED_LINE_COUNT = 8

/** Builds a stable, single-model unified diff so one gutter owns every visible line. */
export function buildUnifiedReviewProjection(
  before: string,
  after: string,
  options: BuildUnifiedReviewProjectionOptions,
): UnifiedReviewProjection {
  const rawLines = annotateWordDiffs(buildRawLines(before, after))
  const lines = collapseUnchangedLines(rawLines, {
    contextLineCount: options.contextLineCount ?? DEFAULT_CONTEXT_LINE_COUNT,
    minimumUnchangedLineCount: options.minimumUnchangedLineCount ?? DEFAULT_MINIMUM_UNCHANGED_LINE_COUNT,
    expandedRegionIDs: options.expandedRegionIDs ?? new Set(),
    visibleSourceLines: options.visibleSourceLines ?? new Set(),
    collapsedLabel: options.collapsedLabel,
  })
  return { value: lines.map((line) => line.content).join('\n'), lines }
}

function annotateWordDiffs(lines: UnifiedReviewLine[]): UnifiedReviewLine[] {
  let index = 0
  while (index < lines.length) {
    if (lines[index].kind === 'unchanged') {
      index += 1
      continue
    }
    const changedStart = index
    while (index < lines.length && lines[index].kind !== 'unchanged') index += 1
    const changedLines = lines.slice(changedStart, index)
    const removed = changedLines.filter((line) => line.kind === 'removed')
    const added = changedLines.filter((line) => line.kind === 'added')
    const pairCount = Math.min(removed.length, added.length)
    for (let pairIndex = 0; pairIndex < pairCount; pairIndex += 1) {
      const ranges = wordDiffRanges(removed[pairIndex].content, added[pairIndex].content)
      removed[pairIndex].wordDiffs = ranges.removed
      added[pairIndex].wordDiffs = ranges.added
    }
  }
  return lines
}

function wordDiffRanges(before: string, after: string): { removed: UnifiedReviewWordDiff[]; added: UnifiedReviewWordDiff[] } {
  const removed: UnifiedReviewWordDiff[] = []
  const added: UnifiedReviewWordDiff[] = []
  let beforeColumn = 1
  let afterColumn = 1
  for (const part of diffWordsWithSpace(before, after)) {
    const length = part.value.length
    if (part.removed) {
      if (length > 0) removed.push({ startColumn: beforeColumn, endColumn: beforeColumn + length })
      beforeColumn += length
    } else if (part.added) {
      if (length > 0) added.push({ startColumn: afterColumn, endColumn: afterColumn + length })
      afterColumn += length
    } else {
      beforeColumn += length
      afterColumn += length
    }
  }
  return { removed, added }
}

export function estimateUnifiedReviewLineCount(before: string, after: string): number {
  return buildUnifiedReviewProjection(before, after, { collapsedLabel: () => '' }).lines.length
}

function buildRawLines(before: string, after: string): UnifiedReviewLine[] {
  const result: UnifiedReviewLine[] = []
  let beforeLineNumber = 1
  let afterLineNumber = 1
  for (const part of diffLines(before, after)) {
    for (const content of logicalLines(part.value)) {
      if (part.removed) {
        result.push({
          kind: 'removed',
          content,
          lineNumberLabel: String(beforeLineNumber),
          commentSide: 'before',
          sourceLineNumber: beforeLineNumber,
          beforeLineNumber,
        })
        beforeLineNumber += 1
        continue
      }
      if (part.added) {
        result.push({
          kind: 'added',
          content,
          lineNumberLabel: String(afterLineNumber),
          commentSide: 'after',
          sourceLineNumber: afterLineNumber,
          afterLineNumber,
        })
        afterLineNumber += 1
        continue
      }
      result.push({
        kind: 'unchanged',
        content,
        lineNumberLabel: String(afterLineNumber),
        commentSide: 'after',
        sourceLineNumber: afterLineNumber,
        beforeLineNumber,
        afterLineNumber,
      })
      beforeLineNumber += 1
      afterLineNumber += 1
    }
  }
  return result
}

function collapseUnchangedLines(
  lines: UnifiedReviewLine[],
  options: {
    contextLineCount: number
    minimumUnchangedLineCount: number
    expandedRegionIDs: ReadonlySet<string>
    visibleSourceLines: ReadonlySet<string>
    collapsedLabel: (count: number) => string
  },
): UnifiedReviewLine[] {
  const result: UnifiedReviewLine[] = []
  let index = 0
  while (index < lines.length) {
    if (lines[index].kind !== 'unchanged') {
      result.push(lines[index])
      index += 1
      continue
    }
    const start = index
    while (index < lines.length && lines[index].kind === 'unchanged') index += 1
    const run = lines.slice(start, index)
    if (run.length < options.minimumUnchangedLineCount || run.some((line) => (
      (line.beforeLineNumber && options.visibleSourceLines.has(`before:${line.beforeLineNumber}`))
      || (line.afterLineNumber && options.visibleSourceLines.has(`after:${line.afterLineNumber}`))
    ))) {
      result.push(...run)
      continue
    }

    const leadingContext = start === 0 ? 0 : Math.min(options.contextLineCount, run.length)
    const trailingContext = index === lines.length
      ? 0
      : Math.min(options.contextLineCount, run.length - leadingContext)
    const hiddenCount = run.length - leadingContext - trailingContext
    if (hiddenCount <= 0) {
      result.push(...run)
      continue
    }

    const firstHidden = run[leadingContext]
    const collapseID = `${firstHidden.beforeLineNumber ?? 0}:${firstHidden.afterLineNumber ?? 0}:${hiddenCount}`
    if (options.expandedRegionIDs.has(collapseID)) {
      result.push(...run)
      continue
    }
    if (leadingContext > 0) result.push(...run.slice(0, leadingContext))
    result.push({
      kind: 'collapsed',
      content: options.collapsedLabel(hiddenCount),
      lineNumberLabel: '',
      collapseID,
      collapsedLineCount: hiddenCount,
    })
    if (trailingContext > 0) result.push(...run.slice(run.length - trailingContext))
  }
  return result
}

function logicalLines(value: string): string[] {
  if (!value) return []
  const lines = value.replaceAll('\r\n', '\n').split('\n')
  if (value.endsWith('\n')) lines.pop()
  return lines.map((line) => line.endsWith('\r') ? line.slice(0, -1) : line)
}
