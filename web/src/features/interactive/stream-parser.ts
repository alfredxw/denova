const NARRATIVE_START = '<NARRATIVE>'
const NARRATIVE_END = '</NARRATIVE>'

const VISIBLE_TAGS = [NARRATIVE_START, NARRATIVE_END]
const HIDDEN_TAG_PREFIXES = ['<hot_state', '<state_delta']
const TAG_PREFIXES = [...VISIBLE_TAGS.map((tag) => tag.toLowerCase()), ...HIDDEN_TAG_PREFIXES]

export function createInteractiveNarrativeFilter() {
  let buffer = ''
  let stopped = false

  return {
    push(chunk: string): string {
      if (!chunk || stopped) return ''
      buffer += chunk
      return drain(false)
    },
    flush(): string {
      if (stopped) return ''
      return drain(true)
    },
  }

  function drain(flushAll: boolean): string {
    let output = ''
    while (buffer) {
      if (startsWithHiddenTag(buffer)) {
        stopped = true
        buffer = ''
        return output
      }
      if (buffer.startsWith(NARRATIVE_START)) {
        buffer = buffer.slice(NARRATIVE_START.length)
        continue
      }
      if (buffer.startsWith(NARRATIVE_END)) {
        buffer = buffer.slice(NARRATIVE_END.length)
        buffer = buffer.trimStart()
        continue
      }

      const nextTag = findNextTag(buffer)
      if (nextTag > 0) {
        output += buffer.slice(0, nextTag)
        buffer = buffer.slice(nextTag)
        continue
      }
      if (nextTag === 0) continue

      const keep = flushAll ? 0 : partialTagSuffixLength(buffer)
      output += buffer.slice(0, buffer.length - keep)
      buffer = buffer.slice(buffer.length - keep)
      return output
    }
    return output
  }
}

function findNextTag(value: string): number {
  let next = -1
  for (const tag of VISIBLE_TAGS) {
    const index = value.indexOf(tag)
    if (index >= 0 && (next < 0 || index < next)) next = index
  }
  const lowerValue = value.toLowerCase()
  const hiddenIndex = findHiddenTagIndex(lowerValue)
  if (hiddenIndex >= 0 && (next < 0 || hiddenIndex < next)) next = hiddenIndex
  return next
}

function partialTagSuffixLength(value: string): number {
  const lowerValue = value.toLowerCase()
  const max = Math.min(value.length, Math.max(...TAG_PREFIXES.map((tag) => tag.length)) - 1)
  for (let length = max; length > 0; length--) {
    const suffix = normalizeTagStart(lowerValue.slice(lowerValue.length - length))
    if (TAG_PREFIXES.some((tag) => tag.startsWith(suffix))) return length
  }
  return 0
}

function startsWithHiddenTag(value: string): boolean {
  const normalized = normalizeTagStart(value.toLowerCase())
  return HIDDEN_TAG_PREFIXES.some((tag) => normalized.startsWith(tag))
}

function findHiddenTagIndex(value: string): number {
  const match = /<\s*(hot_state|state_delta)/i.exec(value)
  return match?.index ?? -1
}

function normalizeTagStart(value: string): string {
  return value.replace(/^<\s*/, '<')
}
