import { diffChars, diffWords, type Change } from 'diff'

export interface InlineEditDiffPart {
  value: string
  kind: 'equal' | 'added' | 'removed'
}

const segmenterCache = new Map<string, Intl.Segmenter>()

function resolveWordSegmenter(locale: string): Intl.Segmenter | null {
  if (typeof Intl === 'undefined' || typeof Intl.Segmenter !== 'function') return null
  const normalized = locale.trim() || 'zh'
  const cached = segmenterCache.get(normalized)
  if (cached) return cached
  try {
    const segmenter = new Intl.Segmenter(normalized, { granularity: 'word' })
    segmenterCache.set(normalized, segmenter)
    return segmenter
  } catch {
    return null
  }
}

/**
 * Computes word-level prose diff with CJK-aware Intl.Segmenter tokenization.
 * Without Intl.Segmenter, jsdiff's default regex treats an entire Chinese clause
 * without spaces as one token, producing unreadable all-red/all-green diffs.
 */
export function computeInlineEditDiff(
  original: string,
  proposed: string,
  locale = 'zh',
  segmenterOverride?: Intl.Segmenter | null,
): InlineEditDiffPart[] {
  if (original === proposed) {
    return original ? [{ value: original, kind: 'equal' }] : []
  }
  if (!original) {
    return proposed ? [{ value: proposed, kind: 'added' }] : []
  }
  if (!proposed) {
    return [{ value: original, kind: 'removed' }]
  }

  const segmenter = segmenterOverride !== undefined
    ? segmenterOverride
    : resolveWordSegmenter(locale)

  let changes: Change[]
  if (segmenter) {
    changes = diffWords(original, proposed, { intlSegmenter: segmenter })
  } else {
    changes = diffChars(original, proposed)
  }

  const result: InlineEditDiffPart[] = []
  for (const change of changes) {
    if (!change.value) continue
    const kind: InlineEditDiffPart['kind'] = change.added
      ? 'added'
      : change.removed
        ? 'removed'
        : 'equal'
    const previous = result[result.length - 1]
    if (previous && previous.kind === kind) {
      previous.value += change.value
    } else {
      result.push({ value: change.value, kind })
    }
  }
  return result
}
