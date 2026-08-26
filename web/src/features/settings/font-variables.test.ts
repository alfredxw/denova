import { beforeEach, describe, expect, it } from 'vitest'
import { getContentFontScale } from './content-font-scale'
import { applyFontSettings, applyReadingTypographySettings, applyUIFontSize } from './font-variables'

describe('font variables', () => {
  beforeEach(() => {
    document.documentElement.removeAttribute('style')
    applyFontSettings({ uiFontSize: 14, readingFontSize: 18 })
  })

  it('scales the complete interface type ramp from one persisted value', () => {
    applyUIFontSize(18)

    const style = document.documentElement.style
    expect(style.getPropertyValue('--nova-ui-body-font-size')).toBe('18px')
    expect(style.getPropertyValue('--nova-ui-compact-font-size')).toBe('16.7px')
    expect(style.getPropertyValue('--nova-ui-display-font-size')).toBe('30.9px')
    expect(style.getPropertyValue('--nova-ui-font-size')).toBe('18px')
  })

  it('clamps released values and preserves a readable lower bound', () => {
    applyUIFontSize(100)
    expect(document.documentElement.style.getPropertyValue('--nova-ui-font-size')).toBe('18px')

    applyUIFontSize(1)
    expect(document.documentElement.style.getPropertyValue('--nova-ui-nano-font-size')).toBe('10px')
    expect(document.documentElement.style.getPropertyValue('--nova-ui-font-size')).toBe('11px')
  })

  it('derives editor and terminal sizes from the reading scale', () => {
    applyReadingTypographySettings({ readingFontSize: 28 })

    expect(getContentFontScale()).toEqual({
      sourceEditor: 21.8,
      terminal: 18.7,
    })
    expect(document.documentElement.style.getPropertyValue('--nova-source-editor-font-size')).toBe('21.8px')
    expect(document.documentElement.style.getPropertyValue('--nova-terminal-font-size')).toBe('18.7px')
  })
})
