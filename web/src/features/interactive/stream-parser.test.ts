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
  it('streams bare narrative across chunks before hidden state', () => {
    const visible = collect([
      '火光照亮了',
      '墙上的新线索。',
      '\n<STATE',
      '_DELTA>{"ops":[{"op":"set","path":"on_stage","value":["林川"]}]}',
    ])
    expect(visible).toBe('火光照亮了墙上的新线索。\n')
  })

  it('still removes legacy narrative tags and hides state delta', () => {
    const visible = collect([
      '<NARRATIVE>\n火光照亮了',
      '墙上的新线索。\n</NARRATIVE>\n<STATE_DELTA>',
      '{"ops":[{"op":"set","path":"on_stage","value":["林川"]}]}',
    ])
    expect(visible).toBe('火光照亮了墙上的新线索。\n')
  })

  it('hides hot state choices after narrative', () => {
    const visible = collect([
      '<NARRATIVE>门后传来风声。</NARRATIVE><HOT',
      '_STATE>{"choices":["我贴近门缝听里面的动静。"]}</HOT_STATE>',
    ])
    expect(visible).toBe('门后传来风声。')
  })

  it('hides lowercase or spaced hot state tags', () => {
    const visible = collect([
      '<NARRATIVE>门后传来风声。</NARRATIVE>\n< hot',
      '_state>{"choices":["我贴近门缝听里面的动静。"]}</hot_state>',
    ])
    expect(visible).toBe('门后传来风声。')
  })

  it('handles tags split across chunks', () => {
    const visible = collect([
      '<NARR',
      'ATIVE>门后传来低沉',
      '的风声。</NARR',
      'ATIVE><STATE',
      '_DELTA>{"ops":[]}',
    ])
    expect(visible).toBe('门后传来低沉的风声。')
  })

  it('passes bare narrative before state delta', () => {
    const visible = collect(['新格式裸正文', '<STATE_DELTA>{"ops":[]}'])
    expect(visible).toBe('新格式裸正文')
  })

  it('strips reasoning prelude with orphan </think> tag', () => {
    // 部分 provider：思考前言无 <think> 开始标签，仅以 </think> 收尾，随后才是 <NARRATIVE>。
    const visible = collect([
      'tags\n\nSince this is a new story.',
      'Let me write the opening:</think>\n\n<NARRATIVE>\n意识像被冷水浇醒。',
      '\n陆沉猛地睁开眼。</NARRATIVE><STATE_DELTA>{"ops":[]}',
    ])
    expect(visible).toBe('意识像被冷水浇醒。\n陆沉猛地睁开眼。')
  })

  it('strips paired <think> reasoning before narrative', () => {
    const visible = collect([
      '<think>让我想想怎么写</think>',
      '<NARRATIVE>夜色降临。</NARRATIVE>',
    ])
    expect(visible).toBe('夜色降临。')
  })

  it('reset flag fires when reasoning prelude precedes narrative', () => {
    const filter = createInteractiveNarrativeFilter()
    const r1 = filter.push('thinking aloud here')
    const r2 = filter.push('</think><NARRATIVE>正文</NARRATIVE>')
    expect(r1.reset).toBe(false)
    expect(r2.reset).toBe(true)
  })
})

describe('sanitizeStoredNarrative', () => {
  it('extracts narrative body from leaked storage with orphan </think>', () => {
    const leaked = 'tags\n\nSince this is a new story.\nLet me write:</think>\n\n<NARRATIVE>\n意识像被冷水浇醒。\n陆沉睁开眼。'
    expect(sanitizeStoredNarrative(leaked)).toBe('意识像被冷水浇醒。\n陆沉睁开眼。')
  })

  it('strips orphan </think> prelude without narrative tags', () => {
    expect(sanitizeStoredNarrative('内心独白</think>\n真正的正文。')).toBe('真正的正文。')
  })

  it('keeps clean narrative unchanged', () => {
    expect(sanitizeStoredNarrative('门后传来风声。')).toBe('门后传来风声。')
  })

  it('drops trailing hidden state block', () => {
    expect(sanitizeStoredNarrative('正文。<STATE_DELTA>{"ops":[]}</STATE_DELTA>')).toBe('正文。')
  })
})
