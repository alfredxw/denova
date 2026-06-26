export interface DialogueHighlightRange {
  from: number
  to: number
}

const QUOTED_DIALOGUE_PATTERN = /("([^"\n]+)"|“([^”\n]+)”|「([^」\n]+)」)/g

export function findDialogueHighlightRanges(text: string): DialogueHighlightRange[] {
  if (!text) return []

  const ranges: DialogueHighlightRange[] = []
  let match: RegExpExecArray | null
  QUOTED_DIALOGUE_PATTERN.lastIndex = 0
  while ((match = QUOTED_DIALOGUE_PATTERN.exec(text)) !== null) {
    ranges.push({ from: match.index, to: QUOTED_DIALOGUE_PATTERN.lastIndex })
  }

  return ranges
}
