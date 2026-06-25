const NARRATIVE_START = '<NARRATIVE>'
const NARRATIVE_END = '</NARRATIVE>'
const THINK_START = '<think>'
const THINK_END = '</think>'

const VISIBLE_TAGS = [NARRATIVE_START, NARRATIVE_END]
const THINK_TAGS = [THINK_START, THINK_END]
const HIDDEN_TAG_PREFIXES = ['<hot_state', '<state_delta']
const TAG_PREFIXES = [
  ...VISIBLE_TAGS.map((tag) => tag.toLowerCase()),
  ...THINK_TAGS.map((tag) => tag.toLowerCase()),
  ...HIDDEN_TAG_PREFIXES,
]

export interface NarrativeChunk {
  /** 本次新增的可见正文 */
  text: string
  /** 为 true 时，调用方应丢弃此前已显示的流式正文：之前输出的其实是思考前言。 */
  reset: boolean
}

/**
 * 互动叙事流式过滤器：只放行 <NARRATIVE> 内的正文，隐藏思考、状态、热状态。
 *
 * 兼容部分 provider 模型的「思考前言无 <think> 开始标签、仅以 </think> 收尾」的输出：
 * 见到 <think> 或孤立 </think> 时，会把此前误显示的前言通过 reset 信号清除。
 */
export function createInteractiveNarrativeFilter() {
  let buffer = ''
  let stopped = false
  let inThink = false
  let emittedVisible = false

  return {
    push(chunk: string): NarrativeChunk {
      if (!chunk || stopped) return { text: '', reset: false }
      buffer += chunk
      return drain(false)
    },
    flush(): NarrativeChunk {
      if (stopped) return { text: '', reset: false }
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
        const end = indexOfFold(buffer, THINK_END)
        if (end >= 0) {
          buffer = trimStart(buffer.slice(end + THINK_END.length))
          inThink = false
          continue
        }
        // 未见到结束标签：丢弃整段思考，仅保留可能被截断的尾部标签前缀。
        const keep = flushAll ? 0 : partialTagSuffixLength(buffer)
        buffer = buffer.slice(buffer.length - keep)
        return { text: output, reset }
      }

      if (startsWithHiddenTag(buffer)) {
        stopped = true
        buffer = ''
        return { text: output, reset }
      }

      if (startsWithFold(buffer, THINK_START)) {
        buffer = buffer.slice(THINK_START.length)
        inThink = true
        requestReset()
        continue
      }
      if (startsWithFold(buffer, THINK_END)) {
        // 思考前言无 <think> 开始标签，到这里才闭合。
        buffer = trimStart(buffer.slice(THINK_END.length))
        requestReset()
        continue
      }
      if (startsWithFold(buffer, NARRATIVE_START)) {
        buffer = trimStart(buffer.slice(NARRATIVE_START.length))
        continue
      }
      if (startsWithFold(buffer, NARRATIVE_END)) {
        buffer = trimStart(buffer.slice(NARRATIVE_END.length))
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
 * 清洗已持久化的叙事正文，兜底历史脏数据（思考前言、</think>、<NARRATIVE> 等标签残留）。
 * 用于渲染存档 turn.narrative，与流式过滤器对齐。
 */
export function sanitizeStoredNarrative(text: string): string {
  if (!text) return text
  let result = text
  const open = result.search(/<\s*NARRATIVE\s*>/i)
  if (open >= 0) {
    // 有 <NARRATIVE> 包裹时直接取其内容，自然丢弃前面的思考前言。
    result = result.slice(open).replace(/<\s*NARRATIVE\s*>/i, '')
    const close = result.search(/<\s*\/\s*NARRATIVE\s*>/i)
    if (close >= 0) result = result.slice(0, close)
  } else {
    // 无 <NARRATIVE>：移除配对 / 未闭合 <think>，再处理孤立 </think> 前言。
    result = result.replace(/<think>[\s\S]*?(?:<\/think>|$)/gi, '')
    const close = result.search(/<\s*\/\s*think\s*>/i)
    if (close >= 0) result = result.slice(close).replace(/<\s*\/\s*think\s*>/i, '')
  }
  // 截断隐藏状态块及之后的内容。
  result = result.replace(/<\s*(hot_state|state_delta)[\s\S]*$/i, '')
  // 清理任何残留标签。
  result = result.replace(/<\/?\s*(think|NARRATIVE)\s*>/gi, '')
  return result.trim()
}

function findNextTag(value: string): number {
  const lower = value.toLowerCase()
  let next = -1
  for (const tag of [...VISIBLE_TAGS, ...THINK_TAGS]) {
    const index = lower.indexOf(tag.toLowerCase())
    if (index >= 0 && (next < 0 || index < next)) next = index
  }
  const hiddenIndex = findHiddenTagIndex(lower)
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

function startsWithFold(value: string, prefix: string): boolean {
  return value.slice(0, prefix.length).toLowerCase() === prefix.toLowerCase()
}

function indexOfFold(value: string, sub: string): number {
  return value.toLowerCase().indexOf(sub.toLowerCase())
}

function trimStart(value: string): string {
  return value.replace(/^\s+/, '')
}
