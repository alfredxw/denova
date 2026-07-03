import { describe, expect, it } from 'vitest'
import { createInteractiveNarrativeFilter, sanitizeStoredNarrative } from './stream-parser'

/** 模拟 StoryStage 的累积逻辑：reset 时清空已显示正文，否则追加。 */
function collect(chunks: string[]): string {
  const filter = createInteractiveNarrativeFilter()
  let shown = ''
  for (const chunk of chunks) {
    const { text, reset } = filter.push(chunk)
    if (reset) shown = ''
    shown += text
  }
  const { text, reset } = filter.flush()
  if (reset) shown = ''
  shown += text
  return shown
}

describe('createInteractiveNarrativeFilter', () => {
  it('streams bare narrative across chunks', () => {
    const visible = collect([
      '火光照亮了',
      '墙上的新线索。',
    ])
    expect(visible).toBe('火光照亮了墙上的新线索。')
  })

  it('strips reasoning prelude with orphan </think> tag', () => {
    // 部分 provider：思考前言无 <think> 开始标签，仅以 </think> 收尾，随后才是正文。
    const visible = collect([
      'tags\n\nSince this is a new story.',
      'Let me write the opening:</think>\n\n意识像被冷水浇醒。',
      '\n陆沉猛地睁开眼。',
    ])
    expect(visible).toBe('意识像被冷水浇醒。\n陆沉猛地睁开眼。')
  })

  it('strips paired <think> reasoning before narrative', () => {
    const visible = collect([
      '<think>让我想想怎么写</think>',
      '夜色降临。',
    ])
    expect(visible).toBe('夜色降临。')
  })

  it('reset flag fires when reasoning prelude precedes narrative', () => {
    const filter = createInteractiveNarrativeFilter()
    const r1 = filter.push('thinking aloud here')
    const r2 = filter.push('</think>正文')
    expect(r1.reset).toBe(false)
    expect(r2.reset).toBe(true)
  })
})

describe('sanitizeStoredNarrative', () => {
  it('extracts narrative body from leaked storage with orphan </think>', () => {
    const leaked = 'tags\n\nSince this is a new story.\nLet me write:</think>\n\n意识像被冷水浇醒。\n陆沉睁开眼。'
    expect(sanitizeStoredNarrative(leaked)).toBe('意识像被冷水浇醒。\n陆沉睁开眼。')
  })

  it('strips orphan </think> prelude without narrative tags', () => {
    expect(sanitizeStoredNarrative('内心独白</think>\n真正的正文。')).toBe('真正的正文。')
  })

  it('keeps clean narrative unchanged', () => {
    expect(sanitizeStoredNarrative('门后传来风声。')).toBe('门后传来风声。')
  })
})
