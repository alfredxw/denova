import { describe, expect, it } from 'vitest'
import { createInteractiveNarrativeFilter } from './stream-parser'

describe('createInteractiveNarrativeFilter', () => {
  it('removes narrative tags and hides state delta', () => {
    const filter = createInteractiveNarrativeFilter()
    const visible = [
      filter.push('<NARRATIVE>\n火光照亮了'),
      filter.push('墙上的新线索。\n</NARRATIVE>\n<STATE_DELTA>'),
      filter.push('{"ops":[{"op":"set","path":"on_stage","value":["林川"]}]}'),
      filter.flush(),
    ].join('')

    expect(visible).toBe('\n火光照亮了墙上的新线索。\n')
  })

  it('hides hot state choices after narrative', () => {
    const filter = createInteractiveNarrativeFilter()
    const visible = [
      filter.push('<NARRATIVE>门后传来风声。</NARRATIVE><HOT'),
      filter.push('_STATE>{"choices":["我贴近门缝听里面的动静。"]}</HOT_STATE>'),
      filter.flush(),
    ].join('')

    expect(visible).toBe('门后传来风声。')
  })

  it('hides lowercase or spaced hot state tags', () => {
    const filter = createInteractiveNarrativeFilter()
    const visible = [
      filter.push('<NARRATIVE>门后传来风声。</NARRATIVE>\n< hot'),
      filter.push('_state>{"choices":["我贴近门缝听里面的动静。"]}</hot_state>'),
      filter.flush(),
    ].join('')

    expect(visible).toBe('门后传来风声。')
  })

  it('handles tags split across chunks', () => {
    const filter = createInteractiveNarrativeFilter()
    const visible = [
      filter.push('<NARR'),
      filter.push('ATIVE>门后传来低沉'),
      filter.push('的风声。</NARR'),
      filter.push('ATIVE><STATE'),
      filter.push('_DELTA>{"ops":[]}'),
      filter.flush(),
    ].join('')

    expect(visible).toBe('门后传来低沉的风声。')
  })

  it('passes legacy narrative before state delta', () => {
    const filter = createInteractiveNarrativeFilter()
    const visible = [
      filter.push('旧格式正文'),
      filter.push('<STATE_DELTA>{"ops":[]}'),
      filter.flush(),
    ].join('')

    expect(visible).toBe('旧格式正文')
  })
})
