import type { Monaco } from '@monaco-editor/react'

export const REVIEW_MONACO_THEME_DARK = 'denova-review-dark'
export const REVIEW_MONACO_THEME_LIGHT = 'denova-review-light'

const DARK_COLORS = {
  addedLine: '#1f3124',
  removedLine: '#3c1f1b',
  addedWord: '#32533c',
  removedWord: '#62302a',
} as const

const LIGHT_COLORS = {
  addedLine: '#e2f2e7',
  removedLine: '#f8e2dd',
  addedWord: '#b9e2c3',
  removedWord: '#efb7ad',
} as const

const installedMonacoInstances = new WeakSet<Monaco>()

/** Installs review-only themes so Monaco's split view matches the unified projection. */
export function installReviewMonacoThemes(monaco: Monaco) {
  if (installedMonacoInstances.has(monaco)) return
  installedMonacoInstances.add(monaco)
  monaco.editor.defineTheme(REVIEW_MONACO_THEME_DARK, theme('vs-dark', DARK_COLORS))
  monaco.editor.defineTheme(REVIEW_MONACO_THEME_LIGHT, theme('vs', LIGHT_COLORS))
}

function theme(base: 'vs' | 'vs-dark', colors: typeof DARK_COLORS | typeof LIGHT_COLORS) {
  return {
    base,
    inherit: true,
    rules: [],
    colors: {
      'diffEditor.insertedLineBackground': colors.addedLine,
      'diffEditor.removedLineBackground': colors.removedLine,
      'diffEditor.insertedTextBackground': colors.addedWord,
      'diffEditor.removedTextBackground': colors.removedWord,
      'diffEditorGutter.insertedLineBackground': colors.addedLine,
      'diffEditorGutter.removedLineBackground': colors.removedLine,
    },
  }
}
