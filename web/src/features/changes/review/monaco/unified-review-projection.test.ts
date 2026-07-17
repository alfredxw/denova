import { describe, expect, it } from 'vitest'
import { buildUnifiedReviewProjection } from './unified-review-projection'

describe('buildUnifiedReviewProjection', () => {
  it('creates one visual line-number column with a commentable source for every rendered content line', () => {
    const projection = buildUnifiedReviewProjection(
      'same\nold\nend\n',
      'same\nnew\nend\n',
      { collapsedLabel: (count) => `${count} unmodified lines` },
    )

    expect(projection.lines.map((line) => ({
      kind: line.kind,
      label: line.lineNumberLabel,
      side: line.commentSide,
      sourceLine: line.sourceLineNumber,
    }))).toEqual([
      { kind: 'unchanged', label: '1', side: 'after', sourceLine: 1 },
      { kind: 'removed', label: '2', side: 'before', sourceLine: 2 },
      { kind: 'added', label: '2', side: 'after', sourceLine: 2 },
      { kind: 'unchanged', label: '3', side: 'after', sourceLine: 3 },
    ])
  })

  it('keeps word-level ranges for paired removed and added lines', () => {
    const projection = buildUnifiedReviewProjection(
      'const answer = oldValue\n',
      'const answer = newValue\n',
      { collapsedLabel: (count) => `${count} unmodified lines` },
    )

    const removed = projection.lines.find((line) => line.kind === 'removed')
    const added = projection.lines.find((line) => line.kind === 'added')
    expect(removed?.wordDiffs?.map((range) => removed.content.slice(range.startColumn - 1, range.endColumn - 1))).toEqual(['oldValue'])
    expect(added?.wordDiffs?.map((range) => added.content.slice(range.startColumn - 1, range.endColumn - 1))).toEqual(['newValue'])
  })

  it('collapses long unchanged ranges without losing the source rows needed to expand them', () => {
    const unchanged = Array.from({ length: 12 }, (_, index) => `line ${index + 1}`)
    const projection = buildUnifiedReviewProjection(
      [...unchanged, 'old'].join('\n'),
      [...unchanged, 'new'].join('\n'),
      { collapsedLabel: (count) => `${count} unmodified lines` },
    )

    expect(projection.lines[0]).toMatchObject({
      kind: 'collapsed',
      content: '9 unmodified lines',
      collapsedLineCount: 9,
    })
    expect(projection.lines.slice(1, 4).map((line) => line.lineNumberLabel)).toEqual(['10', '11', '12'])

    const collapseID = projection.lines[0].collapseID
    expect(collapseID).toBeTruthy()
    const expanded = buildUnifiedReviewProjection(
      [...unchanged, 'old'].join('\n'),
      [...unchanged, 'new'].join('\n'),
      {
        collapsedLabel: (count) => `${count} unmodified lines`,
        expandedRegionIDs: new Set([collapseID as string]),
      },
    )
    expect(expanded.lines).toHaveLength(14)
    expect(expanded.lines.some((line) => line.kind === 'collapsed')).toBe(false)
  })
})
