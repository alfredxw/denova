import { describe, expect, it } from 'vitest'
import { lineDiffStats } from './diff-stats'

describe('lineDiffStats', () => {
  it('counts Chinese changed and appended lines with trailing newlines', () => {
    expect(lineDiffStats('中文\n旧句\n', '中文\n新句\n增加\n')).toEqual({ additions: 2, deletions: 1 })
  })

  it('counts a final unterminated line once', () => {
    expect(lineDiffStats('', '没有末尾换行')).toEqual({ additions: 1, deletions: 0 })
    expect(lineDiffStats('删除我', '')).toEqual({ additions: 0, deletions: 1 })
  })
})
