import { describe, expect, it } from 'vitest'
import {
  customFontFamilyFromValue,
  customFontValue,
  fontStackFor,
  MAX_CUSTOM_FONT_FAMILY_LENGTH,
} from './font-options'

describe('font options', () => {
  it('round-trips one trimmed custom font family', () => {
    const value = customFontValue('  Microsoft YaHei  ')
    expect(value).toBe('custom:Microsoft YaHei')
    expect(customFontFamilyFromValue(value)).toBe('Microsoft YaHei')
  })

  it('quotes custom font names and preserves the selected fallback stack', () => {
    const value = customFontValue('Writer "Display" \\ Serif')
    expect(fontStackFor(value, 'apple-system')).toBe('"Writer \\"Display\\" \\\\ Serif", "SF Pro Text", "SF Pro Display", "PingFang SC", "Hiragino Sans GB", -apple-system, BlinkMacSystemFont, sans-serif')
  })

  it('rejects empty, control-character and oversized custom font names', () => {
    expect(customFontValue('  ')).toBeNull()
    expect(customFontValue('Broken\nFont')).toBeNull()
    expect(customFontValue('x'.repeat(MAX_CUSTOM_FONT_FAMILY_LENGTH + 1))).toBeNull()
    expect(fontStackFor('custom:Broken\nFont', 'apple-system')).toContain('SF Pro Text')
  })
})
