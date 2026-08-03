import { render } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Monaco } from '@monaco-editor/react'
import type { editor } from 'monaco-editor'

const harness = vi.hoisted(() => ({
  editorProps: [] as Array<Record<string, unknown>>,
  diffEditorProps: [] as Array<Record<string, unknown>>,
  resolvedTheme: 'dark',
}))

const ENHANCED_TEXT_CONTRAST_RATIO = 7

vi.mock('@monaco-editor/react', () => ({
  Editor: (props: Record<string, unknown>) => {
    harness.editorProps.push(props)
    return <div data-testid="monaco-editor" />
  },
  DiffEditor: (props: Record<string, unknown>) => {
    harness.diffEditorProps.push(props)
    return <div data-testid="monaco-diff-editor" />
  },
}))

vi.mock('next-themes', () => ({
  useTheme: () => ({ resolvedTheme: harness.resolvedTheme }),
}))

import {
  DENOVA_MONACO_THEME_DARK,
  DENOVA_MONACO_THEME_LIGHT,
  DenovaMonacoDiffEditor,
  DenovaMonacoEditor,
  installDenovaMonacoThemes,
} from './DenovaMonaco'

describe('DenovaMonaco', () => {
  beforeEach(() => {
    harness.editorProps.splice(0)
    harness.diffEditorProps.splice(0)
    harness.resolvedTheme = 'dark'
  })

  it('installs syntax-preserving light and dark themes once per Monaco runtime', () => {
    const defineTheme = vi.fn()
    const monaco = { editor: { defineTheme } } as unknown as Monaco

    installDenovaMonacoThemes(monaco)

    expect(defineTheme).toHaveBeenCalledWith(DENOVA_MONACO_THEME_DARK, expect.objectContaining({
      base: 'vs-dark',
      inherit: true,
      rules: expect.arrayContaining([
        expect.objectContaining({ token: 'string' }),
        expect.objectContaining({ token: 'string.value.json' }),
      ]),
      colors: expect.objectContaining({
        'editor.background': '#0a0a0a',
        'editorWidget.border': '#292929',
        'diffEditor.insertedLineBackground': '#1f3124',
        'diffEditor.removedLineBackground': '#3c1f1b',
      }),
    }))
    expect(defineTheme).toHaveBeenCalledWith(DENOVA_MONACO_THEME_LIGHT, expect.objectContaining({
      base: 'vs',
      inherit: true,
      colors: expect.objectContaining({ 'editor.background': '#ffffff' }),
    }))

    installDenovaMonacoThemes(monaco)
    expect(defineTheme).toHaveBeenCalledTimes(2)
  })

  it('keeps all syntax and supporting text at enhanced contrast in both variants', () => {
    const defineTheme = vi.fn()
    const monaco = { editor: { defineTheme } } as unknown as Monaco

    installDenovaMonacoThemes(monaco)

    expectThemeTextContrast(definedTheme(defineTheme, DENOVA_MONACO_THEME_DARK))
    expectThemeTextContrast(definedTheme(defineTheme, DENOVA_MONACO_THEME_LIGHT))
  })

  it('enforces the CJK-safe Unicode policy without disabling spoofing detection', () => {
    render(
      <DenovaMonacoEditor
        options={{
          unicodeHighlight: {
            nonBasicASCII: true,
            invisibleCharacters: false,
            ambiguousCharacters: false,
            allowedLocales: { fr: true },
          },
        }}
      />,
    )

    expect(harness.editorProps.at(-1)).toMatchObject({
      theme: DENOVA_MONACO_THEME_DARK,
      options: {
        unicodeHighlight: {
          nonBasicASCII: false,
          invisibleCharacters: true,
          ambiguousCharacters: true,
          allowedLocales: {
            _os: true,
            _vscode: true,
            fr: true,
            'zh-hans': true,
            'zh-hant': true,
            ja: true,
          },
        },
      },
    })
  })

  it('applies the same policy and light theme to diff editors', () => {
    harness.resolvedTheme = 'light'
    render(<DenovaMonacoDiffEditor original="旧" modified="新" />)

    expect(harness.diffEditorProps.at(-1)).toMatchObject({
      theme: DENOVA_MONACO_THEME_LIGHT,
      options: {
        unicodeHighlight: {
          nonBasicASCII: false,
          invisibleCharacters: true,
          ambiguousCharacters: true,
          allowedLocales: {
            'zh-hans': true,
            'zh-hant': true,
            ja: true,
          },
        },
      },
    })
  })
})

function definedTheme(defineTheme: ReturnType<typeof vi.fn>, name: string): editor.IStandaloneThemeData {
  const definition = defineTheme.mock.calls.find(([themeName]) => themeName === name)?.[1]
  expect(definition).toBeDefined()
  return definition as editor.IStandaloneThemeData
}

function expectThemeTextContrast(theme: editor.IStandaloneThemeData) {
  const background = theme.colors['editor.background']
  const colors = new Set([
    theme.colors['editor.foreground'],
    theme.colors['editorLineNumber.foreground'],
    ...theme.rules.map((rule) => rule.foreground),
  ].filter((color): color is string => Boolean(color)))

  expect(background).toBeDefined()
  for (const color of colors) {
    expect(contrastRatio(color, background!), `${color} against ${background}`)
      .toBeGreaterThanOrEqual(ENHANCED_TEXT_CONTRAST_RATIO)
  }
}

function contrastRatio(foreground: string, background: string) {
  const foregroundLuminance = relativeLuminance(foreground)
  const backgroundLuminance = relativeLuminance(background)
  const lighter = Math.max(foregroundLuminance, backgroundLuminance)
  const darker = Math.min(foregroundLuminance, backgroundLuminance)
  return (lighter + 0.05) / (darker + 0.05)
}

function relativeLuminance(color: string) {
  const hex = color.replace('#', '')
  const [red, green, blue] = [0, 2, 4].map((offset) => Number.parseInt(hex.slice(offset, offset + 2), 16) / 255)
  const [linearRed, linearGreen, linearBlue] = [red, green, blue]
    .map((channel) => channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4)
  return 0.2126 * linearRed + 0.7152 * linearGreen + 0.0722 * linearBlue
}
