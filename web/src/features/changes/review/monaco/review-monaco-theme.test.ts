import { describe, expect, it, vi } from 'vitest'
import type { Monaco } from '@monaco-editor/react'
import { installReviewMonacoThemes, REVIEW_MONACO_THEME_DARK } from './review-monaco-theme'

describe('installReviewMonacoThemes', () => {
  it('uses the agreed dark line backgrounds for split diffs', () => {
    const defineTheme = vi.fn()
    const monaco = { editor: { defineTheme } } as unknown as Monaco
    installReviewMonacoThemes(monaco)

    expect(defineTheme).toHaveBeenCalledWith(REVIEW_MONACO_THEME_DARK, expect.objectContaining({
      colors: expect.objectContaining({
        'diffEditor.insertedLineBackground': '#1f3124',
        'diffEditor.removedLineBackground': '#3c1f1b',
      }),
    }))

    installReviewMonacoThemes(monaco)
    expect(defineTheme).toHaveBeenCalledTimes(2)
  })
})
