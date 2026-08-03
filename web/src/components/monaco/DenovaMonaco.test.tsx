import { render } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Monaco } from '@monaco-editor/react'

const harness = vi.hoisted(() => ({
  editorProps: [] as Array<Record<string, unknown>>,
  diffEditorProps: [] as Array<Record<string, unknown>>,
  resolvedTheme: 'dark',
}))

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
