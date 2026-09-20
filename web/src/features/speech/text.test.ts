import { describe, expect, it } from 'vitest'
import { speechChunks, speechText } from './text'

describe('speech text selection', () => {
  it.each([
    ['她说：“你好。”', 'all', false, '她说：“你好。”'],
    ['她说：“你好。”', 'quoted', false, '你好。'],
    ['“你说『明天』？”', 'quoted', false, '你说明天？'],
    ['*她低声说“别动。”* “好。”', 'quoted', false, '别动。\n\n好。'],
    ['*她低声说“别动。”* “好。”', 'quoted', true, '好。'],
    ['“好。”她又说：“还没说完', 'quoted', false, '好。'],
    ['**门开了。** *她走进去。*', 'all', true, '门开了。'],
    ['没有对白。', 'quoted', false, ''],
    ['\'single\' apostrophes aren\'t dialogue', 'quoted', false, ''],
    ['「第一行\n第二行」和 "最后一句"', 'quoted', false, '第一行\n第二行\n\n最后一句'],
    ['*未闭合的动作 “仍有对白。”', 'quoted', true, '仍有对白。'],
    ['* 她说“动作内对白。” * “好。”', 'quoted', true, '好。'],
  ] as const)('%s, %s, ignore=%s', (text, mode, ignore_asterisks, expected) => {
    expect(speechText(text, { mode, ignore_asterisks })).toBe(expected)
  })
  it('keeps visible labels but excludes code, media and URLs', () => {
    const text = '# 标题\n\n[链接文字](https://example.com)\n\n![图片](cover.png)\n\n```js\n"不读代码"\n```\n\n<audio src="voice.mp3">不要读</audio>\n\n正文 https://example.com/file.mp3\n\n<b>加粗</b>'
    expect(speechText(text, { mode: 'all', ignore_asterisks: false })).toBe('标题\n链接文字\n正文 \n加粗')
  })
  it('chunks over ten thousand characters without loss, duplication or broken Unicode', () => {
    const text = '很长的故事。😀'.repeat(1500)
    const chunks = speechChunks(text)
    expect(chunks.length).toBeGreaterThan(10)
    expect(chunks.join('')).toBe(text)
    expect(chunks.every(chunk => Array.from(chunk).length <= 1000 && !/^[\uDC00-\uDFFF]|[\uD800-\uDBFF]$/.test(chunk))).toBe(true)
  })
})
