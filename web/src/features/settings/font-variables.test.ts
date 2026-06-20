import { afterEach, describe, expect, it } from 'vitest'
import { applyFontSettings, fontSettingsFromEffective } from './font-variables'

describe('font variables', () => {
  afterEach(() => {
    document.documentElement.removeAttribute('style')
  })

  it('applies effective font settings to root css variables', () => {
    applyFontSettings(fontSettingsFromEffective({
      ui_font_family: 'system-sans',
      ui_font_size: 13,
      reading_font_family: 'lxgw-wenkai',
      reading_font_size: 21,
    }))

    const style = document.documentElement.style
    expect(style.getPropertyValue('--nova-ui-font-size')).toBe('13px')
    expect(style.getPropertyValue('--nova-ui-sm-font-size')).toBe('15px')
    expect(style.getPropertyValue('--nova-reading-font-size')).toBe('21px')
    expect(style.getPropertyValue('--nova-reading-font-family')).toContain('LXGW WenKai')
  })

  it('clamps out-of-range sizes before writing variables', () => {
    applyFontSettings({
      uiFontSize: 99,
      readingFontSize: 2,
    })

    const style = document.documentElement.style
    expect(style.getPropertyValue('--nova-ui-font-size')).toBe('16px')
    expect(style.getPropertyValue('--nova-reading-font-size')).toBe('14px')
  })
})
