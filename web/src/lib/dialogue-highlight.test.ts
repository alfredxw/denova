import { describe, expect, it } from 'vitest'
import { findDialogueHighlightRanges } from './dialogue-highlight'

function highlightedText(text: string) {
  return findDialogueHighlightRanges(text).map((range) => text.slice(range.from, range.to))
}

describe('findDialogueHighlightRanges', () => {
  it('识别常见引号对白', () => {
    expect(highlightedText('他说：“走吧。” 她答：「等等。」')).toEqual(['“走吧。”', '「等等。」'])
  })

  it('不按角色名冒号识别对白，避免误判叙述句', () => {
    expect(highlightedText('林晚：我们走。\nJohn: wait here.\n他说：“走吧。”')).toEqual(['“走吧。”'])
  })
})
