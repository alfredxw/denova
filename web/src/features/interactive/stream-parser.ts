const TAG_PREFIXES = [
  '<think',
  '</think',
]

export interface NarrativeChunk {
  /** 本次新增的可见正文 */
  text: string
  /** 为 true 时，调用方应丢弃此前已显示的流式正文：之前输出的其实是思考前言。 */
  reset: boolean
}

/**
 * 互动叙事流式过滤器：默认放行裸正文，隐藏思考。
 *
 * 兼容部分 provider 模型的「思考前言无 <think> 开始标签、仅以 </think> 收尾」的输出：
 * 见到 <think> 或孤立 </think> 时，会把此前误显示的前言通过 reset 信号清除。
 */
export function createInteractiveNarrativeFilter() {
  let buffer = ''
  let inThink = false
  let emittedVisible = false

  return {
    push(chunk: string): NarrativeChunk {
      if (!chunk) return { text: '', reset: false }
      buffer += chunk
      return drain(false)
    },
    flush(): NarrativeChunk {
      return drain(true)
    },
  }

  function drain(flushAll: boolean): NarrativeChunk {
    let output = ''
    let reset = false

    // 思考开始（含无开始标签的孤立 </think>）意味着此前显示的都是思考前言，需要清除。
    const requestReset = () => {
      if (emittedVisible || output) {
        reset = true
        output = ''
        emittedVisible = false
      }
    }

    while (buffer) {
      if (inThink) {
        const end = findTag(buffer, 'think', true)
        if (end) {
          buffer = trimStart(buffer.slice(end.index + end.length))
          inThink = false
          continue
        }
        // 未见到结束标签：丢弃整段思考，仅保留可能被截断的尾部标签前缀。
        const keep = flushAll ? 0 : partialTagSuffixLength(buffer)
        buffer = buffer.slice(buffer.length - keep)
        return { text: output, reset }
      }

      const thinkStart = matchTagAtStart(buffer, 'think', false)
      if (thinkStart) {
        buffer = buffer.slice(thinkStart.length)
        inThink = true
        requestReset()
        continue
      }
      const thinkEnd = matchTagAtStart(buffer, 'think', true)
      if (thinkEnd) {
        // 思考前言无 <think> 开始标签，到这里才闭合。
        buffer = trimStart(buffer.slice(thinkEnd.length))
        requestReset()
        continue
      }

      const nextTag = findNextTag(buffer)
      if (nextTag > 0) {
        output += buffer.slice(0, nextTag)
        emittedVisible = true
        buffer = buffer.slice(nextTag)
        continue
      }
      if (nextTag === 0) continue

      const keep = flushAll ? 0 : partialTagSuffixLength(buffer)
      const emit = buffer.slice(0, buffer.length - keep)
      if (emit) {
        output += emit
        emittedVisible = true
      }
      buffer = buffer.slice(buffer.length - keep)
      return { text: output, reset }
    }
    return { text: output, reset }
  }
}

/**
 * 清洗已持久化的叙事正文，兜底思考前言残留。
 * 用于渲染存档 turn.narrative，与流式过滤器对齐。
 */
export function sanitizeStoredNarrative(text: string): string {
  if (!text) return text
  let result = text
  result = result.replace(/<think>[\s\S]*?(?:<\/think>|$)/gi, '')
  const close = result.search(/<\s*\/\s*think\s*>/i)
  if (close >= 0) result = result.slice(close).replace(/<\s*\/\s*think\s*>/i, '')
  result = result.replace(/<\/?\s*think\s*>/gi, '')
  return result.trim()
}

function findNextTag(value: string): number {
  let next = -1
  for (const closing of [false, true]) {
    const match = findTag(value, 'think', closing)
    if (match && (next < 0 || match.index < next)) next = match.index
  }
  return next
}

function partialTagSuffixLength(value: string): number {
  const lowerValue = value.toLowerCase()
  const max = Math.min(value.length, Math.max(...TAG_PREFIXES.map((tag) => tag.length)) + 4)
  for (let length = max; length > 0; length--) {
    const suffix = normalizeTagStart(lowerValue.slice(lowerValue.length - length))
    if (TAG_PREFIXES.some((tag) => tag.startsWith(suffix))) return length
  }
  return 0
}

function normalizeTagStart(value: string): string {
  if (!value.startsWith('<')) return value
  return value.replace(/^<\s*\/\s*/, '</').replace(/^<\s*/, '<').replace(/\s+$/, '')
}

function matchTagAtStart(value: string, name: string, closing: boolean): { length: number } | null {
  const slash = closing ? String.raw`\/\s*` : ''
  const match = new RegExp(String.raw`^<\s*${slash}${name}\s*>`, 'i').exec(value)
  return match ? { length: match[0].length } : null
}

function findTag(value: string, name: string, closing: boolean): { index: number; length: number } | null {
  const slash = closing ? String.raw`\/\s*` : ''
  const match = new RegExp(String.raw`<\s*${slash}${name}\s*>`, 'i').exec(value)
  return match ? { index: match.index, length: match[0].length } : null
}

function trimStart(value: string): string {
  return value.replace(/^\s+/, '')
}
